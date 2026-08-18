package browser

import (
	"context"
	"testing"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- baseBrowserToolName ---

func TestBaseBrowserToolName_NoPrefix(t *testing.T) {
	if got := baseBrowserToolName("browser_navigate"); got != "browser_navigate" {
		t.Fatalf("expected browser_navigate, got %q", got)
	}
}

func TestBaseBrowserToolName_WithMCPPrefix(t *testing.T) {
	if got := baseBrowserToolName("bw_browser_navigate_back"); got != "browser_navigate_back" {
		t.Fatalf("expected browser_navigate_back, got %q", got)
	}
	if got := baseBrowserToolName("playwright_browser_click"); got != "browser_click" {
		t.Fatalf("expected browser_click, got %q", got)
	}
}

func TestBaseBrowserToolName_NoBrowserPrefix(t *testing.T) {
	if got := baseBrowserToolName("file_read"); got != "file_read" {
		t.Fatalf("expected file_read, got %q", got)
	}
}

func TestBaseBrowserToolName_Empty(t *testing.T) {
	if got := baseBrowserToolName(""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
}

func TestBaseBrowserToolName_CaseInsensitive(t *testing.T) {
	if got := baseBrowserToolName("BW_BROWSER_NAVIGATE"); got != "browser_navigate" {
		t.Fatalf("expected browser_navigate, got %q", got)
	}
}

// --- classifyBrowserTool ---

func TestClassifyBrowserTool_Navigate(t *testing.T) {
	tests := []string{
		"browser_navigate",
		"browser_navigate_back",
		"browser_navigate_forward",
		"bw_browser_navigate",
		"playwright_browser_navigate_back",
	}
	for _, name := range tests {
		if got := classifyBrowserTool(name); got != SubGroupNavigate {
			t.Errorf("classifyBrowserTool(%q) = %q, want %q", name, got, SubGroupNavigate)
		}
	}
}

func TestClassifyBrowserTool_Interact(t *testing.T) {
	tests := []string{
		"browser_click",
		"browser_type",
		"browser_press_key",
		"browser_hover",
		"browser_select_option",
		"browser_fill",
		"browser_fill_form",
		"browser_mouse_click_xy",
		"browser_drag",
		"bw_browser_click",
	}
	for _, name := range tests {
		if got := classifyBrowserTool(name); got != SubGroupInteract {
			t.Errorf("classifyBrowserTool(%q) = %q, want %q", name, got, SubGroupInteract)
		}
	}
}

func TestClassifyBrowserTool_Observe(t *testing.T) {
	tests := []string{
		"browser_snapshot",
		"browser_take_screenshot",
		"browser_screenshot",
		"browser_get_text",
		"browser_wait_for",
		"bw_browser_snapshot",
	}
	for _, name := range tests {
		if got := classifyBrowserTool(name); got != SubGroupObserve {
			t.Errorf("classifyBrowserTool(%q) = %q, want %q", name, got, SubGroupObserve)
		}
	}
}

func TestClassifyBrowserTool_Tabs(t *testing.T) {
	tests := []string{
		"browser_tab_list",
		"browser_tab_create",
		"browser_tab_close",
		"browser_tab_select",
		"bw_browser_tab_list",
	}
	for _, name := range tests {
		if got := classifyBrowserTool(name); got != SubGroupTabs {
			t.Errorf("classifyBrowserTool(%q) = %q, want %q", name, got, SubGroupTabs)
		}
	}
}

func TestClassifyBrowserTool_Other(t *testing.T) {
	tests := []string{
		"browser_close",
		"browser_wait",
		"browser_evaluate",
		"unknown_tool",
	}
	for _, name := range tests {
		if got := classifyBrowserTool(name); got != SubGroupOther {
			t.Errorf("classifyBrowserTool(%q) = %q, want %q", name, got, SubGroupOther)
		}
	}
}

// --- FilteringToolSet construction ---

func TestNewFilteringToolSet_NilInner(t *testing.T) {
	if got := NewFilteringToolSet(nil, []string{"navigate"}); got != nil {
		t.Fatalf("expected nil for nil inner, got %v", got)
	}
}

func TestNewFilteringToolSet_EmptyGroups(t *testing.T) {
	inner := &mockToolSet{}
	got := NewFilteringToolSet(inner, nil)
	if got == nil {
		t.Fatal("expected non-nil for nil groups")
	}
	if got.allowedGroups != nil {
		t.Fatalf("expected nil allowedGroups for empty input, got %v", got.allowedGroups)
	}
}

func TestNewFilteringToolSet_NormalizesGroups(t *testing.T) {
	inner := &mockToolSet{}
	got := NewFilteringToolSet(inner, []string{"  Navigate  ", "OBSERVE", ""})
	if got == nil || got.allowedGroups == nil {
		t.Fatal("expected non-nil with allowedGroups")
	}
	if !got.allowedGroups["navigate"] {
		t.Error("expected 'navigate' in allowedGroups")
	}
	if !got.allowedGroups["observe"] {
		t.Error("expected 'observe' in allowedGroups")
	}
	if len(got.allowedGroups) != 2 {
		t.Errorf("expected 2 groups, got %d", len(got.allowedGroups))
	}
}

// --- FilteringToolSet.Name / Close ---

func TestFilteringToolSet_Name_NilReceiver(t *testing.T) {
	var s *FilteringToolSet
	if s.Name() != "" {
		t.Fatal("nil receiver should return empty name")
	}
}

func TestFilteringToolSet_Name_Delegates(t *testing.T) {
	inner := &mockToolSet{nameFn: func() string { return "browser" }}
	s := NewFilteringToolSet(inner, nil)
	if s.Name() != "browser" {
		t.Fatalf("expected 'browser', got %q", s.Name())
	}
}

func TestFilteringToolSet_Close_NilReceiver(t *testing.T) {
	var s *FilteringToolSet
	if err := s.Close(); err != nil {
		t.Fatalf("nil receiver should return nil, got %v", err)
	}
}

func TestFilteringToolSet_Close_Delegates(t *testing.T) {
	closed := false
	inner := &mockToolSet{closeFn: func() error {
		closed = true
		return nil
	}}
	s := NewFilteringToolSet(inner, nil)
	if err := s.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closed {
		t.Fatal("expected Close to be delegated")
	}
}

// --- FilteringToolSet.Tools ---

func TestFilteringToolSet_Tools_NilReceiver(t *testing.T) {
	var s *FilteringToolSet
	if tools := s.Tools(context.Background()); tools != nil {
		t.Fatalf("nil receiver should return nil, got %v", tools)
	}
}

func TestFilteringToolSet_Tools_EmptyInner(t *testing.T) {
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return nil
	}}
	s := NewFilteringToolSet(inner, []string{"navigate"})
	if tools := s.Tools(context.Background()); tools != nil {
		t.Fatalf("expected nil for empty inner, got %v", tools)
	}
}

