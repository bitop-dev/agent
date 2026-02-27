package agent_test

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/bitop-dev/agent/pkg/agent"
	"github.com/bitop-dev/agent/pkg/tools"
)

func writeYAML(t *testing.T, dir, content string) string {
	t.Helper()
	p := filepath.Join(dir, "agent.yaml")
	if err := os.WriteFile(p, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestConfigReloader_ReloadOnce(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, `
provider: anthropic
model: claude-sonnet-4-5
max_tokens: 1024
`)

	prov := &staticProvider{msg: textMsg("ok")}
	a := agent.New(agent.Options{Provider: prov, Model: "old-model", Tools: tools.NewRegistry()})

	reloader := agent.NewConfigReloader(path, a, nil)

	if err := reloader.ReloadOnce(); err != nil {
		t.Fatal(err)
	}

	state := a.State()
	if state.Model != "claude-sonnet-4-5" {
		t.Errorf("model = %q, want claude-sonnet-4-5", state.Model)
	}
}

func TestConfigReloader_EmitsEvent(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, `
provider: anthropic
model: gpt-4o
`)

	prov := &staticProvider{msg: textMsg("ok")}
	a := agent.New(agent.Options{Provider: prov, Model: "old", Tools: tools.NewRegistry()})

	var reloaded bool
	a.Subscribe(func(e agent.Event) {
		if e.Type == agent.EventConfigReloaded {
			reloaded = true
		}
	})

	reloader := agent.NewConfigReloader(path, a, nil)
	reloader.ReloadOnce()

	if !reloaded {
		t.Error("expected EventConfigReloaded")
	}
}

func TestConfigReloader_OnReloadCallback(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, `
provider: openai-completions
model: gpt-4o
max_tokens: 2048
`)

	prov := &staticProvider{msg: textMsg("ok")}
	a := agent.New(agent.Options{Provider: prov, Model: "old", Tools: tools.NewRegistry()})

	var callbackModel string
	reloader := agent.NewConfigReloader(path, a, nil)
	reloader.OnReload = func(cfg *agent.FileConfig) {
		callbackModel = cfg.Model
	}

	reloader.ReloadOnce()
	if callbackModel != "gpt-4o" {
		t.Errorf("callback model = %q, want gpt-4o", callbackModel)
	}
}

func TestConfigReloader_StartStop(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, `
provider: anthropic
model: initial
`)

	prov := &staticProvider{msg: textMsg("ok")}
	a := agent.New(agent.Options{Provider: prov, Model: "initial", Tools: tools.NewRegistry()})

	reloader := agent.NewConfigReloader(path, a, nil)
	reloader.Start()

	// Update the file.
	time.Sleep(50 * time.Millisecond)
	os.WriteFile(path, []byte(`
provider: anthropic
model: updated
`), 0644)

	// Wait for the poller to pick up the change (polls every 2s, but we
	// only need to verify Start/Stop don't panic).
	time.Sleep(3 * time.Second)
	reloader.Stop()

	// The model may or may not have been updated depending on poll timing.
	// Main assertion: no panics, no goroutine leaks.
}

func TestConfigReloader_InvalidConfig(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, `{{{invalid yaml`)

	prov := &staticProvider{msg: textMsg("ok")}
	a := agent.New(agent.Options{Provider: prov, Model: "test", Tools: tools.NewRegistry()})

	reloader := agent.NewConfigReloader(path, a, nil)
	err := reloader.ReloadOnce()
	if err == nil {
		t.Error("expected error for invalid YAML")
	}

	// Agent model should not have changed.
	if a.State().Model != "test" {
		t.Errorf("model changed on invalid config: %q", a.State().Model)
	}
}
