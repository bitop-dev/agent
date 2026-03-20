package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/bitop-dev/agent/internal/registry"
	"github.com/bitop-dev/agent/pkg/config"
	plg "github.com/bitop-dev/agent/pkg/plugin"
	"github.com/bitop-dev/agent/pkg/tool"
)

func TestDescriptorToolRunHTTP(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/send-email" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("decode request: %v", err)
		}
		if body["tool"] != "email/send" {
			t.Fatalf("unexpected tool: %#v", body["tool"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"output": "email accepted",
			"data":   map[string]any{"id": "msg_123"},
		})
	}))
	defer server.Close()

	toolImpl := DescriptorTool{
		PluginName: "send-email",
		Descriptor: plg.ToolDescriptor{ID: "email/send", Description: "Send email", Execution: plg.ToolExecution{Mode: "http", Operation: "send-email"}},
		Runtime:    plg.Runtime{Type: plg.RuntimeHTTP},
		Config:     config.PluginConfig{Enabled: true, Config: map[string]any{"baseURL": server.URL}},
	}
	result, err := toolImpl.Run(context.Background(), tool.Call{ToolID: "email/send", Arguments: map[string]any{"to": "a@example.com"}})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Output != "email accepted" {
		t.Fatalf("unexpected output: %q", result.Output)
	}
	if result.Data["id"] != "msg_123" {
		t.Fatalf("unexpected data: %#v", result.Data)
	}
}

func TestRegisterDiscoveredRegistersPluginAssetsAndTools(t *testing.T) {
	// Build a self-contained test plugin in a temp dir.
	dir := t.TempDir()
	pluginDir := filepath.Join(dir, "test-plugin")
	if err := os.MkdirAll(filepath.Join(pluginDir, "tools"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "prompts"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "profiles"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(pluginDir, "policies"), 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := `apiVersion: agent/v1
kind: Plugin
metadata:
  name: test-plugin
  version: 0.1.0
  description: test
spec:
  category: asset
  runtime:
    type: asset
  contributes:
    tools:
      - id: test/tool
        path: tools/tool.yaml
    prompts:
      - id: test/prompt
        path: prompts/prompt.md
    profileTemplates:
      - id: test/profile
        path: profiles/profile.yaml
    policies:
      - id: test/policy
        path: policies/policy.yaml
  configSchema:
    type: object
    properties: {}
    required: []
  requires:
    framework: ">=0.1.0"
    plugins: []
`
	os.WriteFile(filepath.Join(pluginDir, "plugin.yaml"), []byte(manifest), 0o644)
	os.WriteFile(filepath.Join(pluginDir, "tools", "tool.yaml"), []byte("id: test/tool\ndescription: test tool\ninputSchema:\n  type: object\n  properties: {}\nexecution:\n  mode: http\n  operation: test\nrisk:\n  level: low\n"), 0o644)
	os.WriteFile(filepath.Join(pluginDir, "prompts", "prompt.md"), []byte("test prompt"), 0o644)
	os.WriteFile(filepath.Join(pluginDir, "profiles", "profile.yaml"), []byte("test profile"), 0o644)
	os.WriteFile(filepath.Join(pluginDir, "policies", "policy.yaml"), []byte("version: 1\nrules: []\n"), 0o644)

	loader := Loader{Roots: []string{dir}, Enable: func(name string) bool { return name == "test-plugin" }}
	toolRegistry := registry.NewToolRegistry()
	promptRegistry := registry.NewPromptRegistry()
	profileRegistry := registry.NewProfileTemplateRegistry()
	policyRegistry := registry.NewPolicyRegistry()
	pluginRegistry := registry.NewPluginRegistry()

	err := RegisterDiscovered(context.Background(), loader, Registries{
		Plugins:          pluginRegistry,
		Tools:            toolRegistry,
		Prompts:          promptRegistry,
		ProfileTemplates: profileRegistry,
		Policies:         policyRegistry,
		PluginConfigs:    map[string]config.PluginConfig{},
	})
	if err != nil {
		t.Fatalf("register discovered: %v", err)
	}
	if _, ok := pluginRegistry.Get("test-plugin"); !ok {
		t.Fatal("expected test-plugin to be registered")
	}
	if _, ok := toolRegistry.Get("test/tool"); !ok {
		t.Fatal("expected test/tool to be registered")
	}
	if _, ok := promptRegistry.Get("test/prompt"); !ok {
		t.Fatal("expected test/prompt to be registered")
	}
	if _, ok := profileRegistry.Get("test/profile"); !ok {
		t.Fatal("expected test/profile to be registered")
	}
	if _, ok := policyRegistry.Get("test/policy"); !ok {
		t.Fatal("expected test/policy to be registered")
	}
}

// --- command runtime tests ---

func TestDescriptorToolRunCommandArgv(t *testing.T) {
	// Use "echo" as a simple argv-template command. It prints its arguments to stdout.
	toolImpl := DescriptorTool{
		PluginName: "echo-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/echo",
			Description: "echo args",
			Execution: plg.ToolExecution{
				Mode: "command",
				Argv: []string{"hello", "{{name}}"},
			},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: []string{"echo"}},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{}},
	}

	result, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/echo",
		Arguments: map[string]any{"name": "world"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Output != "hello world" {
		t.Fatalf("expected 'hello world', got %q", result.Output)
	}
}

func TestDescriptorToolRunCommandArgvOptionalFlag(t *testing.T) {
	// When a template value is empty and preceded by a flag, both should be omitted.
	toolImpl := DescriptorTool{
		PluginName: "flag-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/flags",
			Description: "test flag omission",
			Execution: plg.ToolExecution{
				Mode: "command",
				Argv: []string{"hello", "--name", "{{name}}", "--greeting", "{{greeting}}"},
			},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: []string{"echo"}},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{}},
	}

	// name is provided, greeting is missing -> "--greeting" and "{{greeting}}" should be omitted.
	result, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/flags",
		Arguments: map[string]any{"name": "world"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Output != "hello --name world" {
		t.Fatalf("expected 'hello --name world', got %q", result.Output)
	}
}

