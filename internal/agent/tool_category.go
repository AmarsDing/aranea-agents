package agent

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	biztool "aranea-agents/internal/biz/tool"
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

// toolCatalogLister is the minimal subset of biz/tool.ToolRegistryReader needed
// to build a category registry. Accepting a narrow interface keeps the agent
// package decoupled from the full usecase surface.
type toolCatalogLister interface {
	SearchTools(ctx context.Context, q biztool.ToolListQuery) (biztool.ToolListResult, error)
}

// NewToolCategorizerFromCatalog builds a ToolCategorizer backed by the tool
// catalog's category metadata. Unknown/empty categories are omitted so the
// prefix fallback in defaultToolCategorizer still applies.
func NewToolCategorizerFromCatalog(ctx context.Context, catalog toolCatalogLister) ToolCategorizer {
	if catalog == nil {
		return NewToolCategorizer(nil)
	}
	res, err := catalog.SearchTools(ctx, biztool.ToolListQuery{Limit: 10000})
	if err != nil {
		return NewToolCategorizer(nil)
	}
	registry := make(map[string]biz.ToolCategory, len(res.Items))
	for _, t := range res.Items {
		if cat, ok := mapCatalogCategory(t.Category); ok {
			registry[t.Key] = cat
		}
	}
	return NewToolCategorizer(registry)
}

// mapCatalogCategory maps a catalog category string to a frontend ToolCategory.
// It returns false when the category is unknown so callers can fall back to
// prefix/name-based classification.
func mapCatalogCategory(cat string) (biz.ToolCategory, bool) {
	switch strings.ToLower(strings.TrimSpace(cat)) {
	case "shell", "bash", "terminal":
		return biz.ToolCategoryShell, true
	case "browser", "playwright", "web":
		return biz.ToolCategoryBrowser, true
	case "file_read", "read", "file read":
		return biz.ToolCategoryFileRead, true
	case "file_write", "write", "edit", "file write", "edit_file":
		return biz.ToolCategoryFileWrite, true
	case "file_search", "search", "file search", "find", "grep":
		return biz.ToolCategoryFileSearch, true
	case "web_search", "web search":
		return biz.ToolCategoryWebSearch, true
	case "mcp":
		return biz.ToolCategoryMCP, true
	case "code", "coding", "execute", "python":
		return biz.ToolCategoryCode, true
	case "todo":
		return biz.ToolCategoryTodo, true
	}
	return "", false
}
