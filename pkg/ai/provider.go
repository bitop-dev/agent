package ai

import "context"

// Provider streams an LLM response for a given context.
// Events are sent to the returned channel; it is closed when the stream ends.
// The returned AssistantMessage is the final, complete message.
//
// Implementations must close the channel (and not panic) even when ctx is
// cancelled, so callers can always range over it safely.
type Provider interface {
	// Name returns the provider identifier, e.g. "openai", "anthropic".
	Name() string

	// Stream starts a streaming LLM call. It returns:
	//   - a channel of incremental events
	//   - a function that blocks until the stream is complete and returns the
	//     final AssistantMessage (or error)
	Stream(
		ctx context.Context,
		model string,
		llmCtx Context,
		opts StreamOptions,
	) (<-chan StreamEvent, func() (*AssistantMessage, error))
}
