// Binary agent is a configurable LLM agent with a plugin-based tool system.
//
// Usage:
//
//	agent [flags]
//
// Flags:
//
//	-config  path to YAML config file (default: agent.yaml)
//	-prompt  one-shot prompt (skips interactive mode)
//	-cwd     override the working directory for file tools
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/bitop-dev/agent/pkg/agent"
	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/ai/models"
	"github.com/bitop-dev/agent/pkg/ai/providers/anthropic"
	"github.com/bitop-dev/agent/pkg/ai/providers/azure"
	"github.com/bitop-dev/agent/pkg/ai/providers/bedrock"
	"github.com/bitop-dev/agent/pkg/ai/providers/google"
	"github.com/bitop-dev/agent/pkg/ai/providers/openai"
	"github.com/bitop-dev/agent/pkg/ai/providers/proxy"
	"github.com/bitop-dev/agent/pkg/prompts"
	"github.com/bitop-dev/agent/pkg/session"
	"github.com/bitop-dev/agent/pkg/skills"
	"github.com/bitop-dev/agent/pkg/tools"
	"github.com/bitop-dev/agent/pkg/tools/builtin"
)

func main() {
	configPath := flag.String("config", "agent.yaml", "path to agent config file")
	oneShot := flag.String("prompt", "", "one-shot prompt (non-interactive)")
	cwdFlag := flag.String("cwd", "", "override working directory for file tools")
	sessionFlag := flag.String("session", "", "session ID to resume (prefix match)")
	listSessions := flag.Bool("sessions", false, "list recent sessions and exit")
	flag.Parse()

	cfg, err := agent.LoadFileConfig(*configPath)
	if err != nil {
		fatalf("config: %v", err)
	}

	// Resolve working directory
	cwd := cfg.Tools.WorkDir
	if *cwdFlag != "" {
		cwd = *cwdFlag
	}
	if cwd == "" {
		cwd, err = os.Getwd()
		if err != nil {
			fatalf("getwd: %v", err)
		}
	}

	// Handle -sessions flag: list sessions and exit.
	sessDir := session.DefaultSessionsDir()
	if *listSessions {
		infos, err := session.List(sessDir)
		if err != nil {
			fatalf("sessions: %v", err)
		}
		if len(infos) == 0 {
			fmt.Println("[no sessions]")
			return
		}
		for _, info := range infos {
			fmt.Printf("%s  %-30s  msgs=%-3d  %s\n",
				info.ID[:8],
				truncate(info.CWD, 30),
				info.MessageCount,
				info.Created.Format("2006-01-02 15:04"),
			)
		}
		return
	}

	// Build provider
	provider, err := buildProvider(cfg)
	if err != nil {
		fatalf("provider: %v", err)
	}

	// Build tool registry
	registry := tools.NewRegistry()

	// Register built-in tools from preset
	preset := builtin.Preset(cfg.ToolPreset())
	builtin.Register(registry, preset, cwd)
	if preset != builtin.PresetNone {
		fmt.Printf("[agent] built-in tools: preset=%s cwd=%s\n", preset, cwd)
	}

	// Load external plugin tools
	var pluginTools []tools.Tool
	for _, pc := range cfg.Tools.Plugins {
		pt, err := tools.LoadPlugin(pc.Path, pc.Args...)
		if err != nil {
			fatalf("plugin %s: %v", pc.Path, err)
		}
		registry.Register(pt)
		pluginTools = append(pluginTools, pt)
		fmt.Printf("[agent] loaded plugin: %s\n", pt.Definition().Name)
	}
	defer func() {
		for _, pt := range pluginTools {
			tools.ClosePlugin(pt)
		}
	}()

	// Load skills and prompt templates.
	agentSkills := skills.LoadSkills(cwd)
	agentTemplates := prompts.LoadTemplates(cwd)
	if len(agentSkills) > 0 {
		fmt.Printf("[agent] loaded %d skill(s)\n", len(agentSkills))
	}
	if len(agentTemplates) > 0 {
		fmt.Printf("[agent] loaded %d prompt template(s)\n", len(agentTemplates))
	}

	// Build system prompt.
	// If the user supplied system_prompt in config, use it as the custom base.
	// Either way, we always append date/time, cwd, context files, and skills.
	systemPrompt := agent.BuildSystemPrompt(agent.SystemPromptOptions{
		CustomPrompt: cfg.SystemPrompt,
		ActiveTools:  registry.Names(),
		Cwd:          cwd,
		SkillsBlock:  skills.FormatSkillsForPrompt(agentSkills),
	})

	// Session persistence.
	streamOpts := ai.StreamOptions{
		APIKey:         cfg.APIKey,
		MaxTokens:      cfg.MaxTokens,
		Temperature:    cfg.Temperature,
		ThinkingLevel:  ai.ThinkingLevel(cfg.ThinkingLevel),
		CacheRetention: ai.CacheRetention(cfg.CacheRetention),
	}

	var sess *session.Session
	var resumeMessages []ai.Message

	if *sessionFlag != "" {
		// Resume existing session.
		sess, err = session.Load(sessDir, *sessionFlag)
		if err != nil {
			fatalf("session resume: %v", err)
		}
		resumeMessages, err = sess.Messages()
		if err != nil {
			fatalf("session load messages: %v", err)
		}
		fmt.Printf("[agent] resumed session %s (%d messages)\n", sess.ID()[:8], len(resumeMessages))
	} else {
		// Create new session.
		sess, err = session.Create(sessDir, cwd)
		if err != nil {
			// Non-fatal: agent can work without session persistence.
			fmt.Fprintf(os.Stderr, "[warn] could not create session: %v\n", err)
		} else {
			fmt.Printf("[agent] session %s\n", sess.ID()[:8])
		}
	}
	if sess != nil {
		defer sess.Close()
	}

	// Resolve context window: explicit config > model registry > 0.
	ctxWindow := cfg.ContextWindow
	if ctxWindow == 0 {
		ctxWindow = models.ContextWindowFor(cfg.Model)
	}
	if ctxWindow > 0 {
		fmt.Printf("[agent] model context window: %d tokens\n", ctxWindow)
	}

	// Compaction config.
	compactionCfg := agent.CompactionConfig{
		Enabled:          cfg.Compaction.Enabled,
		ContextWindow:    cfg.Compaction.ContextWindow,
		ReserveTokens:    cfg.Compaction.ReserveTokens,
		KeepRecentTokens: cfg.Compaction.KeepRecentTokens,
	}
	// Apply resolved context window to compaction if not set explicitly.
	if compactionCfg.ContextWindow == 0 {
		compactionCfg.ContextWindow = ctxWindow
	}

	// Build agent
	ag := agent.New(agent.Options{
		SystemPrompt:  systemPrompt,
		Model:         cfg.Model,
		Provider:      provider,
		Tools:         registry,
		Session:       sess,
		Compaction:    compactionCfg,
		StreamOptions: streamOpts,
	})

	// Load resumed messages into agent history if resuming.
	if len(resumeMessages) > 0 {
		ag.AttachSession(sess, resumeMessages)
	}

	unsub := ag.Subscribe(makeEventPrinter())
	defer unsub()

	callCfg := agent.Config{
		StreamOptions:      streamOpts,
		MaxTurns:           cfg.MaxTurns,
		MaxRetries:         cfg.MaxRetries,
		MaxToolConcurrency: cfg.MaxToolConcurrency,
		RetryBaseDelay:     time.Second,
	}

	// Auto-approve mode: skip tool confirmation for unattended operation.
	// When auto_approve is false (default) and running interactively,
	// a confirmation hook could be wired here. For now, nil = auto-approve.
	if cfg.AutoApprove {
		callCfg.ConfirmToolCall = agent.AutoApproveAll
	}

	// Handle SIGINT / SIGTERM
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigCh
		ag.Abort()
	}()

	if *oneShot != "" {
		if err := ag.Prompt(context.Background(), *oneShot, callCfg); err != nil {
			fatalf("prompt: %v", err)
		}
		return
	}

	// Interactive REPL
	fmt.Printf("[agent] provider=%s model=%s tools=%v\n",
		provider.Name(), cfg.Model, registry.Names())
	fmt.Println("[agent] type a prompt and press enter. Commands: /clear /state /session /sessions exit")

	scanner := bufio.NewScanner(os.Stdin)
	for {
		fmt.Print("> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		switch strings.ToLower(line) {
		case "exit", "quit":
			return
		case "/clear":
			ag.ClearMessages()
			fmt.Println("[cleared]")
			continue
		case "/state":
			s := ag.State()
			fmt.Printf("[state] messages=%d context_tokens=%d streaming=%v error=%q\n",
				len(s.Messages), s.ContextTokens, s.IsStreaming, s.Error)
			continue
		case "/skills":
			if len(agentSkills) == 0 {
				fmt.Println("[no skills loaded]")
			}
			for _, s := range agentSkills {
				fmt.Printf("  %-20s  %s\n", s.Name, s.Description)
			}
			continue
		case "/templates":
			if len(agentTemplates) == 0 {
				fmt.Println("[no templates loaded]")
			}
			for _, t := range agentTemplates {
				fmt.Printf("  /%-20s  %s\n", t.Name, t.Description)
			}
			continue
		case "/model":
			info := models.Lookup(cfg.Model)
			if info == nil {
				fmt.Printf("[model] %s (unknown — not in registry)\n", cfg.Model)
			} else {
				fmt.Printf("[model] %s — context=%d out=%d vision=%v thinking=%v in=$%.2f/1M out=$%.2f/1M\n",
					info.DisplayName, info.ContextWindow, info.MaxOutputTokens,
					info.SupportsVision, info.SupportsThinking,
					info.InputCostPer1M, info.OutputCostPer1M,
				)
			}
			continue
		case "/export":
			if sess == nil {
				fmt.Println("[export] no active session")
				continue
			}
			data, readErr := os.ReadFile(sess.FilePath())
			if readErr != nil {
				fmt.Fprintf(os.Stderr, "export read: %v\n", readErr)
				continue
			}
			htmlBytes, exportErr := session.ExportHTML(data, session.ExportOptions{Title: "Agent Session"})
			if exportErr != nil {
				fmt.Fprintf(os.Stderr, "export: %v\n", exportErr)
				continue
			}
			outPath := fmt.Sprintf("session-%s.html", sess.ID()[:8])
			if writeErr := os.WriteFile(outPath, htmlBytes, 0o644); writeErr != nil {
				fmt.Fprintf(os.Stderr, "export write: %v\n", writeErr)
			} else {
				fmt.Printf("[export] written to %s\n", outPath)
			}
			continue
		case "/session":
			if sess != nil {
				fmt.Printf("[session] id=%s  cwd=%s\n", sess.ID(), sess.CWD())
			} else {
				fmt.Println("[session] none (persistence disabled)")
			}
			continue
		case "/sessions":
			infos, err := session.List(sessDir)
			if err != nil {
				fmt.Fprintf(os.Stderr, "sessions: %v\n", err)
				continue
			}
			if len(infos) == 0 {
				fmt.Println("[no sessions]")
				continue
			}
			for i, info := range infos {
				if i >= 10 {
					fmt.Printf("  ... (%d more)\n", len(infos)-10)
					break
				}
				fmt.Printf("  %s  %-30s  msgs=%-3d  %s  %s\n",
					info.ID[:8],
					truncate(info.CWD, 30),
					info.MessageCount,
					info.Created.Format("01-02 15:04"),
					truncate(info.FirstMessage, 40),
				)
			}
			continue
		}

		// /fork [N] — fork session at message N (default: current head).
		if strings.HasPrefix(strings.ToLower(line), "/fork") {
			if sess == nil {
				fmt.Println("[fork] no active session")
				continue
			}
			parts := strings.Fields(line)
			msgs := ag.Messages()
			keepN := len(msgs)
			if len(parts) > 1 {
				if n, err := strconv.Atoi(parts[1]); err == nil && n > 0 && n <= len(msgs) {
					keepN = n
				}
			}

			var branchSummary string
			if discarded := msgs[keepN:]; len(discarded) > 0 {
				fmt.Printf("[fork] summarising %d discarded messages…\n", len(discarded))
				forkCtx, forkCancel := context.WithTimeout(context.Background(), 2*time.Minute)
				branchSummary, _ = agent.GenerateBranchSummary(forkCtx, provider, cfg.Model, callCfg.StreamOptions, discarded)
				forkCancel()
			}

			childSess, forkErr := sess.Fork(sessDir, keepN, branchSummary)
			if forkErr != nil {
				fmt.Fprintf(os.Stderr, "fork: %v\n", forkErr)
				continue
			}

			oldSess := sess
			sess = childSess
			ag.AttachSession(childSess, msgs[:keepN])
			_ = oldSess.Close()

			fmt.Printf("[fork] session %s (kept %d/%d messages)\n", childSess.ID()[:8], keepN, len(msgs))
			if branchSummary != "" {
				fmt.Printf("[fork] branch: %s\n", truncate(branchSummary, 120))
			}
			continue
		}

		// Expand /template-name … before sending to the agent.
		expanded := prompts.Expand(line, agentTemplates)
		if expanded != line {
			fmt.Printf("[template expanded]\n")
			line = expanded
		}

		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		if err := ag.Prompt(ctx, line, callCfg); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
		}
		cancel()
	}
}

