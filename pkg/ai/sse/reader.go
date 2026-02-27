// Package sse provides a minimal Server-Sent Events reader.
// It reads a stream of SSE lines and emits (event, data) pairs.
package sse

import (
	"bufio"
	"io"
	"strings"
)

// Event is a single SSE event with an optional type and data payload.
type Event struct {
	Type string // value of the "event:" field (may be empty)
	Data string // value of the "data:" field(s), joined with "\n"
}

// Reader reads SSE events from an io.Reader.
type Reader struct {
	scanner *bufio.Scanner
}

func NewReader(r io.Reader) *Reader {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 1<<20), 1<<20) // 1 MB buffer
	return &Reader{scanner: sc}
}

// Next returns the next event. Returns (Event{}, io.EOF) at end of stream.
func (r *Reader) Next() (Event, error) {
	var ev Event
	var dataLines []string

	for r.scanner.Scan() {
		line := r.scanner.Text()

		if line == "" {
			// Blank line = dispatch event
			if len(dataLines) > 0 || ev.Type != "" {
				ev.Data = strings.Join(dataLines, "\n")
				return ev, nil
			}
			continue
		}

		if strings.HasPrefix(line, "event:") {
			ev.Type = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		} else if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
		// id: and retry: fields are intentionally ignored
	}

	if err := r.scanner.Err(); err != nil {
		return Event{}, err
	}
	return Event{}, io.EOF
}
