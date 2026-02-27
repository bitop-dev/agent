---
description: Generate comprehensive Go tests for a file. Usage: /test <path>
---

Read `$1` and write comprehensive tests for it.

Rules:
- Use table-driven tests with `t.Run` for every function with multiple cases
- Cover: happy path, edge cases (nil, empty, zero), and error conditions
- Use `t.Helper()` in any assertion helpers you write
- Add `BenchmarkXxx` functions for anything performance-sensitive
- Output file should be `$1` with `_test.go` appended (same package, `_test` suffix for black-box tests, or same package for white-box)

For each function under test, think through:
1. What inputs could cause a panic?
2. What are the boundary conditions?
3. What does the function guarantee? Test that guarantee.

Write the complete test file, ready to run with `go test ./...`.
Do not omit any imports. Make the file syntactically complete.
