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
	if len(command) == 0 {
		return nil, fmt.Errorf("mcp plugin %s has no command configured", manifest.Metadata.Name)
	}
	// Expand env from plugin config.
	env := os.Environ()
	if envMap, ok := cfg.Config["env"].(map[string]any); ok {
		for k, v := range envMap {
			env = append(env, fmt.Sprintf("%s=%v", k, v))
		}
	}
	return StartStdio(ctx, command, env)
}
