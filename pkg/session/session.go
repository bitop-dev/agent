// Package session manages persistent agent sessions stored as JSONL files.
//
// Each session is one JSONL file:
//   - Line 1: Header (type=session, id, version, cwd, timestamp)
//   - Lines 2+: MessageEntry or CompactionEntry (one per line)
//
// Entry IDs are 8-character hex strings (short enough to not bloat the file,
// unique enough for our purposes). The parent_id chain allows us to reconstruct
// the conversation tree and correctly apply compaction on load.
//
// Usage:
//
//	// Create new session
//	sess, _ := session.Create("~/.config/agent/sessions", ".")
//
//	// Append messages as they arrive
//	sess.AppendMessage(msg)
//
//	// Later: resume
//	sess, _ = session.Load("~/.config/agent/sessions", sessionID)
//	msgs, _ := sess.Messages()
package session

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/bitop-dev/agent/pkg/ai"
)

// Session is an open session file. All writes are append-only and safe for
// concurrent use (a single goroutine is the expected writer, but the mutex
// guards against accidental concurrent calls).
type Session struct {
	mu       sync.Mutex
	f        *os.File
	w        *bufio.Writer
	id       string
	leafID   string // ID of the last written entry
	cwd      string
	dir      string
}

// ID returns the session's UUID.
func (s *Session) ID() string { return s.id }

// CWD returns the working directory the session was created in.
func (s *Session) CWD() string { return s.cwd }

// FilePath returns the absolute path to the session's JSONL file.
func (s *Session) FilePath() string { return s.f.Name() }

// LeafID returns the ID of the most-recently written entry (useful as a
// parent reference when writing the next entry).
func (s *Session) LeafID() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.leafID
}

// ---------------------------------------------------------------------------
// Creating and loading sessions
// ---------------------------------------------------------------------------

// Create opens a new session file in dir, writes the header, and returns the
// session. cwd is the working directory at session start.
func Create(dir, cwd string) (*Session, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("session: mkdir %s: %w", dir, err)
	}

	id := uuid.New().String()
	name := fmt.Sprintf("%s-%s.jsonl",
		time.Now().UTC().Format("20060102-150405"),
		id[:8],
	)
	path := filepath.Join(dir, name)

	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return nil, fmt.Errorf("session: create %s: %w", path, err)
	}

	s := &Session{f: f, w: bufio.NewWriter(f), id: id, cwd: cwd, dir: dir}

	header := Header{
		Type:      EntryTypeSession,
		ID:        id,
		Version:   currentVersion,
		Timestamp: time.Now().UTC().Format(time.RFC3339),
		CWD:       cwd,
	}
	if err := s.writeLine(header); err != nil {
		f.Close()
		return nil, err
	}

	return s, nil
}

// Load opens an existing session file by ID prefix (first 8 chars of UUID),
// reads all existing entries to restore leafID, and returns a session ready
// for appending.
func Load(dir, idPrefix string) (*Session, error) {
	path, err := findSessionFile(dir, idPrefix)
	if err != nil {
		return nil, err
	}

	// Read existing entries to find leaf ID and session metadata.
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session: read %s: %w", path, err)
	}

	var id, cwd, leafID string
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	for _, line := range lines {
		if line == "" {
			continue
		}
		typ, raw, err := ParseLine([]byte(line))
		if err != nil {
			continue
		}
		switch typ {
		case EntryTypeSession:
			var h Header
			if err := json.Unmarshal(raw, &h); err == nil {
				id = h.ID
				cwd = h.CWD
			}
		case EntryTypeMessage:
			var e MessageEntry
			if err := json.Unmarshal(raw, &e); err == nil {
				leafID = e.ID
			}
		case EntryTypeCompaction:
			var e CompactionEntry
			if err := json.Unmarshal(raw, &e); err == nil {
				leafID = e.ID
			}
		}
	}

	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return nil, fmt.Errorf("session: open %s for append: %w", path, err)
	}

	return &Session{
		f:      f,
		w:      bufio.NewWriter(f),
		id:     id,
		cwd:    cwd,
		dir:    dir,
		leafID: leafID,
	}, nil
}

// ---------------------------------------------------------------------------
// Appending entries
// ---------------------------------------------------------------------------

