package sse_test

import (
	"strings"
	"testing"

	"github.com/nickcecere/agent/pkg/ai/sse"
)

func events(input string) []sse.Event {
	r := sse.NewReader(strings.NewReader(input))
	var out []sse.Event
	for {
		ev, err := r.Next()
		if err != nil {
			break
		}
		out = append(out, ev)
	}
	return out
}

func TestReader_SingleEvent(t *testing.T) {
	evs := events("data: hello\n\n")
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if evs[0].Data != "hello" {
		t.Errorf("data = %q, want %q", evs[0].Data, "hello")
	}
}

func TestReader_EventWithType(t *testing.T) {
	evs := events("event: ping\ndata: pong\n\n")
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if evs[0].Type != "ping" {
		t.Errorf("type = %q, want %q", evs[0].Type, "ping")
	}
	if evs[0].Data != "pong" {
		t.Errorf("data = %q, want %q", evs[0].Data, "pong")
	}
}

func TestReader_MultipleEvents(t *testing.T) {
	evs := events("data: one\n\ndata: two\n\ndata: three\n\n")
	if len(evs) != 3 {
		t.Fatalf("want 3 events, got %d", len(evs))
	}
	want := []string{"one", "two", "three"}
	for i, w := range want {
		if evs[i].Data != w {
			t.Errorf("event[%d].Data = %q, want %q", i, evs[i].Data, w)
		}
	}
}

func TestReader_SkipsComments(t *testing.T) {
	evs := events(": this is a comment\ndata: real\n\n")
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if evs[0].Data != "real" {
		t.Errorf("data = %q", evs[0].Data)
	}
}

func TestReader_DoneSignal(t *testing.T) {
	evs := events("data: [DONE]\n\n")
	// [DONE] is a valid event — readers may handle it upstream.
	// The SSE reader itself just surfaces it as data.
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	if evs[0].Data != "[DONE]" {
		t.Errorf("data = %q, want [DONE]", evs[0].Data)
	}
}

func TestReader_MultilineData(t *testing.T) {
	evs := events("data: line1\ndata: line2\n\n")
	if len(evs) != 1 {
		t.Fatalf("want 1 event, got %d", len(evs))
	}
	// Per SSE spec, multiple data lines are joined with \n.
	if evs[0].Data != "line1\nline2" {
		t.Errorf("data = %q, want %q", evs[0].Data, "line1\nline2")
	}
}

func TestReader_EmptyStream(t *testing.T) {
	evs := events("")
	if len(evs) != 0 {
		t.Errorf("want 0 events on empty stream, got %d", len(evs))
	}
}

func TestReader_WhitespaceStrippedFromData(t *testing.T) {
	// SSE spec: strip a single leading space after the colon.
	evs := events("data: hello world\n\n")
	if len(evs) == 0 {
		t.Fatal("no events")
	}
	if evs[0].Data != "hello world" {
		t.Errorf("data = %q", evs[0].Data)
	}
}
