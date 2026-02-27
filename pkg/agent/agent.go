package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/session"
	"github.com/nickcecere/agent/pkg/tools"
)

// Agent orchestrates the LLM + tool loop.
// It is safe to subscribe/unsubscribe listeners from multiple goroutines,
// but Prompt / Steer / FollowUp must not be called concurrently.
type Agent struct {
	mu           sync.RWMutex
	systemPrompt string
	model        string
	provider     ai.Provider
	tools        *tools.Registry

	messages    []ai.Message
	isStreaming  bool
	pendingCalls map[string]bool
	err          string

	listeners   map[int]func(Event)
	listenerSeq int
	listenerMu  sync.RWMutex

	abortFn   context.CancelFunc
	abortOnce sync.Once

	steeringQueue  []ai.Message
	steeringMu     sync.Mutex
	followUpQueue  []ai.Message
	followUpMu     sync.Mutex

	// Session persistence (optional).
	sess *session.Session
	// entryIDs maps message index → session entry ID, used for compaction.
	entryIDs []string

	// Compaction config.
	compactionCfg   CompactionConfig
	prevSummary     string // accumulated summary from previous compactions
	streamOpts      ai.StreamOptions
}

// Options configures a new Agent.
type Options struct {
	SystemPrompt  string
	Model         string
	Provider      ai.Provider
	Tools         *tools.Registry  // nil → empty registry
	Session       *session.Session // optional: persist conversation to file
	Compaction    CompactionConfig // optional: auto-compact when context grows
	StreamOptions ai.StreamOptions // passed to every LLM call
}

// New creates a new Agent.
func New(opts Options) *Agent {
	reg := opts.Tools
	if reg == nil {
		reg = tools.NewRegistry()
	}
	a := &Agent{
		systemPrompt:  opts.SystemPrompt,
		model:         opts.Model,
		provider:      opts.Provider,
		tools:         reg,
		pendingCalls:  make(map[string]bool),
		listeners:     make(map[int]func(Event)),
		sess:          opts.Session,
		compactionCfg: opts.Compaction,
		streamOpts:    opts.StreamOptions,
	}
	return a
}

// SetSession attaches a session for persistence. Existing session entries are
// NOT replayed; use session.ParseMessages before creating the agent to resume.
func (a *Agent) SetSession(s *session.Session) {
	a.mu.Lock()
	a.sess = s
	a.mu.Unlock()
}

