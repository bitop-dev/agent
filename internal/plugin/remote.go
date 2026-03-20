package plugin

import (
	"archive/tar"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	loaderutil "github.com/ncecere/agent/internal/loader"
	"github.com/ncecere/agent/pkg/config"
	plg "github.com/ncecere/agent/pkg/plugin"
)

// ─── Registry response types ───────────────────────────────────────────────

type registryIndex struct {
	APIVersion  string          `json:"apiVersion"`
	GeneratedAt string          `json:"generatedAt"`
	Packages    []registryEntry `json:"packages"`
}

type registryEntry struct {
	Name          string   `json:"name"`
	LatestVersion string   `json:"latestVersion"`
	Description   string   `json:"description"`
	Category      string   `json:"category"`
	Runtime       string   `json:"runtime"`
	Keywords      []string `json:"keywords"`
	Source        string   `json:"source"`
}

type registryPackageMeta struct {
	APIVersion  string            `json:"apiVersion"`
	Name        string            `json:"name"`
	Description string            `json:"description"`
	Versions    []registryVersion `json:"versions"`
}

type registryVersion struct {
	Version  string           `json:"version"`
	Framework string          `json:"framework"`
	Runtime  string           `json:"runtime"`
	Artifact registryArtifact `json:"artifact"`
}

type registryArtifact struct {
	Type   string `json:"type"`
	URL    string `json:"url"`
	SHA256 string `json:"sha256"`
}

var registryHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ─── Index / metadata fetch ────────────────────────────────────────────────

func fetchRegistryIndex(baseURL string) (*registryIndex, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/index.json"
	resp, err := registryHTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("registry %s: %w", baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry %s: index returned %d", baseURL, resp.StatusCode)
	}
	var index registryIndex
	if err := json.NewDecoder(resp.Body).Decode(&index); err != nil {
		return nil, fmt.Errorf("registry %s: decode index: %w", baseURL, err)
	}
	return &index, nil
}

