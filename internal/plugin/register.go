package plugin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	loaderutil "github.com/ncecere/agent/internal/loader"
	"github.com/ncecere/agent/internal/mcp"
	"github.com/ncecere/agent/internal/registry"
	"github.com/ncecere/agent/pkg/config"
	pkghost "github.com/ncecere/agent/pkg/host"
	plg "github.com/ncecere/agent/pkg/plugin"
	"github.com/ncecere/agent/pkg/tool"
)

type Registries struct {
	Plugins          *registry.PluginRegistry
	Tools            *registry.ToolRegistry
	Prompts          *registry.PromptRegistry
	ProfileTemplates *registry.ProfileTemplateRegistry
	Policies         *registry.PolicyRegistry
	PluginConfigs    map[string]config.PluginConfig
	HostCapabilities pkghost.Capabilities
	MCPManager       *mcp.Manager
}

func RegisterDiscovered(ctx context.Context, loader Loader, regs Registries) error {
	discovered, err := loader.Discover(ctx)
	if err != nil {
		return err
	}
	for _, item := range discovered {
		if !item.Reference.Enabled {
			continue
		}
		if err := ValidateManifest(item.Manifest); err != nil {
			return fmt.Errorf("validate plugin manifest %s: %w", item.Manifest.Metadata.Name, err)
		}
		if err := ValidateConfig(item.Manifest, regs.PluginConfigs[item.Manifest.Metadata.Name]); err != nil {
			return err
		}
		if err := registerOne(item, regs); err != nil {
			return fmt.Errorf("register plugin %s: %w", item.Manifest.Metadata.Name, err)
		}
	}
	return nil
}

func registerOne(item Discovered, regs Registries) error {
	if regs.Plugins != nil {
		if err := regs.Plugins.Register(item.Manifest); err != nil {
			return err
		}
	}
	baseDir := filepath.Dir(item.Reference.Path)
	for _, contribution := range item.Manifest.Spec.Contributes.Tools {
		if regs.Tools == nil {
			continue
		}
		descriptorPath := contribution.Path
		if descriptorPath == "" {
			descriptorPath = contribution.Entrypoint
		}
		if descriptorPath == "" {
			return fmt.Errorf("tool contribution %s missing path", contribution.ID)
		}
		descriptor, err := loaderutil.LoadYAML[plg.ToolDescriptor](filepath.Join(baseDir, descriptorPath))
		if err != nil {
			return fmt.Errorf("load tool descriptor %s: %w", contribution.ID, err)
		}
		if descriptor.ID == "" {
			descriptor.ID = contribution.ID
		}
		if _, exists := regs.Tools.Get(descriptor.ID); exists {
			continue
		}
		if err := regs.Tools.Register(DescriptorTool{PluginName: item.Manifest.Metadata.Name, Descriptor: descriptor, Runtime: item.Manifest.Spec.Runtime, Config: regs.PluginConfigs[item.Manifest.Metadata.Name], HostCaps: regs.HostCapabilities, MCPManager: regs.MCPManager, Manifest: item.Manifest}); err != nil {
			return err
		}
	}
	for _, contribution := range item.Manifest.Spec.Contributes.Prompts {
		if regs.Prompts != nil {
			if err := regs.Prompts.Register(assetRef(item, baseDir, contribution)); err != nil {
				return err
			}
		}
	}
	for _, contribution := range item.Manifest.Spec.Contributes.ProfileTemplates {
		if regs.ProfileTemplates != nil {
			if err := regs.ProfileTemplates.Register(assetRef(item, baseDir, contribution)); err != nil {
				return err
			}
		}
	}
	for _, contribution := range item.Manifest.Spec.Contributes.Policies {
		if regs.Policies != nil {
			if err := regs.Policies.Register(assetRef(item, baseDir, contribution)); err != nil {
				return err
			}
		}
	}
	return nil
}

func assetRef(item Discovered, baseDir string, contribution plg.Contribution) registry.AssetReference {
	path := contribution.Path
	if path == "" {
		path = contribution.Entrypoint
	}
	if path != "" {
		path = filepath.Join(baseDir, path)
	}
	return registry.AssetReference{PluginName: item.Manifest.Metadata.Name, ID: contribution.ID, Path: path}
}

type DescriptorTool struct {
	PluginName string
	Descriptor plg.ToolDescriptor
	Runtime    plg.Runtime
	Config     config.PluginConfig
	HostCaps   pkghost.Capabilities
	MCPManager *mcp.Manager
	Manifest   plg.Manifest
}

func (t DescriptorTool) Definition() tool.Definition {
	return tool.Definition{ID: t.Descriptor.ID, Description: t.Descriptor.Description, Schema: t.Descriptor.InputSchema}
}