// AttachSession opens or creates a session and optionally loads its messages
// into the agent's history. Call before first Prompt().
func (a *Agent) AttachSession(s *session.Session, msgs []ai.Message) {
	a.mu.Lock()
	a.sess = s
	// Build entryIDs slice (all zeros for pre-loaded messages).
	a.entryIDs = make([]string, len(msgs))
	a.messages = msgs
	a.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Configuration setters
// ---------------------------------------------------------------------------

func (a *Agent) SetSystemPrompt(s string) {
	a.mu.Lock()
	a.systemPrompt = s
	a.mu.Unlock()
}

func (a *Agent) SetModel(m string) {
	a.mu.Lock()
	a.model = m
	a.mu.Unlock()
}

func (a *Agent) SetProvider(p ai.Provider) {
	a.mu.Lock()
	a.provider = p
	a.mu.Unlock()
}

func (a *Agent) Tools() *tools.Registry {
	return a.tools
}

// ---------------------------------------------------------------------------
// Event subscriptions
// ---------------------------------------------------------------------------

// Subscribe registers a listener and returns an unsubscribe function.
func (a *Agent) Subscribe(fn func(Event)) func() {
	a.listenerMu.Lock()
	id := a.listenerSeq
	a.listenerSeq++
	a.listeners[id] = fn
	a.listenerMu.Unlock()

	return func() {
		a.listenerMu.Lock()
		delete(a.listeners, id)
		a.listenerMu.Unlock()
	}
}

func (a *Agent) broadcast(e Event) {
	a.listenerMu.RLock()
	fns := make([]func(Event), 0, len(a.listeners))
	for _, fn := range a.listeners {
		fns = append(fns, fn)
	}
	a.listenerMu.RUnlock()
	for _, fn := range fns {
		fn(e)
	}
}

// ---------------------------------------------------------------------------
// Prompt / Steer / FollowUp
// ---------------------------------------------------------------------------

// Prompt sends a new user message and runs the agent loop.
// Returns when the loop is complete (or ctx cancelled).
func (a *Agent) Prompt(ctx context.Context, text string, cfg Config) error {
	return a.PromptMessages(ctx, []ai.Message{
		ai.UserMessage{
			Role:      ai.RoleUser,
			Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
			Timestamp: time.Now().UnixMilli(),
		},
	}, cfg)
}

// PromptMessages sends one or more pre-built messages and runs the loop.
func (a *Agent) PromptMessages(ctx context.Context, msgs []ai.Message, cfg Config) error {
	if a.IsStreaming() {
		return fmt.Errorf("agent is already streaming; use Steer or FollowUp to queue messages")
	}

	loopCtx, cancel := context.WithCancel(ctx)
	a.mu.Lock()
	a.abortFn = cancel
	a.abortOnce = sync.Once{}
	a.isStreaming = true
	a.err = ""
	a.mu.Unlock()

	defer func() {
		a.mu.Lock()
		a.isStreaming = false
		a.abortFn = nil
		a.mu.Unlock()
		cancel()
	}()

	// Wire steering/follow-up hooks into config
	cfg = a.wrapConfig(cfg)

	return a.runLoop(loopCtx, msgs, cfg)
}

// Continue resumes from existing context (e.g. after an error or retry).
func (a *Agent) Continue(ctx context.Context, cfg Config) error {
	if a.IsStreaming() {
		return fmt.Errorf("agent is already streaming")
	}
	msgs := a.snapshotMessages()
	if len(msgs) == 0 {
		return fmt.Errorf("no messages to continue from")
	}
	if msgs[len(msgs)-1].GetRole() == ai.RoleAssistant {
		return fmt.Errorf("last message is assistant; nothing to continue from")
	}
	return a.PromptMessages(ctx, nil, cfg)
}

// Steer queues a message to inject after the current tool call finishes.
func (a *Agent) Steer(m ai.Message) {
	a.steeringMu.Lock()
	a.steeringQueue = append(a.steeringQueue, m)
	a.steeringMu.Unlock()
}

// SteerText queues a plain-text steering message.
func (a *Agent) SteerText(text string) {
	a.Steer(ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		Timestamp: time.Now().UnixMilli(),
	})
}

// FollowUp queues a message to process after the agent would otherwise stop.
func (a *Agent) FollowUp(m ai.Message) {
	a.followUpMu.Lock()
	a.followUpQueue = append(a.followUpQueue, m)
	a.followUpMu.Unlock()
}

// FollowUpText queues a plain-text follow-up message.
func (a *Agent) FollowUpText(text string) {
	a.FollowUp(ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}},
		Timestamp: time.Now().UnixMilli(),
	})
}

// Abort cancels the running loop.
func (a *Agent) Abort() {
	a.mu.RLock()
	fn := a.abortFn
	a.mu.RUnlock()
	if fn != nil {
		a.abortOnce.Do(fn)
	}
}

// ---------------------------------------------------------------------------
// State accessors
// ---------------------------------------------------------------------------

func (a *Agent) IsStreaming() bool {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.isStreaming
}

func (a *Agent) State() State {
	a.mu.RLock()
	defer a.mu.RUnlock()
	msgs := make([]ai.Message, len(a.messages))
	copy(msgs, a.messages)
	pending := make(map[string]bool, len(a.pendingCalls))
	for k, v := range a.pendingCalls {
		pending[k] = v
	}
	usage := EstimateContextTokens(msgs)
	return State{
		SystemPrompt:     a.systemPrompt,
		Model:            a.model,
		Provider:         a.provider.Name(),
		Messages:         msgs,
		IsStreaming:       a.isStreaming,
		PendingToolCalls:  pending,
		Error:            a.err,
		ContextTokens:    usage.Tokens,
	}
}

// Messages returns a snapshot of the full conversation history.
func (a *Agent) Messages() []ai.Message {
	return a.snapshotMessages()
}

