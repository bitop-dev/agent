package host

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	internalpolicy "github.com/bitop-dev/agent/internal/policy"
	profileloader "github.com/bitop-dev/agent/internal/profile"
	"github.com/bitop-dev/agent/internal/registry"
	internalruntime "github.com/bitop-dev/agent/internal/runtime"
	"github.com/bitop-dev/agent/pkg/approval"
	"github.com/bitop-dev/agent/pkg/events"
	pkghost "github.com/bitop-dev/agent/pkg/host"
	pkgruntime "github.com/bitop-dev/agent/pkg/runtime"
	"github.com/bitop-dev/agent/pkg/tool"
	"github.com/bitop-dev/agent/pkg/workspace"
)

// RuntimeCapabilities implements pkg/host.Capabilities.
// This is the bounded controlled boundary between privileged plugins and the runtime.
type RuntimeCapabilities struct {
	Profiles     profileloader.Loader
	Tools        *registry.ToolRegistry
	Providers    *registry.ProviderRegistry
	Prompts      *registry.PromptRegistry
	Events       events.Sink // parent event sink — sub-agent events are forwarded here with a prefix
	DefaultCWD   string
	MaxDepth     int
	currentDepth int
}

// subAgentSink forwards sub-agent events to the parent sink with a prefix
// so the user can see what the child is doing.
type subAgentSink struct {
	parent events.Sink
	prefix string
}

func (s subAgentSink) Publish(ctx context.Context, event events.Event) error {
	if s.parent == nil {
		return nil
	}
	switch event.Type {
	case events.TypeToolRequested:
		event.Message = fmt.Sprintf("%s → %s", s.prefix, event.Message)
	case events.TypeToolFinished:
		event.Message = fmt.Sprintf("%s → %s", s.prefix, truncateForDisplay(event.Message, 120))
	case events.TypeError:
		event.Message = fmt.Sprintf("%s → error: %s", s.prefix, event.Message)
	case events.TypeAssistantDelta:
		return nil // don't forward streaming text from sub-agents
	default:
		return nil // skip turn-started, turn-finished, etc.
	}
	return s.parent.Publish(ctx, event)
}

// formatHandoffContext converts a structured context map into a labeled text block
// that gets prepended to the sub-agent's prompt.
func formatHandoffContext(ctx map[string]any) string {
	var lines []string
	lines = append(lines, "[Context from parent agent]")
	for k, v := range ctx {
		switch val := v.(type) {
		case string:
			lines = append(lines, fmt.Sprintf("%s: %s", k, val))
		case []any:
			items := make([]string, 0, len(val))
			for _, item := range val {
				items = append(items, fmt.Sprint(item))
			}
			lines = append(lines, fmt.Sprintf("%s: %s", k, strings.Join(items, ", ")))
		default:
			lines = append(lines, fmt.Sprintf("%s: %v", k, v))
		}
	}
	return strings.Join(lines, "\n")
}

func truncateForDisplay(s string, max int) string {
	s = strings.Join(strings.Fields(strings.TrimSpace(s)), " ")
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}

