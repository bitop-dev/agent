package plugin

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
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
	plg "github.com/bitop-dev/agent/pkg/plugin"
)

// SourceMatch is a search result entry pairing a source with a discovered plugin.
type SourceMatch struct {
	Source   config.PluginSource
	Manifest plg.Manifest
	Path     string
}

// InstallResult contains the outcome of a successful plugin install.
type InstallResult struct {
	Manifest    plg.Manifest
	Destination string
	Version     string
	Source      string             // "local" for path installs, or the configured source name
	Deps        []DepInstallResult // dependencies that were auto-installed
}

// InstallOptions controls how an install is resolved.
type InstallOptions struct {
	Link         bool   // symlink instead of copy (local installs only)
	SourceFilter string // if non-empty, only consider this source name
	Version      string // if non-empty, install this exact version (registry only)
	SkipDeps     bool   // if true, don't auto-install dependencies
}

// DepInstallResult records a dependency that was auto-installed.
type DepInstallResult struct {
	Name    string
	Version string
	Source  string
}

// ParseNameVersion splits "name@version" into name and version.
// If no "@" is present, version is empty (meaning "latest").
func ParseNameVersion(ref string) (name, version string) {
	if idx := strings.LastIndex(ref, "@"); idx > 0 {
		return ref[:idx], ref[idx+1:]
	}
	return ref, ""
}

func InstallLocal(source, destinationRoot string, link bool) (InstallResult, error) {
	return Install(source, nil, destinationRoot, InstallOptions{Link: link})
}

func Install(source string, sources []config.PluginSource, destinationRoot string, opts InstallOptions) (InstallResult, error) {
	// Filter sources if --source was specified
	filtered := sources
	if opts.SourceFilter != "" {
		filtered = nil
		for _, s := range sources {
			if s.Name == opts.SourceFilter {
				filtered = append(filtered, s)
				break
			}
		}
		if len(filtered) == 0 {
			return InstallResult{}, fmt.Errorf("source %q not found in configured sources", opts.SourceFilter)
		}
	}

	manifestPath, sourceDir, err := resolveSource(source, filtered)
	if err != nil {
		// Local resolution failed. Try registry sources before giving up.
		result, regErr := installFromRegistry(source, filtered, destinationRoot, opts.Version)
		if regErr == nil {
			return result, nil
		}
		// Show both errors so the real problem is visible.
		return InstallResult{}, fmt.Errorf("%w (registry also failed: %v)", err, regErr)
	}
	manifest, err := loaderutil.LoadYAML[plg.Manifest](manifestPath)
	if err != nil {
		return InstallResult{}, err
	}
	if err := ValidateManifest(manifest); err != nil {
		return InstallResult{}, err
	}
	if err := os.MkdirAll(destinationRoot, 0o755); err != nil {
		return InstallResult{}, err
	}
	destinationDir := filepath.Join(destinationRoot, manifest.Metadata.Name)
	if _, err := os.Lstat(destinationDir); err == nil {
		return InstallResult{}, fmt.Errorf("plugin %s already installed at %s", manifest.Metadata.Name, destinationDir)
	} else if !os.IsNotExist(err) {
		return InstallResult{}, err
	}
	if opts.Link {
		if err := os.Symlink(sourceDir, destinationDir); err != nil {
			return InstallResult{}, err
		}
	} else {
		if err := copyDir(sourceDir, destinationDir); err != nil {
			return InstallResult{}, err
		}
	}
	// Determine the source name for record-keeping.
	sourceName := "local"
	for _, s := range filtered {
		if s.Type == "filesystem" && strings.HasPrefix(filepath.Clean(sourceDir), filepath.Clean(s.Path)) {
			sourceName = s.Name
			break
		}
	}
	result := InstallResult{
		Manifest:    manifest,
		Destination: destinationDir,
		Version:     manifest.Metadata.Version,
		Source:      sourceName,
	}

	// Auto-install missing dependencies from the same sources.
	if !opts.SkipDeps {
		installing := map[string]bool{manifest.Metadata.Name: true}
		deps, err := ResolveDependencies(manifest, sources, destinationRoot, installing)
		if err != nil {
			// Dependency failed — the primary plugin is still installed.
			// Return success with the error noted so the CLI can report it.
			result.Deps = deps
			return result, fmt.Errorf("installed %s but dependency resolution failed: %w", manifest.Metadata.Name, err)
		}
		result.Deps = deps
	}

	return result, nil
}

// ResolveDependencies checks the manifest's requires.plugins and auto-installs
// any missing dependencies from the same configured sources. Dependencies are
// installed but NOT enabled — the user must enable and configure them manually.
//
// Returns the list of dependencies that were installed. Already-installed
// dependencies are silently skipped. Circular dependencies are detected and
// prevented via the installing set.
func ResolveDependencies(manifest plg.Manifest, sources []config.PluginSource, destinationRoot string, installing map[string]bool) ([]DepInstallResult, error) {
	if len(manifest.Spec.Requires.Plugins) == 0 {
		return nil, nil
	}

	var installed []DepInstallResult
	for _, dep := range manifest.Spec.Requires.Plugins {
		// Skip if already installed on disk.
		depDir := filepath.Join(destinationRoot, dep)
		if _, err := os.Stat(depDir); err == nil {
			continue
		}
		// Skip if we're already in the process of installing this dep (circular).
		if installing[dep] {
			continue
		}
		installing[dep] = true

		result, err := Install(dep, sources, destinationRoot, InstallOptions{SkipDeps: false})
		if err != nil {
			return installed, fmt.Errorf("auto-install dependency %q (required by %s): %w", dep, manifest.Metadata.Name, err)
		}
		installed = append(installed, DepInstallResult{
			Name:    result.Manifest.Metadata.Name,
			Version: result.Version,
			Source:  result.Source,
		})

		// Recurse: the dependency itself may have dependencies.
		transitive, err := ResolveDependencies(result.Manifest, sources, destinationRoot, installing)
		if err != nil {
			return installed, err
		}
		installed = append(installed, transitive...)
	}
	return installed, nil
}

