package mcp

import "testing"

func TestResolveStringMap(t *testing.T) {
	tests := []struct {
		name string
		cfg  map[string]any
		key  string
		want map[string]string
	}{
		{
			name: "map any",
			cfg:  map[string]any{"headers": map[string]any{"Authorization": "Bearer token", "X-N": 2}},
			key:  "headers",
			want: map[string]string{"Authorization": "Bearer token", "X-N": "2"},
		},
		{
			name: "map string",
			cfg:  map[string]any{"env": map[string]string{"API_KEY": "secret"}},
			key:  "env",
			want: map[string]string{"API_KEY": "secret"},
		},
		{
			name: "missing",
			cfg:  map[string]any{},
			key:  "headers",
			want: nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveStringMap(tt.cfg, tt.key)
			if len(got) != len(tt.want) {
				t.Fatalf("len mismatch: got %v want %v", got, tt.want)
			}
			for k, want := range tt.want {
				if got[k] != want {
					t.Fatalf("key %s: got %q want %q", k, got[k], want)
				}
			}
		})
	}
}

func TestMergeStringMaps(t *testing.T) {
	got := mergeStringMaps(
		map[string]string{"A": "1", "B": "2"},
		map[string]string{"B": "override", "C": "3"},
	)
	want := map[string]string{"A": "1", "B": "override", "C": "3"}
	if len(got) != len(want) {
		t.Fatalf("len mismatch: got %v want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("key %s: got %q want %q", k, got[k], v)
		}
	}
}
