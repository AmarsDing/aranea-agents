package browser

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/pkg/apierror"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- mocks ---

type mockToolSet struct {
	nameFn  func() string
	closeFn func() error
	toolsFn func(ctx context.Context) []trpctool.Tool
}

func (m *mockToolSet) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return "mock-toolset"
}

func (m *mockToolSet) Close() error {
	if m.closeFn != nil {
		return m.closeFn()
	}
	return nil
}

func (m *mockToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if m.toolsFn != nil {
		return m.toolsFn(ctx)
	}
	return nil
}

type mockCallableTool struct {
	declFn func() *trpctool.Declaration
	callFn func(ctx context.Context, jsonArgs []byte) (any, error)
}

func (m *mockCallableTool) Declaration() *trpctool.Declaration {
	if m.declFn != nil {
		return m.declFn()
	}
	return &trpctool.Declaration{Name: "mock-tool"}
}

func (m *mockCallableTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if m.callFn != nil {
		return m.callFn(ctx, jsonArgs)
	}
	return nil, nil
}

// --- requiresURLValidation ---

func TestRequiresURLValidation_EmptyName(t *testing.T) {
	if requiresURLValidation("") {
		t.Fatal("empty tool name should not require validation")
	}
}

func TestRequiresURLValidation_ExactMatch(t *testing.T) {
	if !requiresURLValidation("browser_navigate") {
		t.Fatal("exact browser_navigate should require validation")
	}
}

func TestRequiresURLValidation_PrefixedMatch(t *testing.T) {
	// MCP ToolPrefix prefixes (e.g. "bw_browser_navigate") should match.
	if !requiresURLValidation("bw_browser_navigate") {
		t.Fatal("prefixed browser_navigate should require validation")
	}
	if !requiresURLValidation("playwright_browser_navigate") {
		t.Fatal("prefixed browser_navigate should require validation")
	}
}

func TestRequiresURLValidation_NonNavigationTool(t *testing.T) {
	if requiresURLValidation("browser_click") {
		t.Fatal("browser_click should not require validation")
	}
	if requiresURLValidation("browser_snapshot") {
		t.Fatal("browser_snapshot should not require validation")
	}
	if requiresURLValidation("browser_screenshot") {
		t.Fatal("browser_screenshot should not require validation")
	}
}

func TestRequiresURLValidation_SuffixButNotNavigation(t *testing.T) {
	// A tool whose name ends with _browser_navigate but is not navigation
	// would still match — this is by design (suffix-based matching).
	if !requiresURLValidation("foo_browser_navigate") {
		t.Fatal("suffix match should require validation")
	}
	// But a name that merely contains the substring should not match.
	if requiresURLValidation("browser_navigate_other") {
		t.Fatal("non-suffix match should not require validation")
	}
}

// --- extractURL ---

func TestExtractURL_Empty(t *testing.T) {
	if got := extractURL(nil); got != "" {
		t.Fatalf("expected empty URL for nil args, got %q", got)
	}
	if got := extractURL([]byte{}); got != "" {
		t.Fatalf("expected empty URL for empty args, got %q", got)
	}
}

func TestExtractURL_InvalidJSON(t *testing.T) {
	if got := extractURL([]byte("not-json")); got != "" {
		t.Fatalf("expected empty URL for invalid JSON, got %q", got)
	}
}

func TestExtractURL_NoURLField(t *testing.T) {
	if got := extractURL([]byte(`{"selector":"#btn"}`)); got != "" {
		t.Fatalf("expected empty URL when no url field, got %q", got)
	}
}

func TestExtractURL_URLField(t *testing.T) {
	got := extractURL([]byte(`{"url":"https://example.com"}`))
	if got != "https://example.com" {
		t.Fatalf("expected https://example.com, got %q", got)
	}
}

func TestExtractURL_AlternateKeyCase(t *testing.T) {
	// "URL" and "Url" alternates should be honored.
	for _, key := range []string{"URL", "Url"} {
		input := []byte(`{"` + key + `":"https://example.com"}`)
		if got := extractURL(input); got != "https://example.com" {
			t.Fatalf("expected https://example.com for key %s, got %q", key, got)
		}
	}
}

func TestExtractURL_NonStringValue(t *testing.T) {
	if got := extractURL([]byte(`{"url":123}`)); got != "" {
		t.Fatalf("expected empty URL for non-string value, got %q", got)
	}
}

func TestExtractURL_NullValue(t *testing.T) {
	if got := extractURL([]byte(`{"url":null}`)); got != "" {
		t.Fatalf("expected empty URL for null value, got %q", got)
	}
}

// --- NavigationGuardedToolSet construction ---

