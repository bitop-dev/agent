package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ── Agent message types ───────────────────────────────────────────────────────

// AgentMessage represents a message between agents.
type AgentMessage struct {
	ID        string         `json:"id"`
	From      string         `json:"from"`
	To        string         `json:"to"`        // agent name or "broadcast"
	Type      string         `json:"type"`       // "request", "response", "notification"
	Content   string         `json:"content"`
	Data      map[string]any `json:"data,omitempty"`
	ReplyTo   string         `json:"replyTo,omitempty"` // message ID for threading
	Timestamp string         `json:"timestamp"`
}

// ── In-memory message bus ─────────────────────────────────────────────────────

// MessageBus is a simple in-memory message queue for agent-to-agent communication.
type MessageBus struct {
	mu       sync.Mutex
	messages []AgentMessage
	nextID   int
}

// NewMessageBus creates a new message bus.
func NewMessageBus() *MessageBus {
	return &MessageBus{}
}

// Send adds a message to the bus.
func (b *MessageBus) Send(msg AgentMessage) string {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.nextID++
	msg.ID = fmt.Sprintf("msg-%d", b.nextID)
	msg.Timestamp = time.Now().UTC().Format(time.RFC3339)
	b.messages = append(b.messages, msg)
	return msg.ID
}

// Receive returns pending messages for an agent and removes them from the queue.
func (b *MessageBus) Receive(agent string) []AgentMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	var matched, remaining []AgentMessage
	for _, msg := range b.messages {
		if msg.To == agent || msg.To == "broadcast" {
			matched = append(matched, msg)
		} else {
			remaining = append(remaining, msg)
		}
	}
	b.messages = remaining
	return matched
}

// Peek returns pending messages without removing them.
func (b *MessageBus) Peek(agent string) []AgentMessage {
	b.mu.Lock()
	defer b.mu.Unlock()
	var matched []AgentMessage
	for _, msg := range b.messages {
		if msg.To == agent || msg.To == "broadcast" {
			matched = append(matched, msg)
		}
	}
	return matched
}

// Count returns the total number of messages in the bus.
func (b *MessageBus) Count() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return len(b.messages)
}

// ── HTTP handlers for the message bus ─────────────────────────────────────────

func registerMessageHandlers(mux *http.ServeMux, bus *MessageBus) {
	// POST /v1/messages — send a message
	// GET  /v1/messages?agent=<name> — receive (consume) messages for an agent
	// GET  /v1/messages?agent=<name>&peek=true — peek without consuming
	mux.HandleFunc("/v1/messages", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			var msg AgentMessage
			if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
				writeHTTPError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
				return
			}
			if strings.TrimSpace(msg.To) == "" {
				writeHTTPError(w, http.StatusBadRequest, "to is required")
				return
			}
			if strings.TrimSpace(msg.Content) == "" && len(msg.Data) == 0 {
				writeHTTPError(w, http.StatusBadRequest, "content or data is required")
				return
			}
			id := bus.Send(msg)
			writeHTTPJSON(w, http.StatusCreated, map[string]string{"id": id, "status": "delivered"})

		case http.MethodGet:
			agent := r.URL.Query().Get("agent")
			if agent == "" {
				writeHTTPError(w, http.StatusBadRequest, "agent query parameter is required")
				return
			}
			peek := r.URL.Query().Get("peek") == "true"
			var messages []AgentMessage
			if peek {
				messages = bus.Peek(agent)
			} else {
				messages = bus.Receive(agent)
			}
			writeHTTPJSON(w, http.StatusOK, map[string]any{
				"agent":    agent,
				"messages": messages,
				"count":    len(messages),
			})

		default:
			writeHTTPError(w, http.StatusMethodNotAllowed, "method not allowed")
		}
	})
}
