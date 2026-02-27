---
name: go-expert
description: Expert Go programming guidance. Use when writing, reviewing, refactoring, or debugging Go code.
---

# Go Expert Skill

When helping with Go code, follow these conventions rigorously.

## Code Style

- Use `gofmt`/`goimports` formatting — always
- Prefer standard library over third-party packages where reasonable
- Use short, descriptive variable names; single-letter names only in tight loops or math
- Group imports: stdlib, then external, then internal — separated by blank lines
- Export only what needs to be exported; package-level docs on every exported symbol

## Error Handling

- **Always** handle errors; never use `_` for error returns except in tests
- Wrap errors with context: `fmt.Errorf("operation failed: %w", err)`
- Use `errors.Is` / `errors.As` for type-specific checking — never string comparison
- Return errors to callers; avoid logging errors at the point of creation
- Sentinel errors: `var ErrNotFound = errors.New("not found")`

## Interfaces

- Accept interfaces, return concrete types (usually)
- Define interfaces close to where they're used (consumer side), not the producer
- Keep interfaces small — one or two methods where possible
- Prefer embedding over wrapping for interface composition

## Concurrency

- Goroutines are cheap but not free — document their lifetime
- Use `context.Context` as the first parameter for any function that does I/O or blocks
- Pass `context.Context` down; never store it in a struct
- Use `sync.WaitGroup` + `errgroup.Group` for fan-out patterns
- Prefer channels for ownership transfer; mutexes for shared state
- Channels should have documented direction (`chan<-`, `<-chan`)

## Testing

Write table-driven tests:

```go
func TestMyFunc(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    string
        wantErr bool
    }{
        {"empty input", "", "", true},
        {"valid input", "hello", "HELLO", false},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            got, err := MyFunc(tc.input)
            if (err != nil) != tc.wantErr {
                t.Fatalf("err = %v, wantErr = %v", err, tc.wantErr)
            }
            if got != tc.want {
                t.Errorf("got %q, want %q", got, tc.want)
            }
        })
    }
}
```

- Use `t.Helper()` in assertion helpers
- Use `testify/require` for fatal assertions if already a dep, else plain `t.Fatal`
- Test the exported API; internal impl details tested via exported behaviour
- Benchmark performance-sensitive paths: `func BenchmarkX(b *testing.B)`

## Performance

- Profile before optimizing: `go test -bench=. -benchmem ./...`
- `go tool pprof` for CPU; `go tool trace` for goroutine contention
- Prefer `strings.Builder` over `+` in loops
- Reuse allocations with `sync.Pool` for hot paths
- Use `make([]T, 0, cap)` when capacity is known

## Common Pitfalls

- Loop variable capture in goroutines: shadow with `tc := tc` or use index
- `append` may or may not reallocate — don't assume slice sharing
- `json.Unmarshal` into `interface{}` gives `float64` for numbers
- `time.Time` zero value is not UTC epoch — use `time.Time.IsZero()`
- `defer` in a loop runs at function end, not iteration end — use a closure

## Project Structure (standard Go layout)

```
myproject/
├── cmd/myapp/      # main packages
├── internal/       # private packages
├── pkg/            # importable packages
├── go.mod
└── go.sum
```

Keep `main` packages thin — business logic lives in packages, not `main`.
