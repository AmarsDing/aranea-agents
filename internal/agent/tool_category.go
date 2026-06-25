package agent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// ToolCategorizer classifies a tool by its functional category.
// The category drives frontend UI rendering (shell terminal, browser card,
// file diff, etc.) without the frontend needing to parse tool_name.
//
// Stability:evolving
type ToolCategorizer interface {
	Categorize(toolName string) biz.ToolCategory
}

// defaultToolCategorizer implements ToolCategorizer using a registry lookup
// (accurate) with prefix/name matching as a fallback (covers unregistered tools).
type defaultToolCategorizer struct {
	// toolRegistry maps toolName → ToolCategory. Populated by ToolService at
	// startup from tool metadata. Empty entries fall back to prefix matching.
	toolRegistry map[string]biz.ToolCategory
}

// NewToolCategorizer creates a ToolCategorizer from an optional tool registry.
// When toolRegistry is nil or empty, all tools are classified via prefix matching.
func NewToolCategorizer(toolRegistry map[string]biz.ToolCategory) ToolCategorizer {
	if toolRegistry == nil {
		toolRegistry = make(map[string]biz.ToolCategory)
	}
	return &defaultToolCategorizer{toolRegistry: toolRegistry}
}

// Categorize returns the ToolCategory for a tool name.
//  1. Exact registry lookup (accurate, when ToolService provides metadata)
//  2. Prefix/name matching fallback (covers unregistered tools)
//  3. Falls back to ToolCategoryOther
func (c *defaultToolCategorizer) Categorize(toolName string) biz.ToolCategory {
	if toolName == "" {
		return biz.ToolCategoryOther
	}

	// 1. Registry lookup (accurate)
	if cat, ok := c.toolRegistry[toolName]; ok {
		return cat
	}

	// 2. Prefix/name matching fallback
	lower := strings.ToLower(toolName)
	switch {
	case strings.HasPrefix(lower, "shell") || strings.HasPrefix(lower, "bash"):
		return biz.ToolCategoryShell
	case strings.HasPrefix(lower, "browser") || strings.HasPrefix(lower, "playwright"):
		return biz.ToolCategoryBrowser
	case lower == "read_file" || lower == "cat" || lower == "head":
		return biz.ToolCategoryFileRead
	case lower == "write_file" || lower == "edit_file" || lower == "patch":
		return biz.ToolCategoryFileWrite
	case lower == "find" || lower == "grep" || lower == "glob":
		return biz.ToolCategoryFileSearch
	case lower == "web_search" || lower == "search":
		return biz.ToolCategoryWebSearch
	case strings.HasPrefix(lower, "mcp_"):
		return biz.ToolCategoryMCP
	case lower == "execute_code" || lower == "python":
		return biz.ToolCategoryCode
	case lower == "todo_write" || lower == "todo_read":
		return biz.ToolCategoryTodo
	default:
		return biz.ToolCategoryOther
	}
}

// noopToolCategorizer always returns ToolCategoryOther. Used in tests or when
// tool classification is not needed.
type noopToolCategorizer struct{}

// NewNoopToolCategorizer returns a ToolCategorizer that always returns Other.
func NewNoopToolCategorizer() ToolCategorizer {
	return &noopToolCategorizer{}
}

func (noopToolCategorizer) Categorize(string) biz.ToolCategory {
	return biz.ToolCategoryOther
}