// AppendMessage serialises msg and appends a MessageEntry to the session file.
// Returns the new entry ID.
func (s *Session) AppendMessage(msg ai.Message) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	raw, err := MarshalMessage(msg)
	if err != nil {
		return "", fmt.Errorf("session: marshal message: %w", err)
	}

	role := string(msg.GetRole())
	entry := newMessageEntry(s.leafID, role, raw)
	if err := s.writeLine(entry); err != nil {
		return "", err
	}

	s.leafID = entry.ID
	return entry.ID, nil
}

// AppendCompaction appends a CompactionEntry to the session file.
// summary is the LLM-generated summary text.
// firstKeptEntryID is the ID of the first MessageEntry that was kept.
// tokensBefore is the estimated token count before compaction.
func (s *Session) AppendCompaction(summary, firstKeptEntryID string, tokensBefore int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	entry := newCompactionEntry(s.leafID, summary, firstKeptEntryID, tokensBefore)
	if err := s.writeLine(entry); err != nil {
		return err
	}
	s.leafID = entry.ID
	return nil
}

// Fork creates a new session that branches from this one at keepN messages.
// All message entries up to and including keepN are copied to the new session
// file. A BranchEntry is written as the first entry so history tools can link
// back to the parent. branchSummary may be empty; pass a non-empty string
// (e.g. from GenerateSummary) to annotate what was discarded in the parent.
//
// The returned Session is ready for writing and is NOT closed by Fork.
func (s *Session) Fork(dir string, keepN int, branchSummary string) (*Session, error) {
	s.mu.Lock()
	parentPath := s.f.Name()
	s.mu.Unlock()

	// Read all message entries from the parent to find the Nth message.
	parentData, err := os.ReadFile(parentPath)
	if err != nil {
		return nil, fmt.Errorf("session fork: read parent: %w", err)
	}

	lines := strings.Split(strings.TrimSpace(string(parentData)), "\n")

	// Collect header + up to keepN message entries.
	var toKeep []string
	var forkEntryID string
	msgCount := 0

	for _, line := range lines {
		if line == "" {
			continue
		}
		typ, raw, err := ParseLine([]byte(line))
		if err != nil {
			continue
		}
		switch typ {
		case EntryTypeSession:
			// Skip — the new session gets its own header.
			continue
		case EntryTypeMessage:
			if keepN > 0 && msgCount >= keepN {
				continue // stop copying once we have keepN messages
			}
			var e MessageEntry
			if err := json.Unmarshal(raw, &e); err == nil {
				forkEntryID = e.ID
			}
			toKeep = append(toKeep, line)
			msgCount++
		case EntryTypeCompaction:
			// Copy compaction entries that fall before the keepN boundary.
			if keepN <= 0 || msgCount < keepN {
				toKeep = append(toKeep, line)
			}
		}
	}

	// Create the new session.
	child, err := Create(dir, s.cwd)
	if err != nil {
		return nil, fmt.Errorf("session fork: create child: %w", err)
	}

	// Write branch header entry first.
	branch := newBranchEntry(parentPath, forkEntryID, branchSummary)
	if err := child.writeLine(branch); err != nil {
		child.Close()
		return nil, fmt.Errorf("session fork: write branch entry: %w", err)
	}
	child.leafID = branch.ID

	// Copy kept entries verbatim.
	for _, line := range toKeep {
		if _, err := child.w.WriteString(line + "\n"); err != nil {
			child.Close()
			return nil, fmt.Errorf("session fork: copy entry: %w", err)
		}
		// Update leafID to last copied message entry.
		typ, raw, _ := ParseLine([]byte(line))
		if typ == EntryTypeMessage {
			var e MessageEntry
			if json.Unmarshal(raw, &e) == nil {
				child.leafID = e.ID
			}
		}
	}
	if err := child.w.Flush(); err != nil {
		child.Close()
		return nil, err
	}

	return child, nil
}

// Close flushes and closes the session file.
func (s *Session) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.w.Flush(); err != nil {
		return err
	}
	return s.f.Close()
}

// ---------------------------------------------------------------------------
// Loading messages back
// ---------------------------------------------------------------------------

