package agent

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestDefaultToolCategorizer_Categorize(t *testing.T) {
	c := NewToolCategorizer(nil)

	cases := []struct {
		name string
		want biz.ToolCategory
	}{
		// shell family
		{"shell", biz.ToolCategoryShell},
		{"shell_exec", biz.ToolCategoryShell},
		{"Shell", biz.ToolCategoryShell}, // case-insensitive
		{"bash", biz.ToolCategoryShell},
		{"BASH_RUN", biz.ToolCategoryShell},
		// browser family
		{"browser", biz.ToolCategoryBrowser},
		{"browser_navigate", biz.ToolCategoryBrowser},
		{"playwright", biz.ToolCategoryBrowser},
		{"playwright_screenshot", biz.ToolCategoryBrowser},
		// file_read family
		{"read_file", biz.ToolCategoryFileRead},
		{"cat", biz.ToolCategoryFileRead},
		{"head", biz.ToolCategoryFileRead},
		// file_write family
		{"write_file", biz.ToolCategoryFileWrite},
		{"edit_file", biz.ToolCategoryFileWrite},
		{"patch", biz.ToolCategoryFileWrite},
		// file_search family
		{"find", biz.ToolCategoryFileSearch},
		{"grep", biz.ToolCategoryFileSearch},
		{"glob", biz.ToolCategoryFileSearch},
		// web_search family
		{"web_search", biz.ToolCategoryWebSearch},
		{"search", biz.ToolCategoryWebSearch},
		// mcp family
		{"mcp_server_tool", biz.ToolCategoryMCP},
		{"mcp_", biz.ToolCategoryMCP},
		// code family
		{"execute_code", biz.ToolCategoryCode},
		{"python", biz.ToolCategoryCode},
		// todo family
		{"todo_write", biz.ToolCategoryTodo},
		{"todo_read", biz.ToolCategoryTodo},
		// unknown / other
		{"", biz.ToolCategoryOther},
		{"unknown_tool", biz.ToolCategoryOther},
		{"custom_widget", biz.ToolCategoryOther},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := c.Categorize(tc.name); got != tc.want {
				t.Fatalf("Categorize(%q) = %q, want %q", tc.name, got, tc.want)
			}
		})
	}
}

func TestDefaultToolCategorizer_RegistryOverridesPrefix(t *testing.T) {
	// Registry entry should take precedence over prefix matching.
	registry := map[string]biz.ToolCategory{
		"shell_custom": biz.ToolCategoryOther, // would match "shell" prefix, but registry says Other
		"read_file":    biz.ToolCategoryCode,  // would match file_read, but registry says Code
	}
	c := NewToolCategorizer(registry)

	if got := c.Categorize("shell_custom"); got != biz.ToolCategoryOther {
		t.Fatalf("registry override failed: Categorize(shell_custom) = %q, want other", got)
	}
	if got := c.Categorize("read_file"); got != biz.ToolCategoryCode {
		t.Fatalf("registry override failed: Categorize(read_file) = %q, want code", got)
	}
	// Non-registry entries still fall through to prefix matching.
	if got := c.Categorize("bash"); got != biz.ToolCategoryShell {
		t.Fatalf("prefix fallback failed: Categorize(bash) = %q, want shell", got)
	}
}

func TestNoopToolCategorizer(t *testing.T) {
	c := NewNoopToolCategorizer()
	for _, name := range []string{"shell", "read_file", "anything", ""} {
		if got := c.Categorize(name); got != biz.ToolCategoryOther {
			t.Fatalf("NoopToolCategorizer.Categorize(%q) = %q, want other", name, got)
		}
	}
}
