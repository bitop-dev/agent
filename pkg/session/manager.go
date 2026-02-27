// Package session — SessionManager: list, create, load sessions.
package session

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// DefaultSessionsDir returns the platform-appropriate directory for session files.
func DefaultSessionsDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent", "sessions")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agent", "sessions")
}

// ---------------------------------------------------------------------------
// SessionInfo — lightweight summary for listing
// ---------------------------------------------------------------------------

// Info is a lightweight summary of a session, used for listing sessions.
type Info struct {
	ID           string    // session UUID (full)
	Path         string    // absolute path to the JSONL file
	CWD          string    // working directory at creation
	Created      time.Time // parsed from the header timestamp
	MessageCount int       // number of message entries
	FirstMessage string    // text of the first user message (truncated)
}

// List returns summary info for all sessions in dir, sorted newest-first.
func List(dir string) ([]Info, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("session list: %w", err)
	}

	var infos []Info
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		path := filepath.Join(dir, e.Name())
		info, err := readInfo(path)
		if err != nil {
			continue // skip malformed files
		}
		infos = append(infos, info)
	}

	sort.Slice(infos, func(i, j int) bool {
		return infos[i].Created.After(infos[j].Created)
	})
	return infos, nil
}

func readInfo(path string) (Info, error) {
	f, err := os.Open(path)
	if err != nil {
		return Info{}, err
	}
	defer f.Close()

	var info Info
	info.Path = path

	data, err := os.ReadFile(path)
	if err != nil {
		return Info{}, err
	}

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
				info.ID = h.ID
				info.CWD = h.CWD
				if t, err := time.Parse(time.RFC3339, h.Timestamp); err == nil {
					info.Created = t
				}
			}
		case EntryTypeMessage:
			info.MessageCount++
			if info.FirstMessage == "" {
				var e MessageEntry
				if err := json.Unmarshal(raw, &e); err == nil && e.Role == "user" {
					info.FirstMessage = extractFirstTextFromLine(line)
				}
			}
		}
	}

	if info.ID == "" {
		return Info{}, fmt.Errorf("no session header in %s", path)
	}
	return info, nil
}

// extractFirstTextFromLine extracts the first text snippet from a raw MessageEntry line.
func extractFirstTextFromLine(line string) string {
	var probe struct {
		Message struct {
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
		} `json:"message"`
	}
	if err := json.Unmarshal([]byte(line), &probe); err != nil {
		return ""
	}
	for _, c := range probe.Message.Content {
		if c.Type == "text" && c.Text != "" {
			if len(c.Text) > 80 {
				return c.Text[:77] + "..."
			}
			return c.Text
		}
	}
	return ""
}
