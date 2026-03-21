package profile

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	loaderutil "github.com/bitop-dev/agent/internal/loader"
	"github.com/bitop-dev/agent/pkg/config"
	pf "github.com/bitop-dev/agent/pkg/profile"
)

type Discovered struct {
	Reference pf.Reference
	Manifest  pf.Manifest
}

type Loader struct {
	Roots         []string
	InstallRoot   string               // where to install profiles from registry (e.g. ~/.agent/profiles)
	PluginSources []config.PluginSource // registry sources to search for profiles
}

func (l Loader) Discover(context.Context) ([]Discovered, error) {
	var out []Discovered
	for _, root := range l.Roots {
		if root == "" {
			continue
		}
		if _, err := os.Stat(root); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				continue
			}
			return nil, err
		}
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() || (d.Name() != "profile.yaml" && d.Name() != "profile.yml") {
				return nil
			}
			manifest, err := loaderutil.LoadYAML[pf.Manifest](path)
			if err != nil {
				return err
			}
			out = append(out, Discovered{
				Reference: pf.Reference{Name: manifest.Metadata.Name, Path: path},
				Manifest:  manifest,
			})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Reference.Name < out[j].Reference.Name })
	return out, nil
}

func (l Loader) Load(ctx context.Context, ref string) (pf.Manifest, string, error) {
	if _, err := os.Stat(ref); err == nil {
		if info, statErr := os.Stat(ref); statErr == nil && info.IsDir() {
			for _, candidate := range []string{"profile.yaml", "profile.yml"} {
				manifestPath := filepath.Join(ref, candidate)
				if _, candidateErr := os.Stat(manifestPath); candidateErr == nil {
					manifest, loadErr := loaderutil.LoadYAML[pf.Manifest](manifestPath)
					return manifest, manifestPath, loadErr
				}
			}
			return pf.Manifest{}, "", fs.ErrNotExist
		}
		manifest, err := loaderutil.LoadYAML[pf.Manifest](ref)
		return manifest, ref, err
	}
	profiles, err := l.Discover(ctx)
	if err != nil {
		return pf.Manifest{}, "", err
	}
	for _, profile := range profiles {
		if profile.Reference.Name == ref {
			return profile.Manifest, profile.Reference.Path, nil
		}
	}

	// Not found locally — try installing from registry.
	if l.InstallRoot != "" && len(l.PluginSources) > 0 {
		if manifest, path, err := l.installFromRegistry(ref); err == nil {
			fmt.Fprintf(os.Stderr, "[on-demand] installed profile %q from registry\n", ref)
			return manifest, path, nil
		}
	}

	return pf.Manifest{}, "", fs.ErrNotExist
}

// installFromRegistry downloads a profile package from a configured registry source.
func (l Loader) installFromRegistry(name string) (pf.Manifest, string, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	for _, source := range l.PluginSources {
		if !source.Enabled || source.Type != "registry" || strings.TrimSpace(source.URL) == "" {
			continue
		}
		// Fetch profile metadata.
		metaURL := strings.TrimRight(source.URL, "/") + "/v1/profiles/" + name + ".json"
		resp, err := client.Get(metaURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			if resp != nil {
				resp.Body.Close()
			}
			continue
		}
		var meta struct {
			Versions []struct {
				Version  string `json:"version"`
				Artifact struct {
					URL    string `json:"url"`
					SHA256 string `json:"sha256"`
				} `json:"artifact"`
			} `json:"versions"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil || len(meta.Versions) == 0 {
			resp.Body.Close()
			continue
		}
		resp.Body.Close()

		// Download the tarball.
		artifactURL := meta.Versions[0].Artifact.URL
		if artifactURL == "" {
			continue
		}
		artResp, err := client.Get(artifactURL)
		if err != nil || artResp.StatusCode != http.StatusOK {
			if artResp != nil {
				artResp.Body.Close()
			}
			continue
		}
		defer artResp.Body.Close()

		// Save to temp file.
		tmp, err := os.CreateTemp("", "agent-profile-*.tar.gz")
		if err != nil {
			continue
		}
		if _, err := io.Copy(tmp, artResp.Body); err != nil {
			tmp.Close()
			os.Remove(tmp.Name())
			continue
		}
		tmp.Close()

		// Extract to install root.
		destDir := filepath.Join(l.InstallRoot, name)
		if _, err := os.Stat(destDir); err == nil {
			os.Remove(tmp.Name())
			// Already installed (race condition) — load it.
			break
		}
		if err := extractProfileTarball(tmp.Name(), l.InstallRoot); err != nil {
			os.Remove(tmp.Name())
			continue
		}
		os.Remove(tmp.Name())

		// Load the installed profile.
		for _, candidate := range []string{"profile.yaml", "profile.yml"} {
			manifestPath := filepath.Join(destDir, candidate)
			if _, err := os.Stat(manifestPath); err == nil {
				manifest, err := loaderutil.LoadYAML[pf.Manifest](manifestPath)
				if err != nil {
					return pf.Manifest{}, "", err
				}
				return manifest, manifestPath, nil
			}
		}
	}
	return pf.Manifest{}, "", fs.ErrNotExist
}

// extractProfileTarball extracts a .tar.gz into destRoot.
func extractProfileTarball(tarPath, destRoot string) error {
	f, err := os.Open(tarPath)
	if err != nil {
		return err
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	cleanRoot := filepath.Clean(destRoot) + string(os.PathSeparator)
	if err := os.MkdirAll(destRoot, 0o755); err != nil {
		return err
	}

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name := filepath.ToSlash(hdr.Name)
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			continue
		}
		target := filepath.Join(destRoot, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanRoot) {
			continue
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			os.MkdirAll(target, 0o755)
		case tar.TypeReg:
			os.MkdirAll(filepath.Dir(target), 0o755)
			mode := os.FileMode(hdr.Mode) & 0o755
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			io.Copy(out, tr)
			out.Close()
		}
	}
	return nil
}