func TestNewNavigationGuardedToolSet_NilInner(t *testing.T) {
	if got := NewNavigationGuardedToolSet(nil, NavigationPolicy{}); got != nil {
		t.Fatalf("expected nil for nil inner, got %v", got)
	}
}

func TestNewNavigationGuardedToolSet_WrapsInner(t *testing.T) {
	inner := &mockToolSet{nameFn: func() string { return "browser" }}
	got := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	if got == nil {
		t.Fatal("expected non-nil guarded toolset")
	}
}

// --- NavigationGuardedToolSet.Name ---

func TestNavigationGuardedToolSet_Name_NilReceiver(t *testing.T) {
	var s *NavigationGuardedToolSet
	if s.Name() != "" {
		t.Fatal("nil receiver should return empty name")
	}
}

func TestNavigationGuardedToolSet_Name_Delegates(t *testing.T) {
	inner := &mockToolSet{nameFn: func() string { return "browser-mcp" }}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	if s.Name() != "browser-mcp" {
		t.Fatalf("expected browser-mcp, got %q", s.Name())
	}
}

// --- NavigationGuardedToolSet.Close ---

func TestNavigationGuardedToolSet_Close_NilReceiver(t *testing.T) {
	var s *NavigationGuardedToolSet
	if err := s.Close(); err != nil {
		t.Fatalf("nil receiver Close should return nil, got %v", err)
	}
}

func TestNavigationGuardedToolSet_Close_Delegates(t *testing.T) {
	closed := false
	inner := &mockToolSet{closeFn: func() error {
		closed = true
		return nil
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	if err := s.Close(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !closed {
		t.Fatal("expected Close to be delegated")
	}
}

func TestNavigationGuardedToolSet_Close_PropagatesError(t *testing.T) {
	inner := &mockToolSet{closeFn: func() error {
		return errors.New("close failed")
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	if err := s.Close(); err == nil || err.Error() != "close failed" {
		t.Fatalf("expected close failed error, got %v", err)
	}
}

// --- NavigationGuardedToolSet.Tools ---

func TestNavigationGuardedToolSet_Tools_NilReceiver(t *testing.T) {
	var s *NavigationGuardedToolSet
	if tools := s.Tools(context.Background()); tools != nil {
		t.Fatalf("nil receiver should return nil, got %v", tools)
	}
}

func TestNavigationGuardedToolSet_Tools_Empty(t *testing.T) {
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return nil
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	if tools := s.Tools(context.Background()); tools != nil {
		t.Fatalf("expected nil for empty tools, got %v", tools)
	}
}

func TestNavigationGuardedToolSet_Tools_NonNavigationPassthrough(t *testing.T) {
	innerTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_click"}
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{innerTool}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0] != innerTool {
		t.Fatal("non-navigation tool should pass through unchanged")
	}
}

func TestNavigationGuardedToolSet_Tools_NavigationWrapped(t *testing.T) {
	innerTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{innerTool}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0] == innerTool {
		t.Fatal("navigation tool should be wrapped, not passed through")
	}
	ct, ok := tools[0].(trpctool.CallableTool)
	if !ok {
		t.Fatal("wrapped tool should implement CallableTool")
	}
	if _, ok := ct.(*urlValidatingCallable); !ok {
		t.Fatal("expected *urlValidatingCallable")
	}
}

func TestNavigationGuardedToolSet_Tools_PrefixedNavigationWrapped(t *testing.T) {
	innerTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "bw_browser_navigate"}
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{innerTool}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0] == innerTool {
		t.Fatal("prefixed navigation tool should be wrapped")
	}
}

func TestNavigationGuardedToolSet_Tools_MixedNavigationAndNonNavigation(t *testing.T) {
	navigateTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		},
	}
	clickTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_click"}
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{navigateTool, clickTool}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	if tools[0] == navigateTool {
		t.Fatal("navigate tool should be wrapped")
	}
	if tools[1] != clickTool {
		t.Fatal("click tool should pass through unchanged")
	}
}

func TestNavigationGuardedToolSet_Tools_NilToolSkipped(t *testing.T) {
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{nil}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 slot (nil preserved), got %d", len(tools))
	}
	if tools[0] != nil {
		t.Fatal("nil tool should remain nil")
	}
}

