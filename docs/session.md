# Session Persistence

Sessions are stored as JSONL (JSON Lines) files. Every message in the
conversation — user, assistant, tool result, and compaction summaries — is
appended as one JSON object per line.

---

## File Location

```
~/.config/agent/sessions/YYYYMMDD-HHMMSS-<8hex>.jsonl
```

Example: `~/.config/agent/sessions/20260226-143012-a3f7c901.jsonl`

The directory is created automatically on first use. Override via
`$XDG_CONFIG_HOME` (defaults to `~/.config`).

---

## Session Lifecycle

### Starting a Session

Every time you run the agent in interactive mode or with `-prompt`, a new
session is created automatically and written to disk.

```bash
# New session (auto-created)
./agent -config agent.yaml

# Resume a previous session by ID prefix
./agent -config agent.yaml -session a3f7c9

# List recent sessions
./agent -config agent.yaml -sessions
./agent -sessions               # same thing
```

### In-Session Commands

```
/session          Print current session ID and file path
/sessions         List recent sessions (sorted by date)
/export           Export current session as self-contained HTML
```

---

## JSONL Format

### Line 1: Header

```json
{"type":"session","id":"a3f7c901-...","version":1,"timestamp":"2026-02-26T14:30:12Z","cwd":"/home/user/project"}
```

### Subsequent Lines: Entries

**Message entry (user, assistant, tool_result):**

```json
{"type":"message","id":"d4e5f6a7","parent_id":"c3d4e5f6","timestamp":"2026-02-26T14:30:13Z","role":"user","message":{"role":"user","content":[{"type":"text","text":"Hello!"}],"timestamp":1740579013000}}
```

```json
{"type":"message","id":"e5f6a7b8","parent_id":"d4e5f6a7","timestamp":"2026-02-26T14:30:15Z","role":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"Hello! How can I help?"}],"model":"claude-sonnet-4-5","provider":"anthropic","usage":{"input":12,"output":8,"total_tokens":20},"stop_reason":"stop","timestamp":1740579015000}}
```

```json
{"type":"message","id":"f6a7b8c9","parent_id":"e5f6a7b8","timestamp":"2026-02-26T14:30:16Z","role":"tool_result","message":{"role":"tool_result","tool_call_id":"call_abc","tool_name":"bash","content":[{"type":"text","text":"main.go\npkg/\n"}],"is_error":false,"timestamp":1740579016000}}
```

**Compaction entry:**

```json
{"type":"compaction","id":"a1b2c3d4","parent_id":"f6a7b8c9","timestamp":"2026-02-26T15:00:00Z","summary":"## Goal\nRefactor the auth module...\n\n## Progress\n...","first_kept_entry_id":"f6a7b8c9","tokens_before":45000}
```

**Branch entry (fork):**

```json
{"type":"branch","id":"b2c3d4e5","timestamp":"2026-02-26T15:30:00Z","parent_session_path":"/home/user/.config/agent/sessions/20260226-143012-a3f7c901.jsonl","fork_entry_id":"f6a7b8c9","branch_summary":"User was refactoring the auth module..."}
```

---

## Entry Types

### `session` (header)

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `"session"` |
| `id` | string | Session UUID |
| `version` | int | Format version (currently 1) |
| `timestamp` | string | ISO 8601 creation time |
| `cwd` | string | Working directory at creation |

### `message`

| Field | Type | Description |
|-------|------|-------------|
| `type` | string | `"message"` |
| `id` | string | 8-char hex entry ID |
| `parent_id` | string | Previous entry ID |
| `timestamp` | string | ISO 8601 |
| `role` | string | `"user"` \| `"assistant"` \| `"tool_result"` |
| `message` | object | Full message object (see below) |

**User message object:**
```json
{
  "role": "user",
  "content": [{"type": "text", "text": "..."}],
  "timestamp": 1740579013000
}
```