func TestDescriptorToolRunCommandJSON(t *testing.T) {
	// Create a temporary script that reads JSON from stdin and writes JSON to stdout.
	dir := t.TempDir()
	var scriptPath string
	var command []string

	if runtime.GOOS == "windows" {
		t.Skip("skipping on windows")
	}

	scriptPath = filepath.Join(dir, "tool.sh")
	script := `#!/bin/sh
# Read stdin, extract message via python or simple approach
input=$(cat)
# Use python3 for reliable JSON handling
python3 -c "
import json, sys
req = json.loads('''$input'''.replace(\"'''\", \"\"))
msg = req.get('arguments', {}).get('message', '')
prefix = req.get('config', {}).get('prefix', '')
output = prefix + msg if prefix else msg
json.dump({'output': output, 'data': {'length': len(msg)}}, sys.stdout)
" <<PYEOF
$input
PYEOF
`
	// Actually, let's use a simpler approach with a python script directly.
	scriptPath = filepath.Join(dir, "tool.py")
	script = `import json, sys
req = json.loads(sys.stdin.read())
msg = req.get("arguments", {}).get("message", "")
prefix = req.get("config", {}).get("prefix", "")
output = (prefix + msg) if prefix else msg
json.dump({"output": output, "data": {"length": len(msg)}}, sys.stdout)
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	command = []string{"python3", scriptPath}

	toolImpl := DescriptorTool{
		PluginName: "json-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/json-echo",
			Description: "JSON echo",
			Execution: plg.ToolExecution{
				Mode:      "command",
				Operation: "echo",
				Timeout:   5,
			},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: command},
		Config: config.PluginConfig{
			Enabled: true,
			Config:  map[string]any{"prefix": "[test] "},
		},
	}

	result, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/json-echo",
		Arguments: map[string]any{"message": "hello"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Output != "[test] hello" {
		t.Fatalf("expected '[test] hello', got %q", result.Output)
	}
	if result.Data["length"] != float64(5) {
		t.Fatalf("expected length 5, got %v", result.Data["length"])
	}
}

func TestDescriptorToolRunCommandJSONError(t *testing.T) {
	// Script that returns a JSON error.
	dir := t.TempDir()
	scriptPath := filepath.Join(dir, "err.py")
	script := `import json, sys
json.dump({"error": "something went wrong"}, sys.stdout)
`
	if err := os.WriteFile(scriptPath, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	toolImpl := DescriptorTool{
		PluginName: "err-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/err",
			Description: "error test",
			Execution:   plg.ToolExecution{Mode: "command", Timeout: 5},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: []string{"python3", scriptPath}},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{}},
	}

	_, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/err",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "something went wrong") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDescriptorToolRunCommandNonZeroExit(t *testing.T) {
	toolImpl := DescriptorTool{
		PluginName: "exit-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/exit",
			Description: "nonzero exit",
			Execution:   plg.ToolExecution{Mode: "command", Argv: []string{"-c", "echo error output >&2; exit 1"}},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: []string{"sh"}},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{}},
	}

	_, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/exit",
		Arguments: map[string]any{},
	})
	if err == nil {
		t.Fatal("expected error on non-zero exit")
	}
	if !strings.Contains(err.Error(), "error output") {
		t.Fatalf("expected stderr in error, got: %v", err)
	}
}

func TestDescriptorToolRunCommandMissingCommand(t *testing.T) {
	toolImpl := DescriptorTool{
		PluginName: "no-cmd",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/no-cmd",
			Description: "missing command",
			Execution:   plg.ToolExecution{Mode: "command"},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: nil},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{}},
	}

	_, err := toolImpl.Run(context.Background(), tool.Call{ToolID: "test/no-cmd", Arguments: map[string]any{}})
	if err == nil {
		t.Fatal("expected error for missing command")
	}
	if !strings.Contains(err.Error(), "requires runtime.command") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDescriptorToolRunCommandArgvConfigTemplate(t *testing.T) {
	// Test that {{config.key}} resolves from plugin config.
	toolImpl := DescriptorTool{
		PluginName: "cfg-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/cfg",
			Description: "config template test",
			Execution: plg.ToolExecution{
				Mode: "command",
				Argv: []string{"--token", "{{config.apiToken}}", "{{message}}"},
			},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: []string{"echo"}},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{"apiToken": "secret123"}},
	}

	result, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/cfg",
		Arguments: map[string]any{"message": "hi"},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Output != "--token secret123 hi" {
		t.Fatalf("expected '--token secret123 hi', got %q", result.Output)
	}
}

func TestDescriptorToolRunCommandEnvVars(t *testing.T) {
	// Verify that config values are injected as AGENT_PLUGIN_* env vars.
	toolImpl := DescriptorTool{
		PluginName: "env-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/env",
			Description: "env test",
			Execution: plg.ToolExecution{
				Mode: "command",
				Argv: []string{"-c", "echo $AGENT_PLUGIN_MYKEY"},
			},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: []string{"sh"}},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{"myKey": "myValue"}},
	}

	result, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/env",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Output != "myValue" {
		t.Fatalf("expected 'myValue', got %q", result.Output)
	}
}

func TestExpandArgvTemplate(t *testing.T) {
	tests := []struct {
		name     string
		argv     []string
		args     map[string]any
		cfg      map[string]any
		expected []string
	}{
		{
			name:     "simple substitution",
			argv:     []string{"list", "--repo", "{{repo}}"},
			args:     map[string]any{"repo": "owner/name"},
			cfg:      nil,
			expected: []string{"list", "--repo", "owner/name"},
		},
		{
			name:     "missing optional with flag",
			argv:     []string{"list", "--state", "{{state}}", "--repo", "{{repo}}"},
			args:     map[string]any{"repo": "owner/name"},
			cfg:      nil,
			expected: []string{"list", "--repo", "owner/name"},
		},
		{
			name:     "config prefix",
			argv:     []string{"--token", "{{config.token}}", "run"},
			args:     map[string]any{},
			cfg:      map[string]any{"token": "abc123"},
			expected: []string{"--token", "abc123", "run"},
		},
		{
			name:     "all missing",
			argv:     []string{"--flag", "{{missing}}"},
			args:     map[string]any{},
			cfg:      nil,
			expected: []string{},
		},
		{
			name:     "no placeholders",
			argv:     []string{"status", "--json"},
			args:     map[string]any{},
			cfg:      nil,
			expected: []string{"status", "--json"},
		},
		{
			name:     "placeholder without preceding flag",
			argv:     []string{"{{verb}}", "something"},
			args:     map[string]any{"verb": "run"},
			cfg:      nil,
			expected: []string{"run", "something"},
		},
		{
			name:     "missing placeholder without preceding flag drops only placeholder",
			argv:     []string{"{{verb}}", "something"},
			args:     map[string]any{},
			cfg:      nil,
			expected: []string{"something"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandArgvTemplate(tt.argv, tt.args, tt.cfg)
			if len(got) != len(tt.expected) {
				t.Fatalf("length mismatch: got %v, expected %v", got, tt.expected)
			}
			for i := range got {
				if got[i] != tt.expected[i] {
					t.Fatalf("index %d: got %q, expected %q (full: %v)", i, got[i], tt.expected[i], got)
				}
			}
		})
	}
}

func TestDescriptorToolRunCommandPlainTextFallback(t *testing.T) {
	// When a JSON-stdin command returns non-JSON output, it should be treated as plain text.
	toolImpl := DescriptorTool{
		PluginName: "plain-test",
		Descriptor: plg.ToolDescriptor{
			ID:          "test/plain",
			Description: "plain text fallback",
			Execution:   plg.ToolExecution{Mode: "command", Timeout: 5},
		},
		Runtime: plg.Runtime{Type: plg.RuntimeCommand, Command: []string{"echo", "just plain text"}},
		Config:  config.PluginConfig{Enabled: true, Config: map[string]any{}},
	}

	result, err := toolImpl.Run(context.Background(), tool.Call{
		ToolID:    "test/plain",
		Arguments: map[string]any{},
	})
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if result.Output != "just plain text" {
		t.Fatalf("expected 'just plain text', got %q", result.Output)
	}
}
