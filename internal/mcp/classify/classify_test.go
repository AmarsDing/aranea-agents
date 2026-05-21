package classify

import "testing"

type fakeMeta struct{ meta map[string]any }

func (f fakeMeta) GetMeta() map[string]any { return f.meta }

func TestIsMCPToolInvocation(t *testing.T) {
	cases := []struct {
		name   string
		tool   string
		result any
		want   bool
	}{
		{"broker", "mcp_call", nil, true},
		{"prefixed", "mcp_sql__query", nil, true},
		{"meta", "other", fakeMeta{meta: map[string]any{"x": 1}}, true},
		{"plain", "read_file", nil, false},
	}
	for _, tc := range cases {
		if got := IsMCPToolInvocation(tc.tool, tc.result); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
