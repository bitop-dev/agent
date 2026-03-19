package plugin

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ncecere/agent/internal/registry"
	"github.com/ncecere/agent/pkg/config"
	plg "github.com/ncecere/agent/pkg/plugin"
	"github.com/ncecere/agent/pkg/tool"
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
	loader := Loader{Roots: []string{filepath.Join("..", "..", "_testing", "plugins")}, Enable: func(name string) bool { return name == "send-email" }}
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
		PluginConfigs: map[string]config.PluginConfig{
			"send-email": {Enabled: true, Config: map[string]any{"baseURL": "https://example.test", "provider": "smtp"}},
		},
	})
	if err != nil {
		t.Fatalf("register discovered: %v", err)
	}
	if _, ok := pluginRegistry.Get("send-email"); !ok {
		t.Fatal("expected send-email plugin to be registered")
	}
	if _, ok := toolRegistry.Get("email/send"); !ok {
		t.Fatal("expected email/send tool to be registered")
	}
	if _, ok := promptRegistry.Get("email/style-default"); !ok {
		t.Fatal("expected email prompt to be registered")
	}
	if _, ok := profileRegistry.Get("email/assistant"); !ok {
		t.Fatal("expected email profile template to be registered")
	}
	if _, ok := policyRegistry.Get("email/default"); !ok {
		t.Fatal("expected email policy to be registered")
	}
}
