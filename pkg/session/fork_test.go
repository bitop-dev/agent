package session

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
)

func addMsg(t *testing.T, sess *Session, role string, text string) string {
	t.Helper()
	var m ai.Message
	ts := time.Now().UnixMilli()
	switch role {
	case "user":
		m = ai.UserMessage{Role: ai.RoleUser, Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}}, Timestamp: ts}
	case "assistant":
		m = ai.AssistantMessage{Role: ai.RoleAssistant, Content: []ai.ContentBlock{ai.TextContent{Type: "text", Text: text}}, StopReason: ai.StopReasonStop, Timestamp: ts}
	}
	id, err := sess.AppendMessage(m)
	if err != nil {
		t.Fatalf("AppendMessage(%s): %v", role, err)
	}
	return id
}

func TestFork_KeepsAllMessages(t *testing.T) {
	dir := t.TempDir()
	parent, _ := Create(dir, "/cwd")
	addMsg(t, parent, "user", "q1")
	addMsg(t, parent, "assistant", "a1")
	addMsg(t, parent, "user", "q2")
	addMsg(t, parent, "assistant", "a2")
	parent.Close()

	childDir := t.TempDir()
	// Reload parent so we can fork.
	parent2, err := Load(dir, parent.ID()[:8])
	if err != nil {
		t.Fatal(err)
	}

	child, err := parent2.Fork(childDir, 4, "") // keep all 4 messages
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	child.Close()
	parent2.Close()

	msgs, err := child.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 4 {
		t.Errorf("fork kept %d messages, want 4", len(msgs))
	}
}

func TestFork_KeepN(t *testing.T) {
	dir := t.TempDir()
	parent, _ := Create(dir, "/cwd")
	addMsg(t, parent, "user", "q1")
	addMsg(t, parent, "assistant", "a1")
	addMsg(t, parent, "user", "q2")   // index 2
	addMsg(t, parent, "assistant", "a2") // index 3
	parent.Close()

	childDir := t.TempDir()
	parent2, _ := Load(dir, parent.ID()[:8])

	// Fork at 2 — only keep q1, a1.
	child, err := parent2.Fork(childDir, 2, "branch summary")
	if err != nil {
		t.Fatalf("Fork: %v", err)
	}
	child.Close()
	parent2.Close()

	msgs, err := child.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) != 2 {
		t.Errorf("fork kept %d messages, want 2", len(msgs))
	}
}

func TestFork_BranchEntryInFile(t *testing.T) {
	dir := t.TempDir()
	parent, _ := Create(dir, "/cwd")
	addMsg(t, parent, "user", "hello")
	addMsg(t, parent, "assistant", "world")
	parent.Close()

	childDir := t.TempDir()
	parent2, _ := Load(dir, parent.ID()[:8])
	child, _ := parent2.Fork(childDir, 2, "my branch summary")
	path := child.FilePath()
	child.Close()
	parent2.Close()

	data, _ := os.ReadFile(path)
	content := string(data)

	if !strings.Contains(content, `"type":"branch"`) {
		t.Error("forked session should contain a branch entry")
	}
	if !strings.Contains(content, "my branch summary") {
		t.Error("branch entry should contain the summary")
	}
	if !strings.Contains(content, parent.ID()[:8]) {
		t.Error("branch entry should reference parent session path")
	}
}

func TestFork_CanContinueAfterFork(t *testing.T) {
	dir := t.TempDir()
	parent, _ := Create(dir, "/cwd")
	addMsg(t, parent, "user", "q1")
	addMsg(t, parent, "assistant", "a1")
	parent.Close()

	childDir := t.TempDir()
	parent2, _ := Load(dir, parent.ID()[:8])
	child, err := parent2.Fork(childDir, 2, "")
	if err != nil {
		t.Fatal(err)
	}
	parent2.Close()

	// Write more messages to the child.
	addMsg(t, child, "user", "q2 in fork")
	addMsg(t, child, "assistant", "a2 in fork")
	child.Close()

	// Reload and verify.
	child2, _ := Load(childDir, child.ID()[:8])
	msgs, _ := child2.Messages()
	child2.Close()

	if len(msgs) != 4 {
		t.Fatalf("expected 4 messages in fork, got %d", len(msgs))
	}
	// Last message should be the one added after the fork.
	last := msgs[3].(ai.AssistantMessage)
	if tc := last.Content[0].(ai.TextContent); tc.Text != "a2 in fork" {
		t.Errorf("last message text = %q", tc.Text)
	}
}
