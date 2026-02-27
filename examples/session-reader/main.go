// examples/session-reader — read and display saved session files.
//
// Demonstrates:
//   - Listing sessions in the default directory
//   - Loading a session by ID prefix
//   - Iterating over messages (with compaction transparency)
//   - Exporting a session as HTML
//
// Usage:
//
//	# List sessions
//	go run ./examples/session-reader list
//
//	# Show a session
//	go run ./examples/session-reader show a3f7c9
//
//	# Export a session as HTML
//	go run ./examples/session-reader export a3f7c9 out.html
//
//	# Show raw JSONL lines
//	go run ./examples/session-reader raw a3f7c9
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/bitop-dev/agent/pkg/ai"
	"github.com/bitop-dev/agent/pkg/session"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(1)
	}

	sessionsDir := defaultSessionsDir()

	switch os.Args[1] {
	case "list":
		cmdList(sessionsDir)
	case "show":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: session-reader show <id-prefix>")
			os.Exit(1)
		}
		cmdShow(sessionsDir, os.Args[2])
	case "export":
		if len(os.Args) < 4 {
			fmt.Fprintln(os.Stderr, "usage: session-reader export <id-prefix> <output.html>")
			os.Exit(1)
		}
		cmdExport(sessionsDir, os.Args[2], os.Args[3])
	case "raw":
		if len(os.Args) < 3 {
			fmt.Fprintln(os.Stderr, "usage: session-reader raw <id-prefix>")
			os.Exit(1)
		}
		cmdRaw(sessionsDir, os.Args[2])
	default:
		usage()
		os.Exit(1)
	}
}

// ── Commands ─────────────────────────────────────────────────────────────────

func cmdList(dir string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "cannot read session dir %s: %v\n", dir, err)
		return
	}

	fmt.Printf("%-25s %-10s %s\n", "FILE", "ID", "SIZE")
	fmt.Println(strings.Repeat("─", 60))

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, _ := e.Info()
		path := filepath.Join(dir, e.Name())
		// Extract 8-char ID from filename
		id := extractID(e.Name())
		fmt.Printf("%-25s %-10s %s\n",
			e.Name(),
			id,
			formatSize(info.Size()),
		)
		_ = path
	}
}

func cmdShow(dir, idPrefix string) {
	sess, err := session.Load(dir, idPrefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer sess.Close()

	fmt.Printf("Session: %s\n", sess.ID())
	fmt.Printf("File:    %s\n", sess.FilePath())
	fmt.Println(strings.Repeat("─", 60))

	msgs, err := sess.Messages()
	if err != nil {
		fmt.Fprintln(os.Stderr, "messages:", err)
		os.Exit(1)
	}
	fmt.Printf("%d messages\n\n", len(msgs))

	for _, msg := range msgs {
		switch m := msg.(type) {
		case ai.UserMessage:
			fmt.Printf("\033[1;32m[User]\033[0m\n")
			for _, b := range m.Content {
				if tc, ok := b.(ai.TextContent); ok {
					fmt.Println(indent(tc.Text, "  "))
				}
			}

		case ai.AssistantMessage:
			fmt.Printf("\033[1;34m[Assistant]\033[0m (model: %s)\n", m.Model)
			for _, b := range m.Content {
				switch c := b.(type) {
				case ai.TextContent:
					fmt.Println(indent(c.Text, "  "))
				case ai.ThinkingContent:
					preview := c.Thinking
					if len(preview) > 120 {
						preview = preview[:120] + "..."
					}
					fmt.Printf("  \033[35m[thinking: %s]\033[0m\n", preview)
				case ai.ToolCall:
					args, _ := json.Marshal(c.Arguments)
					fmt.Printf("  \033[33m[tool_call: %s(%s)]\033[0m\n", c.Name, args)
				}
			}
			if m.Usage.TotalTokens > 0 {
				fmt.Printf("  \033[2m(tokens: in=%d out=%d total=%d)\033[0m\n",
					m.Usage.Input, m.Usage.Output, m.Usage.TotalTokens)
			}

		case ai.ToolResultMessage:
			status := "✓"
			if m.IsError {
				status = "✗"
			}
			fmt.Printf("\033[33m[ToolResult: %s %s]\033[0m\n", status, m.ToolName)
			for _, b := range m.Content {
				if tc, ok := b.(ai.TextContent); ok {
					preview := tc.Text
					if len(preview) > 200 {
						preview = preview[:200] + "..."
					}
					fmt.Println(indent(preview, "  "))
				}
			}
		}
		fmt.Println()
	}
}

func cmdExport(dir, idPrefix, outPath string) {
	sess, err := session.Load(dir, idPrefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer sess.Close()

	data, err := os.ReadFile(sess.FilePath())
	if err != nil {
		fmt.Fprintln(os.Stderr, "read session file:", err)
		os.Exit(1)
	}

	html, err := session.ExportHTML(data, session.ExportOptions{
		Title: fmt.Sprintf("Session %s — %s", sess.ID()[:8], time.Now().Format("2006-01-02")),
	})
	if err != nil {
		fmt.Fprintln(os.Stderr, "export:", err)
		os.Exit(1)
	}

	if err := os.WriteFile(outPath, []byte(html), 0644); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}

	fmt.Printf("Exported to %s (%s)\n", outPath, formatSize(int64(len(html))))
}

func cmdRaw(dir, idPrefix string) {
	sess, err := session.Load(dir, idPrefix)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	defer sess.Close()

	f, err := os.Open(sess.FilePath())
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 1<<20), 1<<20)
	line := 0
	for scanner.Scan() {
		line++
		entryType, raw, err := session.ParseLine(scanner.Bytes())
		if err != nil {
			fmt.Printf("line %d: parse error: %v\n", line, err)
			continue
		}

		// Pretty-print the JSON
		var obj any
		json.Unmarshal(raw, &obj)
		pretty, _ := json.MarshalIndent(obj, "", "  ")

		fmt.Printf("\033[2m── line %d [%s] ──\033[0m\n%s\n\n", line, entryType, pretty)
	}
}

// ── Helpers ──────────────────────────────────────────────────────────────────

func defaultSessionsDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "agent", "sessions")
	}
	home, _ := os.UserHomeDir()
	return filepath.Join(home, ".config", "agent", "sessions")
}

func extractID(filename string) string {
	// Format: YYYYMMDD-HHMMSS-<8hex>.jsonl
	parts := strings.Split(strings.TrimSuffix(filename, ".jsonl"), "-")
	if len(parts) >= 3 {
		return parts[len(parts)-1]
	}
	return filename
}

func indent(s, prefix string) string {
	lines := strings.Split(s, "\n")
	for i, l := range lines {
		lines[i] = prefix + l
	}
	return strings.Join(lines, "\n")
}

func formatSize(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1fMB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%.1fKB", float64(n)/(1<<10))
	default:
		return fmt.Sprintf("%dB", n)
	}
}

func usage() {
	fmt.Println(`session-reader — inspect saved agent session files

Commands:
  list                         List all sessions
  show   <id-prefix>           Display messages in a session
  export <id-prefix> <out.html> Export session as HTML
  raw    <id-prefix>           Print raw JSONL entries`)
}