// Messages reads all entries in the session file and reconstructs the
// conversation history, applying any compaction entries correctly.
func (s *Session) Messages() ([]ai.Message, error) {
	s.mu.Lock()
	path := s.f.Name()
	s.mu.Unlock()

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("session: read %s: %w", path, err)
	}

	return ParseMessages(data)
}

// ParseMessages parses a raw JSONL session file byte slice and returns
// the reconstructed message history. Compaction is handled: messages before
// the firstKeptEntryID are replaced by the summary injected as a user message.
func ParseMessages(data []byte) ([]ai.Message, error) {
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")

	type parsedEntry struct {
		typ        EntryType
		messageID  string
		msg        ai.Message
		compaction *CompactionEntry
	}

	var entries []parsedEntry

	for _, line := range lines {
		if line == "" {
			continue
		}
		typ, raw, err := ParseLine([]byte(line))
		if err != nil {
			continue
		}
		switch typ {
		case EntryTypeMessage:
			var e MessageEntry
			if err := json.Unmarshal(raw, &e); err != nil {
				continue
			}
			msg, err := UnmarshalMessage(e.Role, e.Message)
			if err != nil {
				continue
			}
			entries = append(entries, parsedEntry{typ: EntryTypeMessage, messageID: e.ID, msg: msg})

		case EntryTypeCompaction:
			var e CompactionEntry
			if err := json.Unmarshal(raw, &e); err != nil {
				continue
			}
			entries = append(entries, parsedEntry{typ: EntryTypeCompaction, compaction: &e})
		}
	}

	// Find the last compaction entry.
	lastCompIdx := -1
	for i, e := range entries {
		if e.typ == EntryTypeCompaction {
			lastCompIdx = i
		}
	}

	if lastCompIdx == -1 {
		// No compaction — return all messages in order.
		msgs := make([]ai.Message, 0, len(entries))
		for _, e := range entries {
			if e.typ == EntryTypeMessage {
				msgs = append(msgs, e.msg)
			}
		}
		return msgs, nil
	}

	// With compaction: inject summary, then messages from firstKeptEntryID onward.
	comp := entries[lastCompIdx].compaction
	firstKeptID := comp.FirstKeptEntryID

	// Build the summary message.
	summaryText := fmt.Sprintf(
		"The conversation history before this point was compacted into the following summary:\n\n<summary>\n%s\n</summary>",
		comp.Summary,
	)
	summaryMsg := ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: summaryText}},
		Timestamp: time.Now().UnixMilli(),
	}

	var msgs []ai.Message
	msgs = append(msgs, summaryMsg)

	// Include kept messages (from firstKeptEntryID to lastCompIdx).
	foundFirst := false
	for i := 0; i < lastCompIdx; i++ {
		e := entries[i]
		if e.typ != EntryTypeMessage {
			continue
		}
		if e.messageID == firstKeptID {
			foundFirst = true
		}
		if foundFirst {
			msgs = append(msgs, e.msg)
		}
	}

	// Include messages after the compaction entry.
	for i := lastCompIdx + 1; i < len(entries); i++ {
		if entries[i].typ == EntryTypeMessage {
			msgs = append(msgs, entries[i].msg)
		}
	}

	return msgs, nil
}

// ---------------------------------------------------------------------------
// Internal helpers
// ---------------------------------------------------------------------------

func (s *Session) writeLine(v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("session: marshal entry: %w", err)
	}
	if _, err := s.w.Write(data); err != nil {
		return fmt.Errorf("session: write: %w", err)
	}
	if err := s.w.WriteByte('\n'); err != nil {
		return fmt.Errorf("session: write newline: %w", err)
	}
	return s.w.Flush()
}

// newEntryID generates an 8-character hex entry ID from a random UUID.
func newEntryID() string {
	return uuid.New().String()[:8]
}

// findSessionFile locates a session file matching the given ID prefix.
func findSessionFile(dir, idPrefix string) (string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", fmt.Errorf("session: read dir %s: %w", dir, err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), idPrefix) && strings.HasSuffix(e.Name(), ".jsonl") {
			return filepath.Join(dir, e.Name()), nil
		}
	}
	return "", fmt.Errorf("session: no session found matching %q in %s", idPrefix, dir)
}
