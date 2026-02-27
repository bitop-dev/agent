# Prompt Templates

Prompt templates are Markdown files that expand into full prompts when you
type `/template-name arg1 arg2` in the REPL. They save you from retyping
common prompts and let you parameterize them with arguments.

---

## Discovery

| Location | Priority |
|----------|----------|
| `~/.config/agent/prompts/` | Global |
| `{cwd}/.agent/prompts/` | Project |

Files must end in `.md`. The template name is the filename without the
extension.

---

## Template Format

Templates are plain Markdown files with optional YAML frontmatter:

```markdown
---
description: Review a Go file for idiomatic style and correctness.
---

Please review the file `$1` for:

1. **Correctness** — logic errors, off-by-one, nil dereferences
2. **Idiomatic Go** — error handling, naming, package design
3. **Test coverage** — missing cases, table-driven tests
4. **Performance** — obvious inefficiencies

Respond with a numbered list of findings, ordered by severity.
Most critical issues first.
```

### Frontmatter

| Field | Description |
|-------|-------------|
| `description` | Shown in `/templates` listing |

If no frontmatter, the first non-empty line is used as the description
(truncated to 60 characters).

---

## Argument Placeholders

| Placeholder | Expands to |
|-------------|-----------|
| `$1`, `$2`, … | Positional arguments (1-indexed) |
| `$@` | All arguments joined with spaces |
| `$ARGUMENTS` | Same as `$@` |
| `${@:N}` | Arguments from position N onwards |
| `${@:N:L}` | L arguments starting at position N |

Missing positional arguments expand to empty string.

---

## Using Templates

List available templates:

```
/templates
```

Invoke a template:

```
/review pkg/agent/loop.go
```

With multiple arguments:

```
/compare old_impl.go new_impl.go
```

All remaining text after the template name becomes `$@`:

```
/explain The observer pattern as used in pkg/agent/agent.go
```

---

## Examples

### `review.md` — Code review

```markdown
---
description: Review a file for correctness, style, and security.
---

Please do a thorough code review of `$1`.

Focus on:
- Correctness and edge cases
- Error handling
- Security implications
- Readability and naming
- Missing tests

Be specific: quote the problematic code and explain exactly what to fix.
```

Usage: `/review pkg/tools/builtin/bash.go`

---

### `explain.md` — Explain concept

```markdown
---
description: Explain a concept or piece of code clearly.
---

Please explain $@ in simple terms.

Structure your response as:
1. One-sentence summary
2. Detailed explanation with an analogy if helpful
3. A short Go code example
4. Common pitfalls or misconceptions
```

Usage: `/explain the observer pattern`

---

### `test.md` — Generate tests

```markdown
---
description: Generate comprehensive tests for a Go file.
---

Read `$1` and write comprehensive tests for it.

Requirements:
- Use table-driven tests with `t.Run`
- Cover happy path, edge cases, and error conditions
- Use `t.Helper()` in assertion helpers
- Add benchmark functions for any performance-sensitive code
- Put tests in `${1}_test.go` (same package, `_test` suffix)

Write the complete test file.
```

Usage: `/test pkg/agent/compaction.go`

---

### `compare.md` — Compare two files

```markdown
---
description: Compare two files or implementations and summarise the differences.
---

Read both `$1` and `$2`.

Provide a structured comparison:

## Purpose
How are their goals similar or different?

## Key Differences
List the most important differences with brief explanations.

## Trade-offs
What are the pros and cons of each approach?

## Recommendation
Which would you choose and why?
```

Usage: `/compare old/auth.go new/auth.go`

---

### `commit.md` — Generate commit message

```markdown
---
description: Generate a conventional commit message for staged changes.
---

Run `git diff --staged` and write a conventional commit message for the
changes.

Format:
```
<type>(<scope>): <short description>

<body explaining what and why, not how>

<footer with breaking changes or issue refs if applicable>
```

Types: feat, fix, docs, style, refactor, test, chore

Keep the subject line under 72 characters.
```

Usage: `/commit`

---

## Argument Parsing

Arguments are parsed like shell tokens — quoted strings are supported:

```
/my-template "first argument with spaces" second_arg
```

This produces `$1 = "first argument with spaces"`, `$2 = "second_arg"`.

---

## Creating Your Own Templates

```bash
# Global template
mkdir -p ~/.config/agent/prompts
cat > ~/.config/agent/prompts/my-template.md << 'EOF'
---
description: Short description of what this template does.
---

Your prompt here with $1, $2, $@ placeholders.
EOF

# Project-specific template
mkdir -p .agent/prompts
cat > .agent/prompts/deploy.md << 'EOF'
---
description: Deploy to an environment.
---

Deploy the application to the $1 environment.

Steps:
1. Run the test suite
2. Build the Docker image
3. Push to registry
4. Update the $1 deployment

Check the deployment logs and confirm it is healthy.
EOF
```