func (t DescriptorTool) Run(ctx context.Context, call tool.Call) (tool.Result, error) {
	switch t.Runtime.Type {
	case plg.RuntimeHTTP:
		return t.runHTTP(ctx, call)
	case plg.RuntimeHost:
		return t.runHost(ctx, call)
	case plg.RuntimeMCP:
		return t.runMCP(ctx, call)
	default:
		return tool.Result{}, fmt.Errorf("plugin tool %s from %s is registered but runtime mode %s execution is not implemented yet", t.Descriptor.ID, t.PluginName, t.Runtime.Type)
	}
}

func (t DescriptorTool) runMCP(ctx context.Context, call tool.Call) (tool.Result, error) {
	if t.MCPManager == nil {
		return tool.Result{}, fmt.Errorf("plugin tool %s: mcp manager not configured", t.Descriptor.ID)
	}
	tools, err := t.MCPManager.Tools(ctx, t.Manifest, t.Config)
	if err != nil {
		return tool.Result{}, err
	}
	for _, mcpTool := range tools {
		if mcpTool.Definition().ID == t.Descriptor.ID {
			return mcpTool.Run(ctx, call)
		}
	}
	return tool.Result{}, fmt.Errorf("mcp tool %s not found on server", t.Descriptor.ID)
}

func (t DescriptorTool) runHost(ctx context.Context, call tool.Call) (tool.Result, error) {
	if t.HostCaps == nil {
		return tool.Result{}, fmt.Errorf("plugin tool %s requires host capabilities but none are configured", t.Descriptor.ID)
	}
	switch t.Descriptor.Execution.Operation {
	case "spawn-sub-agent":
		task, _ := call.Arguments["task"].(string)
		profile, _ := call.Arguments["profile"].(string)
		maxTurns := 4
		if mt, ok := call.Arguments["maxTurns"].(float64); ok && mt > 0 {
			maxTurns = int(mt)
		}
		result, err := t.HostCaps.SpawnSubRun(ctx, pkghost.SubRunRequest{
			Task:     task,
			Profile:  profile,
			MaxTurns: maxTurns,
		})
		if err != nil {
			return tool.Result{}, err
		}
		return tool.Result{
			ToolID: call.ToolID,
			Output: result.Output,
			Data:   map[string]any{"sessionId": result.SessionID, "turns": result.Turns},
		}, nil
	default:
		return tool.Result{}, fmt.Errorf("plugin tool %s: unsupported host operation %q", t.Descriptor.ID, t.Descriptor.Execution.Operation)
	}
}

func (t DescriptorTool) runHTTP(ctx context.Context, call tool.Call) (tool.Result, error) {
	baseURL, _ := t.Config.Config["baseURL"].(string)
	if baseURL == "" {
		baseURL = t.Runtime.Endpoint
	}
	if strings.TrimSpace(baseURL) == "" {
		return tool.Result{}, fmt.Errorf("plugin %s requires config.baseURL or runtime.endpoint for tool %s", t.PluginName, t.Descriptor.ID)
	}
	url := strings.TrimRight(baseURL, "/")
	operation := strings.TrimSpace(t.Descriptor.Execution.Operation)
	if operation != "" {
		url += "/" + strings.TrimLeft(operation, "/")
	}
	body := map[string]any{
		"plugin":    t.PluginName,
		"tool":      t.Descriptor.ID,
		"operation": t.Descriptor.Execution.Operation,
		"arguments": call.Arguments,
		"config":    t.Config.Config,
	}
	data, err := json.Marshal(body)
	if err != nil {
		return tool.Result{}, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, strings.NewReader(string(data)))
	if err != nil {
		return tool.Result{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	if token, _ := t.Config.Config["apiKey"].(string); token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return tool.Result{}, err
	}
	defer resp.Body.Close()
	responseBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return tool.Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return tool.Result{}, fmt.Errorf("plugin %s HTTP tool %s failed: %s: %s", t.PluginName, t.Descriptor.ID, resp.Status, strings.TrimSpace(string(responseBody)))
	}
	var decoded struct {
		Output string         `json:"output"`
		Data   map[string]any `json:"data"`
		Error  string         `json:"error"`
	}
	if err := json.Unmarshal(responseBody, &decoded); err == nil && (decoded.Output != "" || decoded.Data != nil || decoded.Error != "") {
		if decoded.Error != "" {
			return tool.Result{}, errors.New(decoded.Error)
		}
		return tool.Result{ToolID: call.ToolID, Output: decoded.Output, Data: decoded.Data}, nil
	}
	return tool.Result{ToolID: call.ToolID, Output: string(responseBody)}, nil
}
