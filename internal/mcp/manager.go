package mcp

import (
	"context"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/ncecere/agent/pkg/config"
	plg "github.com/ncecere/agent/pkg/plugin"
	"github.com/ncecere/agent/pkg/tool"
)

// Manager owns MCP client lifecycle. One client per enabled MCP plugin.
type Manager struct {
	mu      sync.Mutex
	clients map[string]*Client
}

func NewManager() *Manager {
	return &Manager{clients: make(map[string]*Client)}
}

// Tools discovers and returns tools from the MCP server for the given plugin manifest.
// The Manager starts the server on first call and reuses the client.
func (m *Manager) Tools(ctx context.Context, manifest plg.Manifest, cfg config.PluginConfig) ([]tool.Tool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	name := manifest.Metadata.Name
	client, ok := m.clients[name]
	if !ok {
		var err error
		client, err = m.start(ctx, manifest, cfg)
		if err != nil {
			return nil, fmt.Errorf("mcp plugin %s: %w", name, err)
		}
		m.clients[name] = client
	}
	infos, err := client.ListTools(ctx)
	if err != nil {
		return nil, fmt.Errorf("mcp plugin %s list tools: %w", name, err)
	}
	tools := make([]tool.Tool, 0, len(infos))
	for _, info := range infos {
		tools = append(tools, NewTool(info, client))
	}
	return tools, nil
}

// Close shuts down all managed MCP clients.
func (m *Manager) Close() {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, client := range m.clients {
		_ = client.Close()
	}
	m.clients = make(map[string]*Client)
}

func (m *Manager) start(ctx context.Context, manifest plg.Manifest, cfg config.PluginConfig) (*Client, error) {
	rt := manifest.Spec.Runtime
	command := rt.Command
	endpoint := strings.TrimSpace(rt.Endpoint)
	headers := mergeStringMaps(rt.Headers, resolveStringMap(cfg.Config, "headers"))
	envMap := mergeStringMaps(rt.Env, resolveStringMap(cfg.Config, "env"))
	// Config can override the command.
	if rawCmd, ok := cfg.Config["command"]; ok {
		switch v := rawCmd.(type) {
		case string:
			command = strings.Fields(v)
		case []any:
			command = make([]string, 0, len(v))
			for _, item := range v {
				if s, ok := item.(string); ok {
					command = append(command, s)
				}
			}
		case []string:
			command = v
		}
	}
	if rawEndpoint, ok := cfg.Config["endpoint"]; ok {
		if s, ok := rawEndpoint.(string); ok {
			endpoint = strings.TrimSpace(s)
		}
	}
	if endpoint != "" {
		return StartRemote(ctx, endpoint, headers)
	}
	if len(command) == 0 {
		return nil, fmt.Errorf("mcp plugin %s has no command or endpoint configured", manifest.Metadata.Name)
	}
	// Expand env from runtime defaults and plugin config.
	env := os.Environ()
	for k, v := range envMap {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	for k, v := range headers {
		env = append(env, fmt.Sprintf("%s=%s", k, v))
	}
	return StartStdio(ctx, command, env)
}

func resolveStringMap(cfg map[string]any, key string) map[string]string {
	raw, ok := cfg[key]
	if !ok {
		return nil
	}
	out := make(map[string]string)
	switch v := raw.(type) {
	case map[string]any:
		for key, value := range v {
			out[key] = fmt.Sprint(value)
		}
	case map[string]string:
		for key, value := range v {
			out[key] = value
		}
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func mergeStringMaps(base, override map[string]string) map[string]string {
	if len(base) == 0 && len(override) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(override))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range override {
		out[k] = v
	}
	return out
}