func TestNavigationGuardedToolSet_Tools_NonCallableNavigationTool(t *testing.T) {
	// A navigation tool that does not implement CallableTool should pass
	// through unchanged (no wrapping possible).
	nonCallable := &struct{ trpctool.Tool }{
		Tool: &mockTool{decl: &trpctool.Declaration{Name: "browser_navigate"}},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{nonCallable}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if tools[0] != nonCallable {
		t.Fatal("non-callable navigation tool should pass through unchanged")
	}
}

// mockTool is a minimal Tool implementation for testing non-callable tools.
type mockTool struct {
	decl *trpctool.Declaration
}

func (m *mockTool) Declaration() *trpctool.Declaration { return m.decl }

// --- urlValidatingCallable.Call ---

func TestUrlValidatingCallable_Call_NilReceiver(t *testing.T) {
	var c *urlValidatingCallable
	_, err := c.Call(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil receiver")
	}
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected Internal error, got %v", err)
	}
}

func TestUrlValidatingCallable_Call_NilInner(t *testing.T) {
	c := &urlValidatingCallable{}
	_, err := c.Call(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil inner")
	}
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("expected Internal error, got %v", err)
	}
}

func TestUrlValidatingCallable_Call_BlockedURL(t *testing.T) {
	innerCalled := false
	inner := &mockCallableTool{
		callFn: func(_ context.Context, _ []byte) (any, error) {
			innerCalled = true
			return "ok", nil
		},
	}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{}, // blocks loopback by default
	}
	args := []byte(`{"url":"http://localhost"}`)
	_, err := c.Call(context.Background(), args)
	if err == nil {
		t.Fatal("expected error for blocked URL")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected BadRequest error, got %v", err)
	}
	if innerCalled {
		t.Fatal("inner tool should NOT be called when URL is blocked")
	}
}

func TestUrlValidatingCallable_Call_AllowedURL(t *testing.T) {
	innerCalled := false
	var receivedArgs []byte
	inner := &mockCallableTool{
		callFn: func(_ context.Context, jsonArgs []byte) (any, error) {
			innerCalled = true
			receivedArgs = jsonArgs
			return "navigated", nil
		},
	}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{}, // allows public URLs by default
	}
	args := []byte(`{"url":"https://example.com"}`)
	result, err := c.Call(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !innerCalled {
		t.Fatal("inner tool should be called when URL is allowed")
	}
	if string(receivedArgs) != string(args) {
		t.Fatalf("args should be forwarded unchanged, got %s", receivedArgs)
	}
	if result != "navigated" {
		t.Fatalf("expected 'navigated', got %v", result)
	}
}

func TestUrlValidatingCallable_Call_EmptyURL(t *testing.T) {
	// Empty URL is allowed (no-op navigation).
	innerCalled := false
	inner := &mockCallableTool{
		callFn: func(_ context.Context, _ []byte) (any, error) {
			innerCalled = true
			return "ok", nil
		},
	}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{},
	}
	// No url field at all.
	_, err := c.Call(context.Background(), []byte(`{"selector":"#btn"}`))
	if err != nil {
		t.Fatalf("unexpected error for empty URL: %v", err)
	}
	if !innerCalled {
		t.Fatal("inner should be called when URL is empty (allowed)")
	}
}

func TestUrlValidatingCallable_Call_PropagatesInnerError(t *testing.T) {
	innerErr := errors.New("navigation failed")
	inner := &mockCallableTool{
		callFn: func(_ context.Context, _ []byte) (any, error) {
			return nil, innerErr
		},
	}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{},
	}
	_, err := c.Call(context.Background(), []byte(`{"url":"https://example.com"}`))
	if !errors.Is(err, innerErr) {
		t.Fatalf("expected inner error to propagate, got %v", err)
	}
}

func TestUrlValidatingCallable_Call_BlockedDomain(t *testing.T) {
	innerCalled := false
	inner := &mockCallableTool{
		callFn: func(_ context.Context, _ []byte) (any, error) {
			innerCalled = true
			return "ok", nil
		},
	}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{BlockedDomains: []string{"evil.com"}},
	}
	_, err := c.Call(context.Background(), []byte(`{"url":"https://evil.com"}`))
	if err == nil {
		t.Fatal("expected error for blocked domain")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected BadRequest, got %v", err)
	}
	if innerCalled {
		t.Fatal("inner should NOT be called when domain is blocked")
	}
}

func TestUrlValidatingCallable_Call_NotInAllowedDomains(t *testing.T) {
	innerCalled := false
	inner := &mockCallableTool{
		callFn: func(_ context.Context, _ []byte) (any, error) {
			innerCalled = true
			return "ok", nil
		},
	}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{AllowedDomains: []string{"example.com"}},
	}
	_, err := c.Call(context.Background(), []byte(`{"url":"https://other.com"}`))
	if err == nil {
		t.Fatal("expected error for non-allowed domain")
	}
	if !apierror.IsCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected BadRequest, got %v", err)
	}
	if innerCalled {
		t.Fatal("inner should NOT be called when domain is not allowed")
	}
}

