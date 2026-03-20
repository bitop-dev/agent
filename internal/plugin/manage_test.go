package plugin

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/ncecere/agent/pkg/config"
)

func TestResolveSourceFromFilesystemPluginSource(t *testing.T) {
	tempDir := t.TempDir()
	pluginDir := filepath.Join(tempDir, "send-email")
	writePluginManifest(t, pluginDir, "send-email")

	manifestPath, sourceDir, err := resolveSource("send-email", []config.PluginSource{{
		Name:    "local",
		Type:    "filesystem",
		Path:    tempDir,
		Enabled: true,
	}})
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if sourceDir != pluginDir {
		t.Fatalf("unexpected source dir: %s", sourceDir)
	}
	if manifestPath != filepath.Join(pluginDir, "plugin.yaml") {
		t.Fatalf("unexpected manifest path: %s", manifestPath)
	}
}

func TestResolveSourcePrefersExplicitPath(t *testing.T) {
	tempDir := t.TempDir()
	pluginDir := filepath.Join(tempDir, "send-email")
	writePluginManifest(t, pluginDir, "send-email")

	manifestPath, sourceDir, err := resolveSource(pluginDir, nil)
	if err != nil {
		t.Fatalf("resolve source: %v", err)
	}
	if sourceDir != pluginDir {
		t.Fatalf("unexpected source dir: %s", sourceDir)
	}
	if manifestPath != filepath.Join(pluginDir, "plugin.yaml") {
		t.Fatalf("unexpected manifest path: %s", manifestPath)
	}
}

func TestResolveSourceReturnsHelpfulErrorWhenNoSourcesMatch(t *testing.T) {
	_, _, err := resolveSource("missing-plugin", []config.PluginSource{{
		Name:    "local",
		Type:    "filesystem",
		Path:    t.TempDir(),
		Enabled: true,
	}})
	if err == nil {
		t.Fatal("expected error")
	}
	if got := err.Error(); got == "" {
		t.Fatal("expected non-empty error")
	}
}

func TestResolveSourceReturnsErrorWhenOnlyRegistryConfigured(t *testing.T) {
	// resolveSource only checks local paths; registry resolution happens in Install.
	// When only a registry source is configured and the plugin isn't found locally,
	// resolveSource should return a "not found" error.
	_, _, err := resolveSource("send-email", []config.PluginSource{{
		Name:    "registry",
		Type:    "registry",
		URL:     "https://plugins.example.com",
		Enabled: true,
	}})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestSearchSourcesFindsPluginsByNameAndDescription(t *testing.T) {
	tempDir := t.TempDir()
	writePluginManifest(t, filepath.Join(tempDir, "send-email"), "send-email")
	writePluginManifestWithDescription(t, filepath.Join(tempDir, "web-research"), "web-research", "Web search and fetch plugin")

	matches, err := SearchSources("email", []config.PluginSource{{
		Name:    "local",
		Type:    "filesystem",
		Path:    tempDir,
		Enabled: true,
	}})
	if err != nil {
		t.Fatalf("search sources: %v", err)
	}
	if len(matches) != 1 || matches[0].Manifest.Metadata.Name != "send-email" {
		t.Fatalf("unexpected matches: %#v", matches)
	}

	matches, err = SearchSources("search", []config.PluginSource{{
		Name:    "local",
		Type:    "filesystem",
		Path:    tempDir,
		Enabled: true,
	}})
	if err != nil {
		t.Fatalf("search sources: %v", err)
	}
	if len(matches) != 1 || matches[0].Manifest.Metadata.Name != "web-research" {
		t.Fatalf("unexpected matches: %#v", matches)
	}
}

func TestSearchSourcesReturnsAllWhenQueryEmpty(t *testing.T) {
	tempDir := t.TempDir()
	writePluginManifest(t, filepath.Join(tempDir, "a"), "a")
	writePluginManifest(t, filepath.Join(tempDir, "b"), "b")
	matches, err := SearchSources("", []config.PluginSource{{
		Name:    "local",
		Type:    "filesystem",
		Path:    tempDir,
		Enabled: true,
	}})
	if err != nil {
		t.Fatalf("search sources: %v", err)
	}
	if len(matches) != 2 {
		t.Fatalf("expected 2 matches, got %d", len(matches))
	}
}

func TestSearchSourcesReturnsErrorWhenRegistryUnreachable(t *testing.T) {
	// SearchSources now attempts to reach registry sources. An unreachable URL
	// should return a network error, not a "not implemented" message.
	_, err := SearchSources("email", []config.PluginSource{{
		Name:    "registry",
		Type:    "registry",
		URL:     "http://127.0.0.1:19999", // nothing listening here
		Enabled: true,
	}})
	if err == nil {
		t.Fatal("expected error for unreachable registry")
	}
}

func writePluginManifest(t *testing.T, dir, name string) {
	t.Helper()
	writePluginManifestWithDescription(t, dir, name, "example")
}

func writePluginManifestWithDescription(t *testing.T, dir, name, description string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "apiVersion: agent/v1\nkind: Plugin\nmetadata:\n  name: " + name + "\n  version: 1.0.0\n  description: " + description + "\nspec:\n  category: asset\n  runtime:\n    type: asset\n  contributes:\n    tools: []\n    prompts: []\n    profileTemplates: []\n    policies: []\n  configSchema:\n    type: object\n    properties: {}\n    required: []\n  permissions: {}\n  requires:\n    framework: \">=0.1.0\"\n    plugins: []\n"
	if err := os.WriteFile(filepath.Join(dir, "plugin.yaml"), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
}
