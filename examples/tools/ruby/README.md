# template — Ruby Plugin Tool

Renders Mustache-style templates with variable substitution, conditionals,
and loops. No gems required — pure Ruby stdlib.

## Requirements

Ruby 2.7+. No `bundle install` needed.

## Running as a Plugin

```yaml
# agent.yaml
tools:
  preset: coding
  plugins:
    - path: ruby
      args: ["./examples/tools/ruby/tool.rb"]
```

## Testing Manually

```bash
# Describe
echo '{"type":"describe"}' | ruby tool.rb

# Simple substitution
printf '{"type":"describe"}\n{"type":"call","call_id":"c1","params":{"template":"Hello, {{name}}! You are {{age}} years old.","vars":{"name":"Alice","age":30}}}\n' \
  | ruby tool.rb

# Conditional
printf '{"type":"describe"}\n{"type":"call","call_id":"c2","params":{"template":"{{#if admin}}ADMIN ACCESS{{else}}Normal user{{/if}}: {{user}}","vars":{"admin":true,"user":"bob"}}}\n' \
  | ruby tool.rb

# Loop
PAYLOAD='{"type":"call","call_id":"c3","params":{"template":"Items:\n{{#each items}}- {{name}}: ${{price}}\n{{/each}}","vars":{"items":[{"name":"Widget","price":9.99},{"name":"Gadget","price":24.99}]}}}'
printf '{"type":"describe"}\n%s\n' "$PAYLOAD" | ruby tool.rb
```

## Template Syntax

| Tag | Description |
|-----|-------------|
| `{{variable}}` | Substitute a variable (supports dot paths: `{{user.name}}`) |
| `{{{variable}}}` | Same but triple braces (alias) |
| `{{#if key}}...{{/if}}` | Conditional block |
| `{{#if key}}...{{else}}...{{/if}}` | Conditional with else |
| `{{#unless key}}...{{/unless}}` | Inverted conditional |
| `{{#each list}}...{{/each}}` | Loop over an array |

Inside `{{#each}}`, these special variables are available:

| Variable | Value |
|----------|-------|
| `{{@index}}` | Current index (0-based) |
| `{{@first}}` | `true` on the first iteration |
| `{{@last}}` | `true` on the last iteration |
| `{{@this}}` | The item itself (when items are primitives) |

## Parameters

| Name | Type | Required | Description |
|------|------|----------|-------------|
| `template` | string | ✓ | Template string |
| `vars` | object | ✓ | Variables to substitute |

## Example Templates

**Code generation:**
```
func {{name}}({{#each params}}{{type}} {{name}}{{#unless @last}}, {{/unless}}{{/each}}) error {
    // TODO: implement {{name}}
    return nil
}
```

**Markdown report:**
```
# {{title}}

Generated: {{date}}

{{#each sections}}
## {{heading}}

{{body}}

{{/each}}
```