func TestUrlValidatingCallable_Call_InvalidJSON(t *testing.T) {
	// Invalid JSON yields empty URL, which is allowed (no-op navigation).
	innerCalled := false
	inner := &mockCallableTool{
		callFn: func(_ context.Context, _ []byte) (any, error) {
			innerCalled = true
			return "ok", nil
		},
	}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{},
	}
	_, err := c.Call(context.Background(), []byte("not-json"))
	if err != nil {
		t.Fatalf("unexpected error for invalid JSON: %v", err)
	}
	if !innerCalled {
		t.Fatal("inner should be called when URL extraction fails (treated as empty)")
	}
}

// --- urlValidatingCallable.Declaration ---

func TestUrlValidatingCallable_Declaration_NilReceiver(t *testing.T) {
	var c *urlValidatingCallable
	if c.Declaration() != nil {
		t.Fatal("nil receiver should return nil declaration")
	}
}

func TestUrlValidatingCallable_Declaration_NilInner(t *testing.T) {
	c := &urlValidatingCallable{}
	if c.Declaration() != nil {
		t.Fatal("nil inner should return nil declaration")
	}
}

func TestUrlValidatingCallable_Declaration_Delegates(t *testing.T) {
	expected := &trpctool.Declaration{
		Name:        "browser_navigate",
		Description: "Navigate to a URL",
	}
	inner := &mockCallableTool{
		declFn: func() *trpctool.Declaration { return expected },
	}
	c := &urlValidatingCallable{inner: inner, policy: NavigationPolicy{}}
	got := c.Declaration()
	if got != expected {
		t.Fatalf("expected declaration %v, got %v", expected, got)
	}
}

// --- integration: NavigationGuardedToolSet + urlValidatingCallable ---

func TestNavigationGuardedToolSet_Integration_BlockedURL(t *testing.T) {
	innerCalled := false
	innerTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		},
		callFn: func(_ context.Context, _ []byte) (any, error) {
			innerCalled = true
			return "ok", nil
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{innerTool}
	}}
	// Policy blocks loopback by default.
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	ct := tools[0].(trpctool.CallableTool)

	_, err := ct.Call(context.Background(), []byte(`{"url":"http://127.0.0.1"}`))
	if err == nil {
		t.Fatal("expected error for loopback URL")
	}
	if innerCalled {
		t.Fatal("inner tool should NOT be called for blocked URL")
	}
}

func TestNavigationGuardedToolSet_Integration_AllowedURL(t *testing.T) {
	innerCalled := false
	innerTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{Name: "browser_navigate"}
		},
		callFn: func(_ context.Context, _ []byte) (any, error) {
			innerCalled = true
			return "navigated", nil
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{innerTool}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	ct := tools[0].(trpctool.CallableTool)

	result, err := ct.Call(context.Background(), []byte(`{"url":"https://example.com"}`))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !innerCalled {
		t.Fatal("inner tool should be called for allowed URL")
	}
	if result != "navigated" {
		t.Fatalf("expected 'navigated', got %v", result)
	}
}

func TestNavigationGuardedToolSet_Integration_DeclarationPreserved(t *testing.T) {
	innerTool := &mockCallableTool{
		declFn: func() *trpctool.Declaration {
			return &trpctool.Declaration{
				Name:        "browser_navigate",
				Description: "Navigate to a URL",
			}
		},
	}
	inner := &mockToolSet{toolsFn: func(_ context.Context) []trpctool.Tool {
		return []trpctool.Tool{innerTool}
	}}
	s := NewNavigationGuardedToolSet(inner, NavigationPolicy{})
	tools := s.Tools(context.Background())
	ct := tools[0].(trpctool.CallableTool)

	decl := ct.Declaration()
	if decl == nil {
		t.Fatal("declaration should not be nil")
	}
	if decl.Name != "browser_navigate" {
		t.Fatalf("expected name browser_navigate, got %q", decl.Name)
	}
	if decl.Description != "Navigate to a URL" {
		t.Fatalf("expected description preserved, got %q", decl.Description)
	}
}

// --- error message sanity ---

func TestUrlValidatingCallable_Call_BlockedErrorMessageContainsHost(t *testing.T) {
	inner := &mockCallableTool{}
	c := &urlValidatingCallable{
		inner:  inner,
		policy: NavigationPolicy{}, // blocks loopback
	}
	_, err := c.Call(context.Background(), []byte(`{"url":"http://localhost:8080"}`))
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "localhost") {
		t.Fatalf("error message should contain host, got %q", err.Error())
	}
}
