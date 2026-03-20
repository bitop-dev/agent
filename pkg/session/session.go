package session

import (
	"context"
	"time"

	"github.com/bitop-dev/agent/pkg/tool"
)

type Metadata struct {
	ID        string
	Profile   string
	CWD       string
	CreatedAt time.Time
	UpdatedAt time.Time
}

type EntryKind string

const (
	EntryMessage    EntryKind = "message"
	EntryEvent      EntryKind = "event"
	EntryCompaction EntryKind = "compaction" // structured summary replacing older messages
)

type Entry struct {
	Kind      EntryKind
	Role      string
	Content   string
	EventType string
	Metadata  string
	CreatedAt time.Time
}

type MessageMetadata struct {
	ToolCallID string      `json:"toolCallId,omitempty"`
	ToolName   string      `json:"toolName,omitempty"`
	ToolCalls  []tool.Call `json:"toolCalls,omitempty"`
}

type Session struct {
	Metadata Metadata
	Entries  []Entry
}

type Store interface {
	Create(ctx context.Context, meta Metadata) (Session, error)
	Load(ctx context.Context, id string) (Session, error)
	Append(ctx context.Context, id string, entry Entry) error
	MostRecent(ctx context.Context, cwd string) (Session, error)
	List(ctx context.Context, cwd string, limit int) ([]Metadata, error)
	Count(ctx context.Context, cwd string) (int, error)
}

// TaskState represents a persistent long-running task (pipeline or complex workflow).
type TaskState struct {
	ID          string            `json:"id"`
	Profile     string            `json:"profile"`
	Prompt      string            `json:"prompt"`
	Status      string            `json:"status"` // "running", "paused", "completed", "failed"
	CurrentStep int               `json:"currentStep"`
	TotalSteps  int               `json:"totalSteps"`
	Outputs     map[string]string `json:"outputs"`
	SessionID   string            `json:"sessionId"`
	CWD         string            `json:"cwd"`
	CreatedAt   time.Time         `json:"createdAt"`
	UpdatedAt   time.Time         `json:"updatedAt"`
}

func NewID(now time.Time) string {
	return now.UTC().Format("20060102T150405.000000000")
}
