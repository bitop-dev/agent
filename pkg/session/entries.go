// Package session — JSONL session file entry types.
package session

import (
	"encoding/json"
	"fmt"
	"time"
)

const currentVersion = 1

// EntryType identifies the kind of JSONL line.
type EntryType string

const (
	EntryTypeSession    EntryType = "session"
	EntryTypeMessage    EntryType = "message"
	EntryTypeCompaction EntryType = "compaction"
	EntryTypeBranch     EntryType = "branch"
)

// ---------------------------------------------------------------------------
// Header (first line of every session file)
// ---------------------------------------------------------------------------

// Header is the first line written to every session file.
type Header struct {
	Type      EntryType `json:"type"`      // "session"
	ID        string    `json:"id"`        // session UUID
	Version   int       `json:"version"`   // format version
	Timestamp string    `json:"timestamp"` // ISO 8601
	CWD       string    `json:"cwd"`       // working directory at creation
}

// ---------------------------------------------------------------------------
// MessageEntry
// ---------------------------------------------------------------------------

// MessageEntry records one complete message in the conversation.
type MessageEntry struct {
	Type      EntryType       `json:"type"`      // "message"
	ID        string          `json:"id"`        // entry UUID (8 hex chars)
	ParentID  string          `json:"parent_id"` // previous entry ID
	Timestamp string          `json:"timestamp"`
	Role      string          `json:"role"`    // quick access without parsing Message
	Message   json.RawMessage `json:"message"` // serialized message (concrete type)
}

func newMessageEntry(parentID, role string, msg json.RawMessage) MessageEntry {
	return MessageEntry{
		Type:      EntryTypeMessage,
		ID:        newEntryID(),
		ParentID:  parentID,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		Role:      role,
		Message:   msg,
	}
}

// ---------------------------------------------------------------------------
// CompactionEntry
// ---------------------------------------------------------------------------

// CompactionEntry records that an LLM-generated summary replaced the early
// portion of the conversation history.
type CompactionEntry struct {
	Type             EntryType `json:"type"`               // "compaction"
	ID               string    `json:"id"`
	ParentID         string    `json:"parent_id"`
	Timestamp        string    `json:"timestamp"`
	Summary          string    `json:"summary"`
	FirstKeptEntryID string    `json:"first_kept_entry_id"` // ID of first kept MessageEntry
	TokensBefore     int       `json:"tokens_before"`
}

func newCompactionEntry(parentID, summary, firstKeptEntryID string, tokensBefore int) CompactionEntry {
	return CompactionEntry{
		Type:             EntryTypeCompaction,
		ID:               newEntryID(),
		ParentID:         parentID,
		Timestamp:        time.Now().UTC().Format(time.RFC3339),
		Summary:          summary,
		FirstKeptEntryID: firstKeptEntryID,
		TokensBefore:     tokensBefore,
	}
}

// ---------------------------------------------------------------------------
// BranchEntry
// ---------------------------------------------------------------------------

// BranchEntry is the first entry written to a forked session. It records the
// parent session file and the message entry ID at which the fork was made so
// that history viewers can traverse back to the origin.
type BranchEntry struct {
	Type               EntryType `json:"type"`      // "branch"
	ID                 string    `json:"id"`
	Timestamp          string    `json:"timestamp"`
	ParentSessionPath  string    `json:"parent_session_path"`
	ForkEntryID        string    `json:"fork_entry_id"`        // last copied message entry ID
	BranchSummary      string    `json:"branch_summary,omitempty"` // optional LLM summary of parent
}

func newBranchEntry(parentPath, forkEntryID, summary string) BranchEntry {
	return BranchEntry{
		Type:              EntryTypeBranch,
		ID:                newEntryID(),
		Timestamp:         time.Now().UTC().Format(time.RFC3339),
		ParentSessionPath: parentPath,
		ForkEntryID:       forkEntryID,
		BranchSummary:     summary,
	}
}

// ---------------------------------------------------------------------------
// Generic line parser
// ---------------------------------------------------------------------------

// ParseLine peeks at the "type" field of a JSONL line and returns the
// strongly-typed entry.
func ParseLine(line []byte) (EntryType, json.RawMessage, error) {
	var probe struct {
		Type EntryType `json:"type"`
	}
	if err := json.Unmarshal(line, &probe); err != nil {
		return "", nil, fmt.Errorf("parse entry type: %w", err)
	}
	return probe.Type, json.RawMessage(line), nil
}