func (c *RuntimeCapabilities) DiscoverAgents(ctx context.Context) ([]pkghost.AgentInfo, error) {
	discovered, err := c.Profiles.Discover(ctx)
	if err != nil {
		return nil, err
	}
	var agents []pkghost.AgentInfo
	for _, d := range discovered {
		agents = append(agents, pkghost.AgentInfo{
			Name:         d.Manifest.Metadata.Name,
			Version:      d.Manifest.Metadata.Version,
			Description:  d.Manifest.Metadata.Description,
			Capabilities: d.Manifest.Metadata.Capabilities,
			Accepts:      d.Manifest.Metadata.Accepts,
			Returns:      d.Manifest.Metadata.Returns,
			Tools:        d.Manifest.Spec.Tools.Enabled,
		})
	}
	return agents, nil
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
	// Forward sub-agent events to the parent so progress is visible.
	eventSink := subAgentSink{parent: c.Events, prefix: fmt.Sprintf("[sub:%s]", profileRef)}
	runner := internalruntime.Runner{}
	// Build the prompt: inject structured context before the task if provided.
	prompt := req.Task
	if len(req.Context) > 0 {
		prompt = formatHandoffContext(req.Context) + "\n\n" + req.Task
	}

	runReq := pkgruntime.RunRequest{
		Prompt:       prompt,
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

// RunPipeline executes a sequence of agent steps. Each step's output is stored
// under its `as` name and available to subsequent steps via {{var}} expansion.
// Steps with a Parallel field run their sub-steps concurrently.
func (c *RuntimeCapabilities) RunPipeline(ctx context.Context, steps []pkghost.PipelineStep, approver pkghost.PipelineApprover) (pkghost.PipelineResult, error) {
	outputs := make(map[string]string)
	var stepResults []pkghost.PipelineStepResult

	for _, step := range steps {
		// Checkpoint step — pause for human review.
		if step.Checkpoint != nil {
			sr := pkghost.PipelineStepResult{Agent: "checkpoint", As: step.As}
			if c.Events != nil {
				c.Events.Publish(ctx, events.Event{
					Type:    events.TypeApprovalRequest,
					Message: fmt.Sprintf("[pipeline checkpoint] %s", step.Checkpoint.Message),
				})
			}
			if approver != nil && step.Checkpoint.Requires == "approval" {
				approved, err := approver.Approve(ctx, *step.Checkpoint, outputs)
				if err != nil {
					sr.Error = err.Error()
					stepResults = append(stepResults, sr)
					return pkghost.PipelineResult{Steps: stepResults, Outputs: outputs}, fmt.Errorf("checkpoint failed: %w", err)
				}
				if !approved {
					sr.Error = "checkpoint rejected by user"
					sr.Output = "Pipeline stopped at checkpoint: " + step.Checkpoint.Message
					stepResults = append(stepResults, sr)
					return pkghost.PipelineResult{Steps: stepResults, Outputs: outputs}, nil
				}
			}
			sr.Output = "checkpoint passed: " + step.Checkpoint.Message
			stepResults = append(stepResults, sr)
			continue
		}

		// Parallel step — run sub-steps concurrently.
		if len(step.Parallel) > 0 {
			var reqs []pkghost.SubRunRequest
			for _, ps := range step.Parallel {
				reqs = append(reqs, pkghost.SubRunRequest{
					Task:     expandTemplate(ps.Task, outputs),
					Profile:  ps.Agent,
					MaxTurns: defaultMaxTurns(ps.MaxTurns),
					Context:  expandContextMap(ps.Context, outputs),
				})
			}
			results, errs := c.SpawnSubRunParallel(ctx, reqs)
			for i, ps := range step.Parallel {
				sr := pkghost.PipelineStepResult{Agent: ps.Agent, As: ps.As}
				if i < len(errs) && errs[i] != nil {
					sr.Error = errs[i].Error()
				} else if i < len(results) {
					sr.Output = results[i].Output
				}
				if ps.As != "" && sr.Error == "" {
					outputs[ps.As] = sr.Output
				}
				stepResults = append(stepResults, sr)
			}
			continue
		}

		// Sequential step — run one agent.
		task := expandTemplate(step.Task, outputs)
		stepCtx := expandContextMap(step.Context, outputs)

		result, err := c.SpawnSubRun(ctx, pkghost.SubRunRequest{
			Task:     task,
			Profile:  step.Agent,
			MaxTurns: defaultMaxTurns(step.MaxTurns),
			Context:  stepCtx,
		})

		sr := pkghost.PipelineStepResult{Agent: step.Agent, As: step.As}
		if err != nil {
			sr.Error = err.Error()
			stepResults = append(stepResults, sr)
			// Pipeline continues on error — the next step sees the error in context.
			if step.As != "" {
				outputs[step.As] = fmt.Sprintf("[error from %s: %v]", step.Agent, err)
			}
			continue
		}
		sr.Output = result.Output
		stepResults = append(stepResults, sr)
		if step.As != "" {
			outputs[step.As] = result.Output
		}
	}

	return pkghost.PipelineResult{Steps: stepResults, Outputs: outputs}, nil
}

// expandTemplate replaces {{var}} placeholders in a string with values from the outputs map.
func expandTemplate(tmpl string, outputs map[string]string) string {
	result := tmpl
	for k, v := range outputs {
		result = strings.ReplaceAll(result, "{{"+k+"}}", v)
	}
	return result
}

// expandContextMap expands {{var}} in all string values of a context map.
func expandContextMap(ctx map[string]any, outputs map[string]string) map[string]any {
	if len(ctx) == 0 {
		return nil
	}
	expanded := make(map[string]any, len(ctx))
	for k, v := range ctx {
		if s, ok := v.(string); ok {
			expanded[k] = expandTemplate(s, outputs)
		} else {
			expanded[k] = v
		}
	}
	return expanded
}

func defaultMaxTurns(n int) int {
	if n <= 0 {
		return 6
	}
	return n
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