func fetchRegistryPackageMeta(baseURL, name string) (*registryPackageMeta, error) {
	url := strings.TrimRight(baseURL, "/") + "/v1/packages/" + name + ".json"
	resp, err := registryHTTPClient.Get(url)
	if err != nil {
		return nil, fmt.Errorf("registry package %q: %w", name, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return nil, fmt.Errorf("package %q not found in registry", name)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("registry package %q returned %d", name, resp.StatusCode)
	}
	var meta registryPackageMeta
	if err := json.NewDecoder(resp.Body).Decode(&meta); err != nil {
		return nil, fmt.Errorf("registry package %q decode: %w", name, err)
	}
	return &meta, nil
}

// ─── Registry search ───────────────────────────────────────────────────────

// searchRegistrySources fetches the index from every enabled registry source and
// returns matching entries. It is called by SearchSources for registry sources.
func searchRegistrySources(query string, sources []config.PluginSource, seen map[string]struct{}) ([]SourceMatch, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	var matches []SourceMatch
	for _, source := range sources {
		if !source.Enabled || source.Type != "registry" || strings.TrimSpace(source.URL) == "" {
			continue
		}
		index, err := fetchRegistryIndex(source.URL)
		if err != nil {
			return nil, err
		}
		for _, entry := range index.Packages {
			if _, ok := seen[entry.Name]; ok {
				continue
			}
			if needle != "" {
				haystack := strings.ToLower(strings.Join(append(
					[]string{entry.Name, entry.Description, entry.Category, entry.Runtime},
					entry.Keywords...,
				), " "))
				if !strings.Contains(haystack, needle) {
					continue
				}
			}
			seen[entry.Name] = struct{}{}
			matches = append(matches, SourceMatch{
				Source:   source,
				Manifest: entryToManifest(entry),
			})
		}
	}
	return matches, nil
}

// entryToManifest builds a display-only plg.Manifest from a registry index entry.
func entryToManifest(entry registryEntry) plg.Manifest {
	return plg.Manifest{
		Metadata: plg.Metadata{
			Name:        entry.Name,
			Version:     entry.LatestVersion,
			Description: entry.Description,
		},
		Spec: plg.Spec{
			Category: plg.Category(entry.Category),
			Runtime:  plg.Runtime{Type: plg.RuntimeType(entry.Runtime)},
		},
	}
}

// ─── Registry install ──────────────────────────────────────────────────────

// installFromRegistry finds a package by name across enabled registry sources,
// downloads and verifies the artifact, and extracts it to destinationRoot.
func installFromRegistry(name string, sources []config.PluginSource, destinationRoot string) (plg.Manifest, string, error) {
	for _, source := range sources {
		if !source.Enabled || source.Type != "registry" || strings.TrimSpace(source.URL) == "" {
			continue
		}
		meta, err := fetchRegistryPackageMeta(source.URL, name)
		if err != nil {
			// Package not found in this registry; try the next one.
			continue
		}
		if len(meta.Versions) == 0 {
			continue
		}
		// Use the first (latest) version.
		ver := meta.Versions[0]
		if ver.Artifact.URL == "" {
			return plg.Manifest{}, "", fmt.Errorf("registry package %q has no artifact URL", name)
		}
		destDir, err := downloadAndExtract(ver.Artifact.URL, ver.Artifact.SHA256, destinationRoot)
		if err != nil {
			return plg.Manifest{}, "", err
		}
		// Load the real manifest from the extracted plugin directory.
		manifest, path, err := findAndLoadManifest(destDir)
		if err != nil {
			os.RemoveAll(destDir)
			return plg.Manifest{}, "", fmt.Errorf("loaded artifact for %q but could not read plugin.yaml: %w", name, err)
		}
		_ = path
		if err := ValidateManifest(manifest); err != nil {
			os.RemoveAll(destDir)
			return plg.Manifest{}, "", err
		}
		return manifest, destDir, nil
	}
	return plg.Manifest{}, "", fmt.Errorf("plugin %q not found in any configured registry source", name)
}

// findAndLoadManifest finds plugin.yaml in a directory and loads it.
func findAndLoadManifest(dir string) (plg.Manifest, string, error) {
	for _, candidate := range []string{"plugin.yaml", "plugin.yml"} {
		p := filepath.Join(dir, candidate)
		if _, err := os.Stat(p); err == nil {
			manifest, err := loaderutil.LoadYAML[plg.Manifest](p)
			if err != nil {
				return plg.Manifest{}, "", err
			}
			return manifest, p, nil
		}
	}
	return plg.Manifest{}, "", fmt.Errorf("no plugin manifest found in %s", dir)
}

// ─── Artifact download and extraction ─────────────────────────────────────

// downloadAndExtract downloads a tar.gz artifact URL, verifies its SHA256
// checksum (if non-empty), and extracts it into destinationRoot.
// It returns the path of the top-level plugin directory extracted.
func downloadAndExtract(artifactURL, expectedSHA256, destinationRoot string) (string, error) {
	resp, err := registryHTTPClient.Get(artifactURL)
	if err != nil {
		return "", fmt.Errorf("download %s: %w", artifactURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("download %s: returned %d", artifactURL, resp.StatusCode)
	}

	// Stream into a temp file while computing checksum simultaneously.
	tmpFile, err := os.CreateTemp("", "agent-plugin-*.tar.gz")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer os.Remove(tmpPath)

	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(tmpFile, h), resp.Body); err != nil {
		tmpFile.Close()
		return "", fmt.Errorf("download %s: %w", artifactURL, err)
	}
	tmpFile.Close()

	if expectedSHA256 != "" {
		got := hex.EncodeToString(h.Sum(nil))
		if got != expectedSHA256 {
			return "", fmt.Errorf("checksum mismatch for %s: expected %s, got %s", artifactURL, expectedSHA256, got)
		}
	}

	// Peek at the archive to find the top-level directory name.
	topDir, err := tarTopDir(tmpPath)
	if err != nil {
		return "", fmt.Errorf("inspect archive %s: %w", artifactURL, err)
	}

	destDir := filepath.Join(destinationRoot, topDir)
	if _, err := os.Lstat(destDir); err == nil {
		return "", fmt.Errorf("plugin %s already installed at %s", topDir, destDir)
	}

	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		return "", err
	}

	f, err := os.Open(tmpPath)
	if err != nil {
		return "", err
	}
	defer f.Close()

	if err := extractTarGz(f, destinationRoot); err != nil {
		os.RemoveAll(destDir)
		return "", fmt.Errorf("extract archive: %w", err)
	}

	return destDir, nil
}

// tarTopDir returns the name of the top-level directory in a .tar.gz archive.
func tarTopDir(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	gr, err := gzip.NewReader(f)
	if err != nil {
		return "", err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return "", err
		}
		parts := strings.SplitN(filepath.ToSlash(hdr.Name), "/", 2)
		if len(parts) > 0 && parts[0] != "" && parts[0] != "." {
			return parts[0], nil
		}
	}
	return "", fmt.Errorf("archive has no top-level directory")
}

// extractTarGz extracts a gzipped tar stream into destRoot.
// Rejects absolute paths and path traversal entries.
func extractTarGz(r io.Reader, destRoot string) error {
	cleanRoot := filepath.Clean(destRoot) + string(os.PathSeparator)
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		// Reject dangerous paths.
		name := filepath.ToSlash(hdr.Name)
		if strings.HasPrefix(name, "/") || strings.Contains(name, "..") {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		target := filepath.Join(destRoot, filepath.FromSlash(name))
		if !strings.HasPrefix(filepath.Clean(target)+string(os.PathSeparator), cleanRoot) {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(target, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
			mode := os.FileMode(hdr.Mode) & 0o755
			if mode == 0 {
				mode = 0o644
			}
			out, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
			if err != nil {
				return err
			}
			if _, err := io.Copy(out, tr); err != nil {
				out.Close()
				return err
			}
			out.Close()
		}
	}
	return nil
}
