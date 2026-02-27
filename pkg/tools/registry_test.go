package tools_test

import (
	"context"
	"testing"

	"github.com/nickcecere/agent/pkg/ai"
	"github.com/nickcecere/agent/pkg/tools"
)

// stubTool is a minimal Tool implementation for testing the registry.
type stubTool struct{ name string }

func (s *stubTool) Definition() ai.ToolDefinition {
	return ai.ToolDefinition{
		Name:        s.name,
		Description: "stub tool " + s.name,
		Parameters:  tools.MustSchema(tools.SimpleSchema{}),
	}
}

func (s *stubTool) Execute(_ context.Context, _ string, _ map[string]any, _ tools.UpdateFn) (tools.Result, error) {
	return tools.TextResult("ok"), nil
}

func TestRegistry_RegisterAndGet(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&stubTool{"alpha"})

	got := reg.Get("alpha")
	if got == nil {
		t.Fatal("expected to find registered tool 'alpha'")
	}
	if got.Definition().Name != "alpha" {
		t.Errorf("got name %q, want %q", got.Definition().Name, "alpha")
	}
}

func TestRegistry_Get_Missing(t *testing.T) {
	reg := tools.NewRegistry()
	if reg.Get("nonexistent") != nil {
		t.Error("expected nil for missing tool")
	}
}

func TestRegistry_All(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&stubTool{"a"})
	reg.Register(&stubTool{"b"})
	reg.Register(&stubTool{"c"})

	all := reg.All()
	if len(all) != 3 {
		t.Fatalf("want 3 tools, got %d", len(all))
	}
	names := map[string]bool{}
	for _, t := range all {
		names[t.Definition().Name] = true
	}
	for _, n := range []string{"a", "b", "c"} {
		if !names[n] {
			t.Errorf("tool %q missing from All()", n)
		}
	}
}

func TestRegistry_All_Empty(t *testing.T) {
	reg := tools.NewRegistry()
	if len(reg.All()) != 0 {
		t.Error("empty registry should return empty slice")
	}
}

func TestRegistry_Remove(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&stubTool{"x"})
	reg.Register(&stubTool{"y"})

	reg.Remove("x")

	if reg.Get("x") != nil {
		t.Error("tool 'x' should have been removed")
	}
	if reg.Get("y") == nil {
		t.Error("tool 'y' should still be present")
	}
	if len(reg.All()) != 1 {
		t.Errorf("expected 1 tool after remove, got %d", len(reg.All()))
	}
}

func TestRegistry_Remove_Missing(t *testing.T) {
	reg := tools.NewRegistry()
	// Should not panic.
	reg.Remove("does-not-exist")
}

func TestRegistry_RegisterOrReplace(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&stubTool{"dup"})
	reg.RegisterOrReplace(&stubTool{"dup"}) // should not panic

	if len(reg.All()) != 1 {
		t.Errorf("after RegisterOrReplace: want 1 tool, got %d", len(reg.All()))
	}
}

func TestRegistry_Register_Panics_OnDuplicate(t *testing.T) {
	reg := tools.NewRegistry()
	reg.Register(&stubTool{"dup"})
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic on duplicate registration")
		}
	}()
	reg.Register(&stubTool{"dup"})
}
