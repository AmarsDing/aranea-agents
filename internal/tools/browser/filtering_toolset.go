package browser

import (
	"context"
	"strings"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Sub-tool group constants. These classify the Playwright MCP Server's
// browser_* tools into functional categories so administrators can enable
// only the subsets needed for a given use case.
//
// Classification is based on the Playwright MCP tool naming convention:
// all tools are prefixed with "browser_" (after any MCP ToolPrefix).
const (
	// SubGroupNavigate covers URL navigation tools.
	// Tools: browser_navigate, browser_navigate_back, browser_navigate_forward.
	SubGroupNavigate = "navigate"

	// SubGroupInteract covers element interaction tools.
	// Tools: browser_click, browser_type, browser_press_key, browser_hover,
	// browser_select_option, browser_fill.
	SubGroupInteract = "interact"

	// SubGroupObserve covers page observation tools.
	// Tools: browser_snapshot, browser_take_screenshot, browser_get_text.
	SubGroupObserve = "observe"

	// SubGroupTabs covers browser tab management tools.
	// Tools: browser_tab_list, browser_tab_create, browser_tab_close,
	// browser_tab_select.
	SubGroupTabs = "tabs"

	// SubGroupOther covers tools not in the above categories
	// (e.g. browser_close, browser_wait, browser_evaluate).
	// Included in the "other" bucket so it can be enabled/disabled as a group.
	SubGroupOther = "other"
)

// allSubGroups lists all valid sub-group names. Used for validation and
// documentation.
var allSubGroups = []string{
	SubGroupNavigate,
	SubGroupInteract,
	SubGroupObserve,
	SubGroupTabs,
	SubGroupOther,
}

// baseBrowserToolName strips any MCP ToolPrefix from name and returns the
// base tool name starting with "browser_". If name does not contain
// "browser_", the lowercased input is returned unchanged.
//
// Example: "bw_browser_navigate_back" → "browser_navigate_back"
func baseBrowserToolName(name string) string {
	lower := strings.ToLower(name)
	idx := strings.Index(lower, "browser_")
	if idx < 0 {
		return lower
	}
	return lower[idx:]
}

// classifyBrowserTool maps a tool name to its sub-group. Classification is
// prefix-based on the base tool name (after stripping MCP ToolPrefix).
//
// Tools that don't match any known prefix are classified as SubGroupOther.
func classifyBrowserTool(name string) string {
	base := baseBrowserToolName(name)
	switch {
	case strings.HasPrefix(base, "browser_navigate"):
		return SubGroupNavigate
	case strings.HasPrefix(base, "browser_click") ||
		strings.HasPrefix(base, "browser_type") ||
		strings.HasPrefix(base, "browser_press") ||
		strings.HasPrefix(base, "browser_hover") ||
		strings.HasPrefix(base, "browser_select") ||
		strings.HasPrefix(base, "browser_fill") ||
		strings.HasPrefix(base, "browser_mouse") ||
		strings.HasPrefix(base, "browser_drag"):
		return SubGroupInteract
	case strings.HasPrefix(base, "browser_snapshot") ||
		strings.HasPrefix(base, "browser_take_screenshot") ||
		strings.HasPrefix(base, "browser_screenshot") ||
		strings.HasPrefix(base, "browser_get_text") ||
		strings.HasPrefix(base, "browser_wait_for"):
		return SubGroupObserve
	case strings.HasPrefix(base, "browser_tab"):
		return SubGroupTabs
	default:
		return SubGroupOther
	}
}

// FilteringToolSet wraps a trpctool.ToolSet so that only tools in the
// configured sub-groups are exposed via Tools(ctx). Tools outside the
// allowed groups are filtered out.
//
// When allowedGroups is nil or empty, all tools are passed through
// (no filtering). This preserves backward compatibility with configurations
// that don't specify EnabledSubGroups.
//
// Compile-time interface assertion guarantees conformance to trpctool.ToolSet.
var _ trpctool.ToolSet = (*FilteringToolSet)(nil)

type FilteringToolSet struct {
	inner         trpctool.ToolSet
	allowedGroups map[string]bool
}

// NewFilteringToolSet wraps inner so only tools in allowedGroups are exposed.
// Returns nil if inner is nil. If allowedGroups is empty, the returned
// ToolSet passes all tools through (no filtering).
func NewFilteringToolSet(inner trpctool.ToolSet, allowedGroups []string) *FilteringToolSet {
	if inner == nil {
		return nil
	}
	if len(allowedGroups) == 0 {
		return &FilteringToolSet{inner: inner, allowedGroups: nil}
	}
	groups := make(map[string]bool, len(allowedGroups))
	for _, g := range allowedGroups {
		key := strings.TrimSpace(strings.ToLower(g))
		if key != "" {
			groups[key] = true
		}
	}
	if len(groups) == 0 {
		return &FilteringToolSet{inner: inner, allowedGroups: nil}
	}
	return &FilteringToolSet{inner: inner, allowedGroups: groups}
}

// Name returns the wrapped ToolSet's name.
func (s *FilteringToolSet) Name() string {
	if s == nil || s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

// Close releases resources held by the wrapped ToolSet.
func (s *FilteringToolSet) Close() error {
	if s == nil || s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

// Tools returns the wrapped ToolSet's tools, filtered to only include tools
// in the allowed sub-groups. When no groups are configured (nil), all tools
// are returned unchanged.
func (s *FilteringToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if s == nil || s.inner == nil {
		return nil
	}
	raw := s.inner.Tools(ctx)
	if len(raw) == 0 || s.allowedGroups == nil {
		return raw
	}
	out := make([]trpctool.Tool, 0, len(raw))
	for _, t := range raw {
		if t == nil {
			continue
		}
		decl := t.Declaration()
		if decl == nil {
			// Tools without declarations can't be classified; skip them
			// to avoid exposing unclassified tools.
			continue
		}
		group := classifyBrowserTool(decl.Name)
		if s.allowedGroups[group] {
			out = append(out, t)
		}
	}
	return out
}
