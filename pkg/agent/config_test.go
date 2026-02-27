package agent_test

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/nickcecere/agent/pkg/agent"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f := filepath.Join(t.TempDir(), "agent.yaml")
	if err := os.WriteFile(f, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestLoadFileConfig_Minimal(t *testing.T) {
	f := writeConfig(t, `
provider: anthropic
model: claude-sonnet-4-5
api_key: sk-test
`)
	cfg, err := agent.LoadFileConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Provider != "anthropic" {
		t.Errorf("provider = %q", cfg.Provider)
	}
	if cfg.Model != "claude-sonnet-4-5" {
		t.Errorf("model = %q", cfg.Model)
	}
	if cfg.APIKey != "sk-test" {
		t.Errorf("api_key = %q", cfg.APIKey)
	}
}

func TestLoadFileConfig_EnvExpansion(t *testing.T) {
	t.Setenv("TEST_AGENT_KEY", "sk-from-env")
	f := writeConfig(t, `
provider: openai-completions
model: gpt-4o
api_key: ${TEST_AGENT_KEY}
`)
	cfg, err := agent.LoadFileConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.APIKey != "sk-from-env" {
		t.Errorf("env expansion failed, api_key = %q", cfg.APIKey)
	}
}

func TestLoadFileConfig_MissingProvider(t *testing.T) {
	f := writeConfig(t, `model: gpt-4o`)
	_, err := agent.LoadFileConfig(f)
	if err == nil {
		t.Error("expected error for missing provider")
	}
}

func TestLoadFileConfig_MissingModel(t *testing.T) {
	f := writeConfig(t, `provider: anthropic`)
	_, err := agent.LoadFileConfig(f)
	if err == nil {
		t.Error("expected error for missing model")
	}
}

func TestLoadFileConfig_FileNotFound(t *testing.T) {
	_, err := agent.LoadFileConfig("/definitely/does/not/exist.yaml")
	if err == nil {
		t.Error("expected error for missing file")
	}
}

func TestLoadFileConfig_FullFields(t *testing.T) {
	f := writeConfig(t, `
provider: anthropic
model: claude-opus-4-5
api_key: sk-test
max_tokens: 2048
max_turns: 75
thinking_level: high
cache_retention: long
context_window: 200000

compaction:
  enabled: true
  reserve_tokens: 8192
  keep_recent_tokens: 10000

tools:
  preset: all
  work_dir: /tmp
`)
	cfg, err := agent.LoadFileConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MaxTokens != 2048 {
		t.Errorf("max_tokens = %d", cfg.MaxTokens)
	}
	if cfg.MaxTurns != 75 {
		t.Errorf("max_turns = %d", cfg.MaxTurns)
	}
	if cfg.ThinkingLevel != "high" {
		t.Errorf("thinking_level = %q", cfg.ThinkingLevel)
	}
	if cfg.CacheRetention != "long" {
		t.Errorf("cache_retention = %q", cfg.CacheRetention)
	}
	if cfg.ContextWindow != 200000 {
		t.Errorf("context_window = %d", cfg.ContextWindow)
	}
	if !cfg.Compaction.Enabled {
		t.Error("compaction.enabled should be true")
	}
	if cfg.Compaction.ReserveTokens != 8192 {
		t.Errorf("compaction.reserve_tokens = %d", cfg.Compaction.ReserveTokens)
	}
	if cfg.Tools.Preset != "all" {
		t.Errorf("tools.preset = %q", cfg.Tools.Preset)
	}
}

func TestLoadFileConfig_ToolPreset_Default(t *testing.T) {
	f := writeConfig(t, `
provider: anthropic
model: claude-sonnet-4-5
`)
	cfg, err := agent.LoadFileConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolPreset() != "coding" {
		t.Errorf("default preset = %q, want coding", cfg.ToolPreset())
	}
}

func TestLoadFileConfig_ToolPreset_Override(t *testing.T) {
	f := writeConfig(t, `
provider: anthropic
model: claude-sonnet-4-5
tools:
  preset: readonly
`)
	cfg, err := agent.LoadFileConfig(f)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ToolPreset() != "readonly" {
		t.Errorf("preset = %q", cfg.ToolPreset())
	}
}

func TestLoadFileConfig_InvalidYAML(t *testing.T) {
	f := writeConfig(t, `{{{not yaml`)
	_, err := agent.LoadFileConfig(f)
	if err == nil {
		t.Error("expected error for invalid YAML")
	}
}
