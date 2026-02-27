package models

import (
	"strings"
	"testing"
)

func TestLookup_ExactMatch(t *testing.T) {
	cases := []struct {
		id      string
		wantCtx int
	}{
		{"claude-opus-4-5", 200000},
		{"gpt-4o", 128000},
		{"gemini-2.5-pro", 1048576},
		{"o3", 200000},
	}
	for _, tc := range cases {
		info := Lookup(tc.id)
		if info == nil {
			t.Errorf("Lookup(%q) = nil, want info", tc.id)
			continue
		}
		if info.ContextWindow != tc.wantCtx {
			t.Errorf("Lookup(%q).ContextWindow = %d, want %d", tc.id, info.ContextWindow, tc.wantCtx)
		}
	}
}

func TestLookup_FuzzyPrefix(t *testing.T) {
	// Versioned IDs like "claude-sonnet-4-5-20251219" should match "claude-sonnet-4-5".
	info := Lookup("claude-sonnet-4-5-20251219")
	if info == nil {
		t.Fatal("Lookup with version suffix should return a result")
	}
	if !strings.Contains(info.ID, "claude-sonnet-4-5") {
		t.Errorf("unexpected ID %q", info.ID)
	}
}

func TestLookup_Unknown(t *testing.T) {
	if Lookup("no-such-model-xyz") != nil {
		t.Error("Lookup of unknown model should return nil")
	}
}

func TestContextWindowFor(t *testing.T) {
	if w := ContextWindowFor("gpt-4o"); w != 128000 {
		t.Errorf("ContextWindowFor(gpt-4o) = %d, want 128000", w)
	}
	if w := ContextWindowFor("unknown-model"); w != 0 {
		t.Errorf("ContextWindowFor(unknown) = %d, want 0", w)
	}
}

func TestMaxOutputFor(t *testing.T) {
	if n := MaxOutputFor("claude-3-7-sonnet-20250219"); n != 64000 {
		t.Errorf("MaxOutputFor = %d, want 64000", n)
	}
}

func TestSupportsThinking(t *testing.T) {
	thinking := []string{"claude-opus-4-5", "claude-3-7-sonnet-20250219", "o1", "o3", "gemini-2.5-pro"}
	for _, id := range thinking {
		info := Lookup(id)
		if info == nil {
			t.Errorf("Lookup(%q) = nil", id)
			continue
		}
		if !info.SupportsThinking {
			t.Errorf("%q should support thinking", id)
		}
	}

	noThinking := []string{"gpt-4o", "claude-3-5-sonnet-20241022", "gemini-1.5-pro"}
	for _, id := range noThinking {
		info := Lookup(id)
		if info == nil {
			t.Errorf("Lookup(%q) = nil", id)
			continue
		}
		if info.SupportsThinking {
			t.Errorf("%q should NOT support thinking", id)
		}
	}
}

func TestAll_NotEmpty(t *testing.T) {
	all := All()
	if len(all) < 10 {
		t.Errorf("All() returned %d models, want at least 10", len(all))
	}
	// Every model should have a non-zero context window.
	for _, m := range all {
		if m.ContextWindow <= 0 {
			t.Errorf("model %q has zero ContextWindow", m.ID)
		}
		if m.Provider == "" {
			t.Errorf("model %q has empty Provider", m.ID)
		}
	}
}

func TestCostFields(t *testing.T) {
	info := Lookup("claude-opus-4-5")
	if info == nil {
		t.Fatal("claude-opus-4-5 not found")
	}
	if info.InputCostPer1M <= 0 {
		t.Error("InputCostPer1M should be positive for claude-opus-4-5")
	}
	if info.OutputCostPer1M <= 0 {
		t.Error("OutputCostPer1M should be positive for claude-opus-4-5")
	}
}
