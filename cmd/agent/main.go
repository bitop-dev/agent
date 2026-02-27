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
	"strings"
	"syscall"
	"time"

	"github.com/nickcecere/agent/pkg/agent"
	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/ai/providers/anthropic"
	"github.com/nickcecere/agent/pkg/ai/providers/openai"
	"github.com/nickcecere/agent/pkg/tools"
	"github.com/nickcecere/agent/pkg/tools/builtin"
)

func main() {
	configPath := flag.String("config", "agent.yaml", "path to agent config file")
	oneShot := flag.String("prompt", "", "one-shot prompt (non-interactive)")
	cwdFlag := flag.String("cwd", "", "override working directory for file tools")
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
		var err error
		cwd, err = os.Getwd()
		if err != nil {
			fatalf("getwd: %v", err)
		}
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

	// Build agent
	ag := agent.New(agent.Options{
		SystemPrompt: cfg.SystemPrompt,
		Model:        cfg.Model,
		Provider:     provider,
		Tools:        registry,
	})

	// Subscribe to events for terminal output
	unsub := ag.Subscribe(makeEventPrinter())
	defer unsub()

	// Build call config
	callCfg := agent.Config{
		StreamOptions: ai.StreamOptions{
			APIKey:      cfg.APIKey,
			MaxTokens:   cfg.MaxTokens,
			Temperature: cfg.Temperature,
		},
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
	fmt.Println("[agent] type a prompt and press enter. Commands: /clear /state exit")

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
			fmt.Printf("[state] messages=%d streaming=%v error=%q\n",
				len(s.Messages), s.IsStreaming, s.Error)
			continue
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
	switch cfg.Provider {
	case "openai":
		return openai.New(cfg.BaseURL), nil
	case "anthropic":
		return anthropic.New(cfg.BaseURL), nil
	case "openrouter", "groq", "azure", "ollama", "lmstudio":
		if cfg.BaseURL == "" {
			return nil, fmt.Errorf("provider %q requires base_url", cfg.Provider)
		}
		return openai.New(cfg.BaseURL), nil
	default:
		if cfg.BaseURL != "" {
			return openai.New(cfg.BaseURL), nil
		}
		return nil, fmt.Errorf("unknown provider %q (set base_url to use as openai-compatible)", cfg.Provider)
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
