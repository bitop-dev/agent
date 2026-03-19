package plugin

import (
	"testing"

	"github.com/ncecere/agent/pkg/config"
	plg "github.com/ncecere/agent/pkg/plugin"
)

func TestValidateConfigRequiresFields(t *testing.T) {
	manifest := plg.Manifest{
		APIVersion: "github.com/ncecere/agent/v1",
		Kind:       "Plugin",
		Metadata:   plg.Metadata{Name: "send-email"},
		Spec: plg.Spec{
			Runtime: plg.Runtime{Type: plg.RuntimeHTTP},
			ConfigSchema: plg.Schema{
				Properties: map[string]plg.Property{
					"baseURL":  {Type: "string"},
					"provider": {Type: "string", Enum: []string{"smtp", "sendgrid"}},
				},
				Required: []string{"baseURL", "provider"},
			},
		},
	}
	if err := ValidateConfig(manifest, config.PluginConfig{Enabled: true, Config: map[string]any{"baseURL": "http://localhost"}}); err == nil {
		t.Fatal("expected missing required config to fail")
	}
	if err := ValidateConfig(manifest, config.PluginConfig{Enabled: true, Config: map[string]any{"baseURL": "http://localhost", "provider": "smtp"}}); err != nil {
		t.Fatalf("expected valid config to pass: %v", err)
	}
}

func TestValidateConfigRejectsWrongTypes(t *testing.T) {
	manifest := plg.Manifest{
		APIVersion: "github.com/ncecere/agent/v1",
		Kind:       "Plugin",
		Metadata:   plg.Metadata{Name: "web-research"},
		Spec: plg.Spec{
			Runtime: plg.Runtime{Type: plg.RuntimeHTTP},
			ConfigSchema: plg.Schema{
				Properties: map[string]plg.Property{
					"allowedDomains": {Type: "array"},
				},
			},
		},
	}
	if err := ValidateConfig(manifest, config.PluginConfig{Enabled: true, Config: map[string]any{"allowedDomains": "example.com"}}); err == nil {
		t.Fatal("expected wrong type to fail")
	}
}