**Assistant message object:**
```json
{
  "role": "assistant",
  "content": [
    {"type": "text", "text": "..."},
    {"type": "thinking", "thinking": "..."},
    {"type": "tool_call", "id": "call_abc", "name": "bash", "arguments": {"command": "ls"}}
  ],
  "model": "claude-sonnet-4-5",
  "provider": "anthropic",
  "usage": {"input": 100, "output": 50, "cache_read": 80, "cache_write": 20, "total_tokens": 150},
  "stop_reason": "tool_use",
  "timestamp": 1740579015000
}
```

**Tool result message object:**
```json
{
  "role": "tool_result",
  "tool_call_id": "call_abc",
  "tool_name": "bash",
  "content": [{"type": "text", "text": "output..."}],
  "details": null,
  "is_error": false,
  "timestamp": 1740579016000
}
```

### `compaction`

| Field | Type | Description |
|-------|------|-------------|
| `summary` | string | LLM-generated structured summary (Markdown) |
| `first_kept_entry_id` | string | ID of first message NOT summarized |
| `tokens_before` | int | Estimated context tokens before compaction |

### `branch`

| Field | Type | Description |
|-------|------|-------------|
| `parent_session_path` | string | Absolute path to the parent session file |
| `fork_entry_id` | string | Last entry ID copied from parent |
| `branch_summary` | string | Optional LLM summary of the parent session |

---

## Reading Sessions Programmatically

```go
import (
    "github.com/nickcecere/agent/pkg/session"
)

// Load a session
sess, err := session.Load(dir, "a3f7c9")

// Read messages
msgs := sess.Messages()  // returns []ai.Message (respects compaction)

// Access raw session data
data, _ := os.ReadFile(sess.FilePath())
```

`ParseMessages(data)` handles compaction transparently:
- Before the `first_kept_entry_id`, messages are replaced by a synthetic user
  message containing the compaction summary.
- From `first_kept_entry_id` onwards, original messages are returned verbatim.

### Parsing Raw JSONL

```go
import (
    "bufio"
    "encoding/json"
    "os"
    "github.com/nickcecere/agent/pkg/session"
)

f, _ := os.Open("/home/user/.config/agent/sessions/20260226-143012-a3f7c901.jsonl")
scanner := bufio.NewScanner(f)

for scanner.Scan() {
    entryType, raw, _ := session.ParseLine(scanner.Bytes())
    switch entryType {
    case session.EntryTypeSession:
        var hdr session.Header
        json.Unmarshal(raw, &hdr)
        fmt.Printf("Session %s cwd=%s\n", hdr.ID[:8], hdr.CWD)
    case session.EntryTypeMessage:
        var entry session.MessageEntry
        json.Unmarshal(raw, &entry)
        fmt.Printf("[%s] %s\n", entry.ID, entry.Role)
    case session.EntryTypeCompaction:
        var entry session.CompactionEntry
        json.Unmarshal(raw, &entry)
        fmt.Printf("Compaction: %d tokens before\n", entry.TokensBefore)
    case session.EntryTypeBranch:
        var entry session.BranchEntry
        json.Unmarshal(raw, &entry)
        fmt.Printf("Branch from %s\n", entry.ParentSessionPath)
    }
}
```

---

## Session Forking

Fork an existing session to explore a different direction while keeping the
original intact.

```go
// From the REPL
/fork          // fork keeping last 20 messages
/fork 10       // fork keeping last 10 messages

// From Go code
parent, _ := session.Load(dir, "a3f7c9")
child, _ := parent.Fork(newDir, 10, "Exploring alternative approach")
```

The forked session:
1. Writes a `branch` header entry pointing back to the parent
2. Copies the last N messages from the parent
3. Continues as an independent session

---

## HTML Export

Export the current session as a self-contained HTML file (no external
dependencies, dark mode):

```
/export
```

Or from Go:

```go
import "github.com/nickcecere/agent/pkg/session"

data, _ := os.ReadFile(sess.FilePath())
html, err := session.ExportHTML(data, session.ExportOptions{
    Title: "My Session",
})
os.WriteFile("session.html", []byte(html), 0644)
```

The HTML includes color-coded message blocks:
- 🟢 User messages (green)
- 🔵 Assistant messages (blue)
- 🟡 Tool calls (amber)
- ⚫ Tool results (grey)
- 🟣 Thinking blocks (purple)
- 🟠 Compaction summaries (orange)