// ClearMessages resets conversation history.
func (a *Agent) ClearMessages() {
	a.mu.Lock()
	a.messages = nil
	a.mu.Unlock()
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (a *Agent) appendMsg(m ai.Message) {
	// Normalise: dereference pointer types so all stored messages are values.
	// Providers (e.g. streaming loop) return *AssistantMessage.
	m = derefMessage(m)
	a.mu.Lock()
	a.messages = append(a.messages, m)
	var entryID string
	if a.sess != nil {
		var err error
		entryID, err = a.sess.AppendMessage(m)
		if err != nil {
			// Non-fatal: log to stderr but don't fail the agent.
			fmt.Printf("session: write error: %v\n", err)
		}
	}
	a.entryIDs = append(a.entryIDs, entryID)
	a.mu.Unlock()
}

// maybeCompact checks whether compaction should run and, if so, replaces the
// message history with a summary + kept messages. It records the compaction
// entry in the session file.
func (a *Agent) maybeCompact(ctx context.Context) error {
	if !a.compactionCfg.Enabled || a.compactionCfg.ContextWindow <= 0 {
		return nil
	}

	a.mu.RLock()
	msgs := make([]ai.Message, len(a.messages))
	copy(msgs, a.messages)
	entryIDs := make([]string, len(a.entryIDs))
	copy(entryIDs, a.entryIDs)
	prevSummary := a.prevSummary
	a.mu.RUnlock()

	usage := EstimateContextTokens(msgs)
	if !ShouldCompact(usage.Tokens, a.compactionCfg) {
		return nil
	}

	result, err := runCompaction(ctx, a.provider, a.model, a.streamOpts, msgs, a.compactionCfg, prevSummary)
	if err != nil {
		return fmt.Errorf("compaction: %w", err)
	}
	if result == nil {
		return nil // nothing compacted
	}

	// Find the session entry ID of the first kept message.
	cutIdx := len(result.summarisedMessages)
	firstKeptEntryID := ""
	if a.sess != nil && cutIdx < len(entryIDs) {
		firstKeptEntryID = entryIDs[cutIdx]
	}

	// Record compaction in session.
	if a.sess != nil {
		if err := a.sess.AppendCompaction(result.summary, firstKeptEntryID, result.tokensBefore); err != nil {
			fmt.Printf("session: compaction write error: %v\n", err)
		}
	}

	// Rebuild entryIDs for the new message list:
	// [1 empty ID for summary, kept IDs...]
	newEntryIDs := make([]string, 1+len(result.keptMessages))
	copy(newEntryIDs[1:], entryIDs[cutIdx:cutIdx+len(result.keptMessages)])

	a.mu.Lock()
	a.messages = result.newMessages
	a.entryIDs = newEntryIDs
	a.prevSummary = result.summary
	a.mu.Unlock()

	a.broadcast(Event{Type: EventCompaction, Compaction: &CompactionEvent{
		Summary:          result.summary,
		MessagesRemoved:  len(result.summarisedMessages),
		MessagesKept:     len(result.keptMessages),
		TokensBefore:     result.tokensBefore,
		TokensAfter:      EstimateContextTokens(result.newMessages).Tokens,
	}})

	return nil
}

// derefMessage unwraps pointer message types to their value form.
// All concrete types (UserMessage, AssistantMessage, ToolResultMessage) define
// GetRole on value receivers, so both *T and T implement ai.Message. We
// normalise to values to keep type assertions simple throughout the codebase.
func derefMessage(m ai.Message) ai.Message {
	switch p := m.(type) {
	case *ai.UserMessage:
		return *p
	case *ai.AssistantMessage:
		return *p
	case *ai.ToolResultMessage:
		return *p
	}
	return m
}

func (a *Agent) snapshotMessages() []ai.Message {
	a.mu.RLock()
	defer a.mu.RUnlock()
	out := make([]ai.Message, len(a.messages))
	copy(out, a.messages)
	return out
}

func (a *Agent) collectNew() []ai.Message {
	return a.snapshotMessages()
}

// wrapConfig injects the agent's steering/follow-up queues into the config.
func (a *Agent) wrapConfig(cfg Config) Config {
	if cfg.GetSteeringMessages == nil {
		cfg.GetSteeringMessages = func() ([]ai.Message, error) {
			a.steeringMu.Lock()
			defer a.steeringMu.Unlock()
			if len(a.steeringQueue) == 0 {
				return nil, nil
			}
			first := a.steeringQueue[0]
			a.steeringQueue = a.steeringQueue[1:]
			return []ai.Message{first}, nil
		}
	}
	if cfg.GetFollowUpMessages == nil {
		cfg.GetFollowUpMessages = func() ([]ai.Message, error) {
			a.followUpMu.Lock()
			defer a.followUpMu.Unlock()
			if len(a.followUpQueue) == 0 {
				return nil, nil
			}
			first := a.followUpQueue[0]
			a.followUpQueue = a.followUpQueue[1:]
			return []ai.Message{first}, nil
		}
	}
	return cfg
}
