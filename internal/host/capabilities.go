package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	internalpolicy "github.com/ncecere/agent/internal/policy"
	profileloader "github.com/ncecere/agent/internal/profile"
	"github.com/ncecere/agent/internal/registry"
	internalruntime "github.com/ncecere/agent/internal/runtime"
	"github.com/ncecere/agent/pkg/approval"
	"github.com/ncecere/agent/pkg/events"
	pkghost "github.com/ncecere/agent/pkg/host"
	pkgruntime "github.com/ncecere/agent/pkg/runtime"
	"github.com/ncecere/agent/pkg/tool"
	"github.com/ncecere/agent/pkg/workspace"
)

// RuntimeCapabilities implements pkg/host.Capabilities.
// This is the bounded controlled boundary between privileged plugins and the runtime.
type RuntimeCapabilities struct {
	Profiles     profileloader.Loader
	Tools        *registry.ToolRegistry
	Providers    *registry.ProviderRegistry
	Prompts      *registry.PromptRegistry
	DefaultCWD   string
	MaxDepth     int
	currentDepth int
}

func (c *RuntimeCapabilities) SpawnSubRun(ctx context.Context, req pkghost.SubRunRequest) (pkghost.SubRunResult, error) {
	if c.MaxDepth > 0 && c.currentDepth >= c.MaxDepth {
		return pkghost.SubRunResult{}, fmt.Errorf("spawn-sub-agent: max depth %d reached", c.MaxDepth)
	}
	profileRef := req.Profile
	if profileRef == "" {
		profileRef = "readonly"
	}
	manifest, profilePath, err := c.Profiles.Load(ctx, profileRef)
	if err != nil {
		return pkghost.SubRunResult{}, fmt.Errorf("spawn-sub-agent: load profile %q: not found in configured profile directories; pass an absolute path or install the profile to ~/.agent/profiles/", profileRef)
	}
	providerImpl, ok := c.Providers.Get(manifest.Spec.Provider.Default)
	if !ok {
		return pkghost.SubRunResult{}, fmt.Errorf("spawn-sub-agent: provider %q not registered", manifest.Spec.Provider.Default)
	}
	enabled := manifest.Spec.Tools.Enabled
	if len(req.AllowedTools) > 0 {
		enabled = intersect(enabled, req.AllowedTools)
	}
	toolsForRun, err := resolveTools(c.Tools, enabled)
	if err != nil {
		return pkghost.SubRunResult{}, fmt.Errorf("spawn-sub-agent: %w", err)
	}
	workspaceRef, _ := workspace.Resolve(c.DefaultCWD)
	policyEngine := internalpolicy.Engine{
		Workspace: workspaceRef,
		ReadOnly:  internalpolicy.IsReadOnly(manifest.Spec.Workspace.WriteScope, manifest.Spec.Tools.Enabled),
	}
	// Sub-agents always use deny-all approval to prevent runaway nested approvals.
	approvalResolver := denyAllResolver{}
	eventSink := events.NopSink{}
	runner := internalruntime.Runner{}
	runReq := pkgruntime.RunRequest{
		Prompt:       req.Task,
		SystemPrompt: loadSystemPrompt(profilePath, manifest.Spec.Instructions.System, c.Prompts),
		Profile:      manifest,
		Provider:     providerImpl,
		Tools:        toolsForRun,
		Policy:       policyEngine,
		Approvals:    approvalResolver,
		Events:       eventSink,
		Execution: pkgruntime.ExecutionContext{
			CWD:        c.DefaultCWD,
			ProfileRef: profilePath,
			Workspace:  workspaceRef,
		},
	}
	result, err := runner.Run(ctx, runReq)
	if err != nil {
		return pkghost.SubRunResult{}, err
	}
	return pkghost.SubRunResult{
		Output:    result.Output,
		SessionID: result.SessionID,
	}, nil
}

// SpawnSubRunParallel runs multiple sub-agent tasks concurrently.
// Results are returned in the same order as the input requests.
// Individual sub-agent errors are collected and returned alongside results.
func (c *RuntimeCapabilities) SpawnSubRunParallel(ctx context.Context, reqs []pkghost.SubRunRequest) ([]pkghost.SubRunResult, []error) {
	results := make([]pkghost.SubRunResult, len(reqs))
	errs := make([]error, len(reqs))
	var wg sync.WaitGroup
	for i, req := range reqs {
		wg.Add(1)
		go func(i int, req pkghost.SubRunRequest) {
			defer wg.Done()
			results[i], errs[i] = c.SpawnSubRun(ctx, req)
		}(i, req)
	}
	wg.Wait()
	return results, errs
}

type denyAllResolver struct{}

func (denyAllResolver) Resolve(_ context.Context, req approval.Request) (approval.Decision, error) {
	return approval.Decision{Approved: false, Reason: "sub-agent approvals are disabled"}, nil
}

func intersect(a, b []string) []string {
	set := make(map[string]struct{}, len(b))
	for _, v := range b {
		set[v] = struct{}{}
	}
	var out []string
	for _, v := range a {
		if _, ok := set[v]; ok {
			out = append(out, v)
		}
	}
	return out
}

func resolveTools(reg *registry.ToolRegistry, ids []string) ([]tool.Tool, error) {
	out := make([]tool.Tool, 0, len(ids))
	for _, id := range ids {
		t, ok := reg.Get(id)
		if !ok {
			return nil, fmt.Errorf("tool %q is not registered", id)
		}
		out = append(out, t)
	}
	return out, nil
}

func loadSystemPrompt(profilePath string, refs []string, prompts *registry.PromptRegistry) string {
	baseDir := filepath.Dir(profilePath)
	chunks := make([]string, 0, len(refs))
	for _, ref := range refs {
		// 1. Try as a registered plugin prompt ID.
		if prompts != nil {
			if asset, ok := prompts.Get(ref); ok && asset.Path != "" {
				if data, err := os.ReadFile(asset.Path); err == nil {
					chunks = append(chunks, strings.TrimSpace(string(data)))
					continue
				}
			}
		}
		// 2. Try as a file path relative to the profile directory.
		candidate := ref
		if !filepath.IsAbs(candidate) {
			candidate = filepath.Join(baseDir, candidate)
		}
		if data, err := os.ReadFile(candidate); err == nil {
			chunks = append(chunks, strings.TrimSpace(string(data)))
			continue
		}
		// 3. Use as inline literal text.
		chunks = append(chunks, ref)
	}
	return strings.Join(chunks, "\n\n")
}