func TestFilteringToolSet_Tools_NoGroups_Passthrough(t *testing.T) {
	tools := []trpctool.Tool{
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		}},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_click"}
		}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return tools
	}}
	s := NewFilteringToolSet(inner, nil)
	got := s.Tools(context.Background())
	if len(got) != 2 {
		t.Fatalf("expected 2 tools (no filtering), got %d", len(got))
	}
}

func TestFilteringToolSet_Tools_FilterToNavigateOnly(t *testing.T) {
	tools := []trpctool.Tool{
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		}},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_click"}
		}},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_snapshot"}
		}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return tools
	}}
	s := NewFilteringToolSet(inner, []string{"navigate"})
	got := s.Tools(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 tool (navigate only), got %d", len(got))
	}
	decl := got[0].Declaration()
	if decl.Name != "browser_navigate" {
		t.Fatalf("expected browser_navigate, got %q", decl.Name)
	}
}

func TestFilteringToolSet_Tools_FilterToNavigateAndObserve(t *testing.T) {
	tools := []trpctool.Tool{
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		}},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_click"}
		}},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_snapshot"}
		}},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_take_screenshot"}
		}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return tools
	}}
	s := NewFilteringToolSet(inner, []string{"navigate", "observe"})
	got := s.Tools(context.Background())
	if len(got) != 3 {
		t.Fatalf("expected 3 tools (navigate + observe), got %d", len(got))
	}
}

func TestFilteringToolSet_Tools_FilterWithMCPPrefix(t *testing.T) {
	tools := []trpctool.Tool{
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "bw_browser_navigate"}
		}},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "bw_browser_click"}
		}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return tools
	}}
	s := NewFilteringToolSet(inner, []string{"navigate"})
	got := s.Tools(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 prefixed tool (navigate), got %d", len(got))
	}
}

func TestFilteringToolSet_Tools_NilToolSkipped(t *testing.T) {
	tools := []trpctool.Tool{
		nil,
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return tools
	}}
	s := NewFilteringToolSet(inner, []string{"navigate"})
	got := s.Tools(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 tool (nil skipped), got %d", len(got))
	}
}

func TestFilteringToolSet_Tools_NilDeclarationSkipped(t *testing.T) {
	tools := []trpctool.Tool{
		&mockCallableTool{declFn: func() *trpctool.Declaration { return nil }},
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return tools
	}}
	s := NewFilteringToolSet(inner, []string{"navigate"})
	got := s.Tools(context.Background())
	if len(got) != 1 {
		t.Fatalf("expected 1 tool (nil decl skipped), got %d", len(got))
	}
}

func TestFilteringToolSet_Tools_AllGroupsExcluded(t *testing.T) {
	tools := []trpctool.Tool{
		&mockCallableTool{declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return tools
	}}
	// "tabs" group doesn't match browser_navigate, so it should be filtered out.
	s := NewFilteringToolSet(inner, []string{"tabs"})
	got := s.Tools(context.Background())
	if len(got) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(got))
	}
}

// --- Integration: FilteringToolSet wraps NavigationGuardedToolSet ---

func TestFilteringToolSet_WrapsNavigationGuardedToolSet(t *testing.T) {
	// Build a mock inner ToolSet with navigation and non-navigation tools.
	navigateTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		},
		callFn: func(_ context.Context, _ []byte) (any, error) {
			return "navigated", nil
		},
	}
	clickTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_click"}
		},
		callFn: func(_ context.Context, _ []byte) (any, error) {
			return "clicked", nil
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{navigateTool, clickTool}
	}}

	// Wrap with NavigationGuardedToolSet (SSRF protection).
	guarded := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	// Wrap with FilteringToolSet allowing only "navigate" group.
	filtered := NewFilteringToolSet(guarded, []string{"navigate"})

	tools := filtered.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool after filtering, got %d", len(tools))
	}
	decl := tools[0].Declaration()
	if decl.Name != "browser_navigate" {
		t.Fatalf("expected browser_navigate, got %q", decl.Name)
	}
}