// ---------------------------------------------------------------------------
// Provider builder
// ---------------------------------------------------------------------------

func buildProvider(cfg *agent.FileConfig) (ai.Provider, error) {
	p := cfg.Provider

	switch p {
	// ── Anthropic ──────────────────────────────────────────────────────────
	case "anthropic":
		return anthropic.New(cfg.BaseURL), nil

	// ── Google Gemini ──────────────────────────────────────────────────────
	case "google", "gemini":
		return google.New(cfg.BaseURL), nil

	// ── OpenAI (Responses API) — default for "openai" ─────────────────────
	case "openai":
		return openai.NewResponses(cfg.BaseURL), nil

	// ── OpenAI legacy Chat Completions — for proxies that don't support Responses ─
	case "openai-completions", "openai-legacy":
		return openai.New(cfg.BaseURL), nil

	// ── Azure OpenAI ───────────────────────────────────────────────────────
	case "azure", "azure-openai":
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("provider %q requires base_url (deployment endpoint)", p)
		}
		return azure.New(cfg.BaseURL, cfg.APIVersion), nil

	// ── Amazon Bedrock ─────────────────────────────────────────────────────
	case "bedrock", "amazon-bedrock":
		return bedrock.New(cfg.Region, cfg.Profile), nil

	// ── OpenAI-completions-compatible proxies with known base URLs ─────────
	case "openrouter":
		url := cfg.BaseURL
		if url == "" {
			url = "https://openrouter.ai/api/v1"
		}
		return openai.New(url), nil

	case "groq":
		url := cfg.BaseURL
		if url == "" {
			url = "https://api.groq.com/openai/v1"
		}
		return openai.New(url), nil

	case "xai", "grok":
		url := cfg.BaseURL
		if url == "" {
			url = "https://api.x.ai/v1"
		}
		return openai.New(url), nil

	case "mistral":
		url := cfg.BaseURL
		if url == "" {
			url = "https://api.mistral.ai/v1"
		}
		return openai.New(url), nil

	case "cerebras":
		url := cfg.BaseURL
		if url == "" {
			url = "https://api.cerebras.ai/v1"
		}
		return openai.New(url), nil

	case "huggingface":
		url := cfg.BaseURL
		if url == "" {
			url = "https://router.huggingface.co/v1"
		}
		return openai.New(url), nil

	// ── Anthropic-compatible proxies ───────────────────────────────────────
	case "opencode":
		url := cfg.BaseURL
		if url == "" {
			url = "https://opencode.ai/zen"
		}
		return anthropic.New(url), nil

	case "minimax":
		url := cfg.BaseURL
		if url == "" {
			url = "https://api.minimax.io/anthropic"
		}
		return anthropic.New(url), nil

	case "vercel-ai-gateway":
		url := cfg.BaseURL
		if url == "" {
			url = "https://ai-gateway.vercel.sh"
		}
		return anthropic.New(url), nil

	// ── Proxy ─────────────────────────────────────────────────────────────
	case "proxy":
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("proxy provider requires base_url")
		}
		return proxy.New(cfg.BaseURL, cfg.APIKey), nil

	// ── Generic fallback: any base_url → openai-completions ───────────────
	default:
		if cfg.BaseURL != "" {
			fmt.Printf("[agent] unknown provider %q — using OpenAI completions format with base_url\n", p)
			return openai.New(cfg.BaseURL), nil
		}
		return nil, fmt.Errorf("unknown provider %q — set base_url to use as openai-compatible", p)
	}
}