// Upgrade removes the currently installed version of a plugin and reinstalls
// it from its recorded source (or any configured source if the source is gone).
func Upgrade(name string, sources []config.PluginSource, cfg config.Config, destinationRoot string) (InstallResult, error) {
	currentCfg := cfg.Plugins[name]
	sourceFilter := currentCfg.InstalledSource
	if sourceFilter == "local" {
		return InstallResult{}, fmt.Errorf("plugin %q was installed from a local path — use plugins install <path> to reinstall", name)
	}

	// Check registry for the latest version before removing current install.
	result, err := installFromRegistry(name, sources, destinationRoot+".upgrade-tmp", "")
	if err != nil {
		return InstallResult{}, fmt.Errorf("could not fetch upgrade for %q: %w", name, err)
	}
	// Clean up temp dir used during version check.
	os.RemoveAll(result.Destination)

	if result.Version == currentCfg.InstalledVersion {
		return InstallResult{}, fmt.Errorf("plugin %q is already at the latest version (%s)", name, result.Version)
	}

	// Remove current install and reinstall.
	if _, err := RemoveInstalled(name, destinationRoot); err != nil {
		return InstallResult{}, fmt.Errorf("remove current install: %w", err)
	}
	return Install(name, sources, destinationRoot, InstallOptions{SourceFilter: sourceFilter})
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

// PublishResult contains the outcome of a successful publish operation.
type PublishResult struct {
	Name    string
	Version string
	Source  string
}

// Publish packs a plugin directory into a .tar.gz and POSTs it to a registry source.
// The source must be of type "registry" and have a publishToken configured.
func Publish(pluginPath string, sources []config.PluginSource, sourceFilter string) (PublishResult, error) {
	absPath, err := filepath.Abs(pluginPath)
	if err != nil {
		return PublishResult{}, err
	}

	// Load and validate the manifest.
	manifestPath := filepath.Join(absPath, "plugin.yaml")
	if _, err := os.Stat(manifestPath); err != nil {
		return PublishResult{}, fmt.Errorf("no plugin.yaml found in %s", absPath)
	}
	manifest, err := loaderutil.LoadYAML[plg.Manifest](manifestPath)
	if err != nil {
		return PublishResult{}, fmt.Errorf("load manifest: %w", err)
	}
	if err := ValidateManifest(manifest); err != nil {
		return PublishResult{}, err
	}

	// Find the target registry source.
	var target *config.PluginSource
	for i, s := range sources {
		if s.Type != "registry" || !s.Enabled {
			continue
		}
		if sourceFilter != "" && s.Name != sourceFilter {
			continue
		}
		if s.PublishToken == "" {
			continue
		}
		target = &sources[i]
		break
	}
	if target == nil {
		if sourceFilter != "" {
			return PublishResult{}, fmt.Errorf("no publishable registry source named %q — check that it exists, is enabled, and has a publishToken configured", sourceFilter)
		}
		return PublishResult{}, fmt.Errorf("no publishable registry source found — add a registry source with a publishToken")
	}

	// Build the tarball in memory.
	tarball, err := packPlugin(absPath, manifest.Metadata.Name)
	if err != nil {
		return PublishResult{}, fmt.Errorf("pack plugin: %w", err)
	}

	// POST to registry.
	registryURL := strings.TrimRight(target.URL, "/") + "/v1/packages"
	req, err := http.NewRequest(http.MethodPost, registryURL, bytes.NewReader(tarball))
	if err != nil {
		return PublishResult{}, err
	}
	req.Header.Set("Authorization", "Bearer "+target.PublishToken)
	req.Header.Set("Content-Type", "application/gzip")

	httpClient := &http.Client{Timeout: 60 * time.Second}
	resp, err := httpClient.Do(req)
	if err != nil {
		return PublishResult{}, fmt.Errorf("publish to %s: %w", target.URL, err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusCreated {
		return PublishResult{}, fmt.Errorf("registry returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	return PublishResult{
		Name:    manifest.Metadata.Name,
		Version: manifest.Metadata.Version,
		Source:  target.Name,
	}, nil
}

// packPlugin builds a .tar.gz of a plugin directory with <name>/ as the top-level directory.
func packPlugin(dir, name string) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	skipNames := map[string]bool{".git": true, "node_modules": true, ".DS_Store": true}

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if skipNames[d.Name()] {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() && strings.HasPrefix(d.Name(), ".") {
			return filepath.SkipDir
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		hdr, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return err
		}
		hdr.Name = filepath.ToSlash(filepath.Join(name, rel))
		hdr.ModTime = time.Unix(0, 0)
		hdr.AccessTime = time.Unix(0, 0)
		hdr.ChangeTime = time.Unix(0, 0)
		hdr.Uid, hdr.Gid = 0, 0
		if d.IsDir() && !strings.HasSuffix(hdr.Name, "/") {
			hdr.Name += "/"
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		f, err := os.Open(path)
		if err != nil {
			return err
		}
		defer f.Close()
		_, err = io.Copy(tw, f)
		return err
	})
	if err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}
