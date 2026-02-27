// Package session — message serialisation/deserialisation.
//
// ai.Message is an interface; its content fields contain ai.ContentBlock,
// also an interface. Standard json.Unmarshal cannot deserialise these without
// help. This file provides MarshalMessage / UnmarshalMessage that handle the
// full type set.
package session

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
)

// ---------------------------------------------------------------------------
// Content block serialisation
// ---------------------------------------------------------------------------

// rawBlock is a flat representation of any ContentBlock, used for both
// marshalling (each concrete type naturally fits) and unmarshalling (we peek
// at "type" then decode).
type rawBlock struct {
	// Common discriminator
	Type string `json:"type"`

	// TextContent / ThinkingContent
	Text     string `json:"text,omitempty"`
	Thinking string `json:"thinking,omitempty"`

	// ImageContent
	Data     string `json:"data,omitempty"`
	MIMEType string `json:"mime_type,omitempty"`

	// ToolCall
	ID        string         `json:"id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Arguments map[string]any `json:"arguments,omitempty"`
}

func marshalBlocks(blocks []ai.ContentBlock) (json.RawMessage, error) {
	raws := make([]rawBlock, 0, len(blocks))
	for _, b := range blocks {
		switch c := b.(type) {
		case ai.TextContent:
			raws = append(raws, rawBlock{Type: "text", Text: c.Text})
		case ai.ThinkingContent:
			raws = append(raws, rawBlock{Type: "thinking", Thinking: c.Thinking})
		case ai.ImageContent:
			raws = append(raws, rawBlock{Type: "image", Data: c.Data, MIMEType: c.MIMEType})
		case ai.ToolCall:
			raws = append(raws, rawBlock{Type: "tool_call", ID: c.ID, Name: c.Name, Arguments: c.Arguments})
		}
	}
	return json.Marshal(raws)
}

func unmarshalBlocks(raw json.RawMessage) ([]ai.ContentBlock, error) {
	var raws []rawBlock
	if err := json.Unmarshal(raw, &raws); err != nil {
		return nil, err
	}
	blocks := make([]ai.ContentBlock, 0, len(raws))
	for _, r := range raws {
		switch r.Type {
		case "text":
			blocks = append(blocks, ai.TextContent{Type: "text", Text: r.Text})
		case "thinking":
			blocks = append(blocks, ai.ThinkingContent{Type: "thinking", Thinking: r.Thinking})
		case "image":
			blocks = append(blocks, ai.ImageContent{Type: "image", Data: r.Data, MIMEType: r.MIMEType})
		case "tool_call":
			blocks = append(blocks, ai.ToolCall{Type: "tool_call", ID: r.ID, Name: r.Name, Arguments: r.Arguments})
		}
	}
	return blocks, nil
}

// ---------------------------------------------------------------------------
// Message wire types (concrete, fully serialisable)
// ---------------------------------------------------------------------------

type wireUserMessage struct {
	Role      string          `json:"role"`
	Content   json.RawMessage `json:"content"` // []rawBlock
	Timestamp int64           `json:"timestamp"`
}

type wireAssistantMessage struct {
	Role         string          `json:"role"`
	Content      json.RawMessage `json:"content"` // []rawBlock
	Model        string          `json:"model"`
	Provider     string          `json:"provider"`
	Usage        ai.Usage        `json:"usage"`
	StopReason   ai.StopReason   `json:"stop_reason"`
	ErrorMessage string          `json:"error_message,omitempty"`
	Timestamp    int64           `json:"timestamp"`
}

type wireToolResultMessage struct {
	Role       string          `json:"role"`
	ToolCallID string          `json:"tool_call_id"`
	ToolName   string          `json:"tool_name"`
	Content    json.RawMessage `json:"content"` // []rawBlock
	IsError    bool            `json:"is_error"`
	Timestamp  int64           `json:"timestamp"`
}

// ---------------------------------------------------------------------------
// MarshalMessage / UnmarshalMessage
// ---------------------------------------------------------------------------

// MarshalMessage serialises any ai.Message to JSON.
func MarshalMessage(m ai.Message) (json.RawMessage, error) {
	// Dereference pointer types — providers return *AssistantMessage.
	switch p := m.(type) {
	case *ai.UserMessage:
		return MarshalMessage(*p)
	case *ai.AssistantMessage:
		return MarshalMessage(*p)
	case *ai.ToolResultMessage:
		return MarshalMessage(*p)
	}

	switch msg := m.(type) {
	case ai.UserMessage:
		cb, err := marshalBlocks(msg.Content)
		if err != nil {
			return nil, err
		}
		return json.Marshal(wireUserMessage{
			Role:      "user",
			Content:   cb,
			Timestamp: msg.Timestamp,
		})

	case ai.AssistantMessage:
		cb, err := marshalBlocks(msg.Content)
		if err != nil {
			return nil, err
		}
		return json.Marshal(wireAssistantMessage{
			Role:         "assistant",
			Content:      cb,
			Model:        msg.Model,
			Provider:     string(msg.Provider),
			Usage:        msg.Usage,
			StopReason:   msg.StopReason,
			ErrorMessage: msg.ErrorMessage,
			Timestamp:    msg.Timestamp,
		})

	case ai.ToolResultMessage:
		cb, err := marshalBlocks(msg.Content)
		if err != nil {
			return nil, err
		}
		return json.Marshal(wireToolResultMessage{
			Role:       "tool_result",
			ToolCallID: msg.ToolCallID,
			ToolName:   msg.ToolName,
			Content:    cb,
			IsError:    msg.IsError,
			Timestamp:  msg.Timestamp,
		})

	default:
		return nil, fmt.Errorf("session: unknown message type %T", m)
	}
}

// UnmarshalMessage deserialises a JSON blob into an ai.Message.
// role is provided separately (it is also inside the JSON, but we pass it
// explicitly to avoid a double-parse in the hot path).
func UnmarshalMessage(role string, data json.RawMessage) (ai.Message, error) {
	switch role {
	case "user":
		var w wireUserMessage
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, err
		}
		blocks, err := unmarshalBlocks(w.Content)
		if err != nil {
			return nil, err
		}
		ts := w.Timestamp
		if ts == 0 {
			ts = time.Now().UnixMilli()
		}
		return ai.UserMessage{Role: ai.RoleUser, Content: blocks, Timestamp: ts}, nil

	case "assistant":
		var w wireAssistantMessage
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, err
		}
		blocks, err := unmarshalBlocks(w.Content)
		if err != nil {
			return nil, err
		}
		ts := w.Timestamp
		if ts == 0 {
			ts = time.Now().UnixMilli()
		}
		return ai.AssistantMessage{
			Role:         ai.RoleAssistant,
			Content:      blocks,
			Model:        w.Model,
			Provider:     w.Provider,
			Usage:        w.Usage,
			StopReason:   w.StopReason,
			ErrorMessage: w.ErrorMessage,
			Timestamp:    ts,
		}, nil

	case "tool_result":
		var w wireToolResultMessage
		if err := json.Unmarshal(data, &w); err != nil {
			return nil, err
		}
		blocks, err := unmarshalBlocks(w.Content)
		if err != nil {
			return nil, err
		}
		ts := w.Timestamp
		if ts == 0 {
			ts = time.Now().UnixMilli()
		}
		return ai.ToolResultMessage{
			Role:       ai.RoleToolResult,
			ToolCallID: w.ToolCallID,
			ToolName:   w.ToolName,
			Content:    blocks,
			IsError:    w.IsError,
			Timestamp:  ts,
		}, nil

	default:
		return nil, fmt.Errorf("session: unknown role %q", role)
	}
}
