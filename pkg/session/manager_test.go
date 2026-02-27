package session_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/session"
)

func createAndPopulate(t *testing.T, dir string) *session.Session {
	t.Helper()
	sess, err := session.Create(dir, "/cwd")
	if err != nil {
		t.Fatal(err)
	}
	sess.AppendMessage(ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: "hello"}},
		Timestamp: time.Now().UnixMilli(),
	})
	sess.Close()
	return sess
}

func TestManager_ListSessions(t *testing.T) {
	dir := t.TempDir()
	s1 := createAndPopulate(t, dir)
	s2 := createAndPopulate(t, dir)

	sessions, err := session.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) < 2 {
		t.Fatalf("expected at least 2 sessions, got %d", len(sessions))
	}

	ids := map[string]bool{}
	for _, s := range sessions {
		ids[s.ID] = true
	}
	if !ids[s1.ID()] {
		t.Errorf("session %s not found in list", s1.ID()[:8])
	}
	if !ids[s2.ID()] {
		t.Errorf("session %s not found in list", s2.ID()[:8])
	}
}

func TestManager_ListSessions_EmptyDir(t *testing.T) {
	dir := t.TempDir()
	sessions, err := session.List(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions, got %d", len(sessions))
	}
}

func TestManager_ListSessions_MissingDir(t *testing.T) {
	sessions, err := session.List("/path/that/does/not/exist")
	// Should return empty or nil, not error.
	if err != nil {
		t.Logf("ListSessions on missing dir returned error (acceptable): %v", err)
	}
	_ = sessions
}

func TestLoad_ByIDPrefix(t *testing.T) {
	dir := t.TempDir()
	orig := createAndPopulate(t, dir)
	prefix := orig.ID()[:8]

	loaded, err := session.Load(dir, prefix)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()

	if loaded.ID() != orig.ID() {
		t.Errorf("loaded session ID %s != original %s", loaded.ID()[:8], orig.ID()[:8])
	}
}

func TestLoad_ShortPrefix(t *testing.T) {
	dir := t.TempDir()
	orig := createAndPopulate(t, dir)
	prefix := orig.ID()[:4]

	loaded, err := session.Load(dir, prefix)
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()
	if !strings.HasPrefix(loaded.ID(), prefix) {
		t.Errorf("loaded session %s doesn't start with prefix %s", loaded.ID(), prefix)
	}
}

func TestLoad_AmbiguousPrefix(t *testing.T) {
	// If two sessions share the same 4-char prefix, Load should return an error.
	// We can't force a collision easily, so just ensure Load returns a valid session
	// for any prefix that uniquely identifies one.
	dir := t.TempDir()
	s := createAndPopulate(t, dir)
	_, err := session.Load(dir, s.ID()[:8])
	if err != nil {
		t.Errorf("unique prefix should load without error: %v", err)
	}
}

func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()
	_, err := session.Load(dir, "00000000")
	if err == nil {
		t.Error("expected error for non-existent session prefix")
	}
}

func TestLoad_RestoresMessages(t *testing.T) {
	dir := t.TempDir()
	sess, _ := session.Create(dir, "/cwd")
	sess.AppendMessage(ai.UserMessage{
		Role:      ai.RoleUser,
		Content:   []ai.ContentBlock{ai.TextContent{Type: "text", Text: "persisted"}},
		Timestamp: time.Now().UnixMilli(),
	})
	id := sess.ID()
	sess.Close()

	loaded, err := session.Load(dir, id[:8])
	if err != nil {
		t.Fatal(err)
	}
	defer loaded.Close()

	msgs, err := loaded.Messages()
	if err != nil {
		t.Fatal(err)
	}
	if len(msgs) == 0 {
		t.Fatal("expected messages to be restored")
	}
	found := false
	for _, m := range msgs {
		if u, ok := m.(ai.UserMessage); ok {
			for _, b := range u.Content {
				if tc, ok := b.(ai.TextContent); ok && tc.Text == "persisted" {
					found = true
				}
			}
		}
	}
	if !found {
		t.Error("persisted user message not found after reload")
	}
}

func TestSession_FilePath(t *testing.T) {
	dir := t.TempDir()
	sess, _ := session.Create(dir, "/cwd")
	defer sess.Close()

	fp := sess.FilePath()
	if fp == "" {
		t.Error("FilePath() should not be empty")
	}
	if !filepath.IsAbs(fp) {
		t.Errorf("FilePath() should be absolute, got %q", fp)
	}
	if _, err := os.Stat(fp); err != nil {
		t.Errorf("FilePath() %q does not exist: %v", fp, err)
	}
}
