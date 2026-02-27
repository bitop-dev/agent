# Skills

Skills are Markdown files containing specialized instructions for specific
tasks. They extend the agent's capabilities without touching code.

The agent lists available skills in the system prompt. When a task matches a
skill's description, the agent reads the skill file to get detailed
instructions.

---

## Discovery

Skills are loaded from two locations:

| Location | Priority |
|----------|----------|
| `~/.config/agent/skills/` | Global (loaded first) |
| `{cwd}/.agent/skills/` | Project (lower priority) |

If two skills have the same name, the global one wins.

**File formats:**

- `~/.config/agent/skills/my-skill.md` — root `.md` file
- `~/.config/agent/skills/my-skill/SKILL.md` — subdirectory with `SKILL.md`

The subdirectory form lets you bundle related files (examples, templates,
supporting docs) alongside the skill.

---

## Skill File Format

Every skill file must have YAML frontmatter with at least a `description`:

```markdown
---
name: go-expert
description: Expert Go programming guidance. Use when writing, reviewing, or refactoring Go code.
---

# Go Expert

You are an expert Go programmer. Follow these guidelines:

## Code Style
- Use `gofmt` formatting
- Prefer standard library over third-party deps
- Write table-driven tests with `t.Run`

## Error Handling
- Always handle errors; never use `_` for error returns
- Wrap errors with `fmt.Errorf("context: %w", err)`
- Return errors to callers rather than logging them

## Performance
- Profile before optimizing
- Prefer `strings.Builder` over `+` concatenation in loops
- Use `sync.Pool` for frequently allocated objects
```

### Frontmatter Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | No | Skill name (defaults to file/dir name) |
| `description` | **Yes** | Short description shown in system prompt (max 1024 chars) |

**Name validation:**
- Lowercase letters, digits, and hyphens only
- Cannot start or end with `-`
- No consecutive `--`
- Max 64 characters

---

## How the Agent Uses Skills

When skills are loaded, the system prompt includes an `<available_skills>`
block:

```xml
<available_skills>
  <skill>
    <name>go-expert</name>
    <description>Expert Go programming guidance. Use when writing, reviewing, or refactoring Go code.</description>
    <location>/home/user/.config/agent/skills/go-expert/SKILL.md</location>
  </skill>
</available_skills>
```

The agent is instructed:

> "Use the read tool to load a skill's file when the task matches its
> description."

So when you ask "refactor this Go function", the agent will:
1. Identify that the `go-expert` skill matches
2. Call `read` on the skill file
3. Apply the skill's instructions to the task

---

## Listing Skills

In the REPL:

```
/skills
```

This prints all loaded skills with their name, description, source
(global/project), and file path.

---

## Example Skills

### Skill: Go Expert

```
~/.config/agent/skills/go-expert/SKILL.md
```

```markdown
---
name: go-expert
description: Expert Go programming assistance. Use for writing, reviewing, testing, or debugging Go code.
---

# Go Expert Skill

When helping with Go code, follow these conventions:

## Idioms
- Use `errors.Is` / `errors.As` for error comparison
- Prefer `context.Context` as the first parameter for any function that does I/O
- Use `defer` for cleanup (files, mutexes, etc.)
- Avoid named return values except for documentation clarity

## Testing
- Write table-driven tests:
  ```go
  for _, tc := range []struct{ in, want string }{
      {"hello", "HELLO"},
  } {
      t.Run(tc.in, func(t *testing.T) {
          if got := strings.ToUpper(tc.in); got != tc.want {
              t.Errorf("got %q, want %q", got, tc.want)
          }
      })
  }
  ```
- Use `t.Helper()` in assertion helpers
- Test exported behaviour, not internal implementation

## Common Packages
- `encoding/json` for JSON; use struct tags `json:"name,omitempty"`
- `net/http` for HTTP clients/servers (no framework needed for simple APIs)
- `sync.WaitGroup` + `errgroup` for concurrent operations
- `flag` or `cobra` for CLI arguments

## Performance
- Run `go test -bench=. -benchmem ./...` before claiming something is faster
- Profile with `go tool pprof` for real bottlenecks
```

### Skill: SQL Reviewer

```
~/.config/agent/skills/sql-reviewer.md
```

```markdown
---
name: sql-reviewer
description: Review SQL queries for correctness, performance, and security. Use when asked to check, optimize, or write SQL.
---

When reviewing SQL:

1. **Correctness**: Check JOINs, NULL handling, GROUP BY clause
2. **Performance**: Look for missing indexes, N+1 patterns, SELECT *
3. **Security**: Check for SQL injection risks (parameterized queries?)
4. **Style**: Uppercase keywords, lowercase identifiers

Always explain WHY a change is suggested.
```

---

## Creating Your Own Skills

1. Create the directory:
   ```bash
   mkdir -p ~/.config/agent/skills/my-skill
   ```

2. Write the skill file:
   ```bash
   cat > ~/.config/agent/skills/my-skill/SKILL.md << 'EOF'
   ---
   name: my-skill
   description: Does something specific. Use when the user needs X.
   ---
   
   # My Skill
   
   Detailed instructions here...
   EOF
   ```

3. Restart the agent (or reload — skills are loaded at startup).

**Tips:**
- Keep the `description` short and specific — the agent uses it to decide *when* to activate the skill
- The skill body can be as long as you need — it's read lazily via the `read` tool
- Include examples in the skill body; they're very effective
- You can reference other files in the skill directory (use absolute paths or paths relative to the skill's location)
