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
		// MCP tools that return no Meta (e.g. Playwright MCP's
		// playwright_browser_navigate) must still be classified as MCP via the
		// metaGetter type marker. Only *mcp.Tool.mcpToolResult implements
		// GetMeta() in trpc-agent-go, so the type assertion alone is a reliable
		// signal — the nil-Meta check was relaxed to fix mcpCount=0 for
		// Playwright tool calls.
		{"meta-nil", "playwright_browser_navigate", fakeMeta{meta: nil}, true},
		{"plain", "read_file", nil, false},
		{"non-meta-result", "playwright_browser_navigate", "raw string", false},
	}
	for _, tc := range cases {
		if got := IsMCPToolInvocation(tc.tool, tc.result); got != tc.want {
			t.Fatalf("%s: got %v want %v", tc.name, got, tc.want)
		}
	}
}
