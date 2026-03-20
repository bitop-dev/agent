package plugin

import (
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	loaderutil "github.com/ncecere/agent/internal/loader"
	"github.com/ncecere/agent/pkg/config"
	plg "github.com/ncecere/agent/pkg/plugin"
)

type SourceMatch struct {
	Source   config.PluginSource
	Manifest plg.Manifest
	Path     string
}

func InstallLocal(source, destinationRoot string, link bool) (plg.Manifest, string, error) {
	return Install(source, nil, destinationRoot, link)
}

func Install(source string, sources []config.PluginSource, destinationRoot string, link bool) (plg.Manifest, string, error) {
	manifestPath, sourceDir, err := resolveSource(source, sources)
	if err != nil {
		// Local resolution failed. Try registry sources before giving up.
		if manifest, dest, regErr := installFromRegistry(source, sources, destinationRoot); regErr == nil {
			return manifest, dest, nil
		}
		return plg.Manifest{}, "", err
	}
	manifest, err := loaderutil.LoadYAML[plg.Manifest](manifestPath)
	if err != nil {
		return plg.Manifest{}, "", err
	}
	if err := ValidateManifest(manifest); err != nil {
		return plg.Manifest{}, "", err
	}
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		return plg.Manifest{}, "", err
	}
	destinationDir := filepath.Join(destinationRoot, manifest.Metadata.Name)
	if _, err := os.Lstat(destinationDir); err == nil {
		return plg.Manifest{}, "", fmt.Errorf("plugin %s already installed at %s", manifest.Metadata.Name, destinationDir)
	} else if !os.IsNotExist(err) {
		return plg.Manifest{}, "", err
	}
	if link {
		if err := os.Symlink(sourceDir, destinationDir); err != nil {
			return plg.Manifest{}, "", err
		}
	} else {
		if err := copyDir(sourceDir, destinationDir); err != nil {
			return plg.Manifest{}, "", err
		}
	}
	return manifest, destinationDir, nil
}

func SearchSources(query string, sources []config.PluginSource) ([]SourceMatch, error) {
	needle := strings.ToLower(strings.TrimSpace(query))
	var matches []SourceMatch
	seen := make(map[string]struct{})
	registrySources := 0
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		typeName := source.Type
		if typeName == "" {
			typeName = "filesystem"
		}
		if typeName == "registry" {
			registrySources++
			continue
		}
		if typeName != "filesystem" || strings.TrimSpace(source.Path) == "" {
			continue
		}
		entries, err := os.ReadDir(source.Path)
		if err != nil {
			return nil, err
		}
		for _, entry := range entries {
			if !entry.IsDir() {
				continue
			}
			pluginDir := filepath.Join(source.Path, entry.Name())
			manifestPath, _, err := resolveSource(pluginDir, nil)
			if err != nil {
				continue
			}
			manifest, err := loaderutil.LoadYAML[plg.Manifest](manifestPath)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[manifest.Metadata.Name]; ok {
				continue
			}
			haystack := strings.ToLower(strings.Join([]string{
				manifest.Metadata.Name,
				manifest.Metadata.Description,
				string(manifest.Spec.Category),
				string(manifest.Spec.Runtime.Type),
			}, " "))
			if needle != "" && !strings.Contains(haystack, needle) {
				continue
			}
			seen[manifest.Metadata.Name] = struct{}{}
			matches = append(matches, SourceMatch{Source: source, Manifest: manifest, Path: pluginDir})
		}
	}
	// Search registry sources.
	if registrySources > 0 {
		regMatches, err := searchRegistrySources(needle, sources, seen)
		if err != nil {
			return nil, err
		}
		matches = append(matches, regMatches...)
	}

	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Manifest.Metadata.Name == matches[j].Manifest.Metadata.Name {
			return matches[i].Source.Name < matches[j].Source.Name
		}
		return matches[i].Manifest.Metadata.Name < matches[j].Manifest.Metadata.Name
	})
	return matches, nil
}

func RemoveInstalled(name, destinationRoot string) (string, error) {
	destinationDir := filepath.Join(destinationRoot, name)
	if _, err := os.Lstat(destinationDir); err != nil {
		if os.IsNotExist(err) {
			return "", fmt.Errorf("plugin %s is not installed in %s", name, destinationRoot)
		}
		return "", err
	}
	if err := os.RemoveAll(destinationDir); err != nil {
		return "", err
	}
	return destinationDir, nil
}

func resolveSource(source string, sources []config.PluginSource) (manifestPath string, sourceDir string, err error) {
	absSource, err := filepath.Abs(source)
	if err != nil {
		return "", "", err
	}
	info, err := os.Stat(absSource)
	if err != nil {
		return resolveFromSources(source, sources)
	}
	source = absSource
	if info.IsDir() {
		for _, candidate := range []string{"plugin.yaml", "plugin.yml"} {
			manifestPath = filepath.Join(source, candidate)
			if _, err := os.Stat(manifestPath); err == nil {
				return manifestPath, source, nil
			}
		}
		return "", "", fmt.Errorf("no plugin manifest found in %s", source)
	}
	base := filepath.Base(source)
	if base != "plugin.yaml" && base != "plugin.yml" {
		return "", "", fmt.Errorf("plugin source must be a directory or plugin manifest")
	}
	return source, filepath.Dir(source), nil
}

func resolveFromSources(ref string, sources []config.PluginSource) (manifestPath string, sourceDir string, err error) {
	var checked []string
	for _, source := range sources {
		if !source.Enabled {
			continue
		}
		typeName := source.Type
		if typeName == "" {
			typeName = "filesystem"
		}
		switch typeName {
		case "filesystem":
			if source.Path == "" {
				continue
			}
			candidateDir := filepath.Join(source.Path, ref)
			checked = append(checked, candidateDir)
			for _, candidate := range []string{"plugin.yaml", "plugin.yml"} {
				candidateManifest := filepath.Join(candidateDir, candidate)
				if _, statErr := os.Stat(candidateManifest); statErr == nil {
					return candidateManifest, candidateDir, nil
				}
			}
		case "registry":
			// Registry sources are resolved by Install via installFromRegistry.
		}
	}
	if len(checked) == 0 {
		return "", "", fmt.Errorf("plugin source %q not found; pass a local path or configure plugin sources", ref)
	}
	return "", "", fmt.Errorf("plugin %q not found in configured sources: %s", ref, checked)
}

func copyDir(sourceDir, destinationDir string) error {
	return filepath.WalkDir(sourceDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destinationDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(sourcePath, destinationPath string) error {
	if err := os.MkdirAll(filepath.Dir(destinationPath), 0o755); err != nil {
		return err
	}
	source, err := os.Open(sourcePath)
	if err != nil {
		return err
	}
	defer source.Close()
	target, err := os.Create(destinationPath)
	if err != nil {
		return err
	}
	defer target.Close()
	if _, err := io.Copy(target, source); err != nil {
		return err
	}
	return target.Chmod(0o644)
}