// ---------------------------------------------------------------------------
// Terminal event printer
// ---------------------------------------------------------------------------

func makeEventPrinter() func(agent.Event) {
	return func(ev agent.Event) {
		switch ev.Type {
		case agent.EventMessageUpdate:
			if ev.StreamEvent != nil && ev.StreamEvent.Type == ai.StreamEventTextDelta {
				fmt.Print(ev.StreamEvent.Delta)
			}
		case agent.EventMessageEnd:
			if ev.Message != nil && ev.Message.GetRole() == ai.RoleAssistant {
				fmt.Println()
			}
		case agent.EventToolStart:
			fmt.Printf("\n[tool] %s(%s)\n", ev.ToolName, formatArgs(ev.ToolArgs))
		case agent.EventToolEnd:
			status := "ok"
			if ev.IsError {
				status = "error"
			}
			fmt.Printf("[tool] %s → %s\n", ev.ToolName, status)
		case agent.EventCompaction:
			if ev.Compaction != nil {
				fmt.Printf("\n[compaction] removed=%d kept=%d tokens: %d→%d\n",
					ev.Compaction.MessagesRemoved,
					ev.Compaction.MessagesKept,
					ev.Compaction.TokensBefore,
					ev.Compaction.TokensAfter,
				)
			}
		case agent.EventTurnLimitReached:
			fmt.Printf("\n[agent] turn limit reached — stopping loop\n")
		case agent.EventRetry:
			fmt.Printf("\n[retry] attempt %d (delay %s): %v\n", ev.RetryAttempt, ev.RetryDelay, ev.RetryError)
		case agent.EventToolDenied:
			fmt.Printf("\n[denied] %s — tool call blocked\n", ev.ToolName)
		case agent.EventTurnEnd:
			if ev.CostUsage.TotalCost > 0 {
				fmt.Printf("[cost] turn: $%.6f  cumulative: $%.6f\n", ev.CostUsage.TotalCost, ev.CostUsage.TotalCost)
			}
		}
	}
}

func formatArgs(args map[string]any) string {
	if len(args) == 0 {
		return ""
	}
	parts := make([]string, 0, len(args))
	for k, v := range args {
		s := fmt.Sprintf("%v", v)
		if len(s) > 60 {
			s = s[:57] + "..."
		}
		parts = append(parts, fmt.Sprintf("%s=%s", k, s))
	}
	return strings.Join(parts, ", ")
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "fatal: "+format+"\n", args...)
	os.Exit(1)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-3] + "..."
}
