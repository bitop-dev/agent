package agent

import (
	"context"
	"log/slog"
	"os"
	"sync"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
)

// EventConfigReloaded is broadcast when the config file is reloaded.
const EventConfigReloaded EventType = "config_reloaded"

// ConfigReloader watches a YAML config file and applies mutable changes to
// a running Agent. Only safe-to-change-at-runtime fields are updated:
// model, max_tokens, max_turns, thinking_level, cache_retention, temperature.
//
// Provider and tool changes are NOT applied (they require restart).
type ConfigReloader struct {
	path   string
	agent  *Agent
	logger *slog.Logger
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex
	lastMod time.Time

	// OnReload is called after a successful reload with the new config.
	OnReload func(cfg *FileConfig)
}

// NewConfigReloader creates a reloader. Call Start() to begin watching.
func NewConfigReloader(path string, agent *Agent, logger *slog.Logger) *ConfigReloader {
	if logger == nil {
		logger = defaultLogger()
	}
	return &ConfigReloader{
		path:   path,
		agent:  agent,
		logger: logger,
	}
}

// Start begins polling the config file for changes.
// Poll interval is 2 seconds. Call Stop() to end.
func (r *ConfigReloader) Start() {
	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel
	r.wg.Add(1)
	go r.poll(ctx)
}

// Stop ends the polling goroutine and waits for it to finish.
func (r *ConfigReloader) Stop() {
	if r.cancel != nil {
		r.cancel()
	}
	r.wg.Wait()
}

func (r *ConfigReloader) poll(ctx context.Context) {
	defer r.wg.Done()
	ticker := time.NewTicker(2 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.check()
		}
	}
}

func (r *ConfigReloader) check() {
	info, err := os.Stat(r.path)
	if err != nil {
		return // file may have been temporarily removed
	}

	r.mu.Lock()
	if !info.ModTime().After(r.lastMod) {
		r.mu.Unlock()
		return
	}
	r.lastMod = info.ModTime()
	r.mu.Unlock()

	cfg, err := LoadFileConfig(r.path)
	if err != nil {
		r.logger.Warn("config reload: parse error", "path", r.path, "error", err)
		return
	}

	r.apply(cfg)
}

func (r *ConfigReloader) apply(cfg *FileConfig) {
	r.agent.SetModel(cfg.Model)

	// Update stream options on the agent (for next Prompt call).
	// These are stored in agent.streamOpts and agent.compactionCfg.
	r.agent.mu.Lock()
	r.agent.streamOpts.MaxTokens = cfg.MaxTokens
	r.agent.streamOpts.Temperature = cfg.Temperature
	if cfg.ThinkingLevel != "" {
		r.agent.streamOpts.ThinkingLevel = ai.ThinkingLevel(cfg.ThinkingLevel)
	}
	if cfg.CacheRetention != "" {
		r.agent.streamOpts.CacheRetention = ai.CacheRetention(cfg.CacheRetention)
	}
	if cfg.ContextWindow > 0 {
		r.agent.compactionCfg.ContextWindow = cfg.ContextWindow
	}
	r.agent.mu.Unlock()

	r.logger.Info("config reloaded",
		"path", r.path,
		"model", cfg.Model,
		"max_tokens", cfg.MaxTokens,
	)

	r.agent.broadcast(Event{Type: EventConfigReloaded})

	if r.OnReload != nil {
		r.OnReload(cfg)
	}
}

// ReloadOnce reads the config file and applies changes immediately.
// Useful for a /reload REPL command.
func (r *ConfigReloader) ReloadOnce() error {
	cfg, err := LoadFileConfig(r.path)
	if err != nil {
		return err
	}
	r.apply(cfg)
	return nil
}
