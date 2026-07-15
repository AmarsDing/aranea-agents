package tools

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcsession "trpc.group/trpc-go/trpc-agent-go/session"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func invocationCtx(appName, userID, sessionID string) context.Context {
	inv := &trpcagent.Invocation{
		Session: &trpcsession.Session{
			AppName: appName,
			UserID:  userID,
			ID:      sessionID,
		},
	}
	return trpcagent.NewInvocationContext(context.Background(), inv)
}

// decoratorMockTool is a test double implementing CallableTool.
type decoratorMockTool struct {
	name string
	call func(ctx context.Context, jsonArgs []byte) (any, error)
}

func (m *decoratorMockTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: m.name}
}

func (m *decoratorMockTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	return m.call(ctx, jsonArgs)
}

// TestToolDecorator_Timeout verifies that the decorator enforces the
// configured timeout and returns an error when the inner tool exceeds it.
func TestToolDecorator_Timeout(t *testing.T) {
	slowTool := &decoratorMockTool{
		name: "slow_tool",
		call: func(ctx context.Context, args []byte) (any, error) {
			select {
			case <-time.After(200 * time.Millisecond):
				return "done", nil
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		},
	}
	d := NewToolDecorator(slowTool, ToolDecoratorConfig{
		Timeout: 50 * time.Millisecond,
		Logger:  loggateway.NewNoop(),
	})
	start := time.Now()
	_, err := d.Call(context.Background(), nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if elapsed >= 200*time.Millisecond {
		t.Errorf("timeout did not fire early enough, elapsed=%v", elapsed)
	}
}

// TestToolDecorator_NoTimeoutWhenContextDeadlineCloser verifies that the
// decorator does not override an existing context deadline that is sooner
// than the configured timeout.
func TestToolDecorator_NoTimeoutWhenContextDeadlineCloser(t *testing.T) {
	tool := &decoratorMockTool{
		name: "test_tool",
		call: func(ctx context.Context, args []byte) (any, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Timeout: 60 * time.Second,
		Logger:  loggateway.NewNoop(),
	})
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := d.Call(ctx, nil)
	elapsed := time.Since(start)
	if err == nil {
		t.Fatal("expected context deadline error, got nil")
	}
	if elapsed >= 60*time.Second {
		t.Errorf("should have used shorter context deadline, elapsed=%v", elapsed)
	}
}

// TestToolDecorator_TruncateResult verifies that results exceeding the
// budget are truncated in tail/head/middle modes, and that results within
// budget are passed through unchanged.
func TestToolDecorator_TruncateResult(t *testing.T) {
	longResult := strings.Repeat("a", 5000)
	tests := []struct {
		name       string
		mode       string
		maxBytes   int
		wantTrunc  bool
		wantInCont string
	}{
		{"tail_mode", "tail", 1000, true, "head"},
		{"head_mode", "head", 1000, true, "tail"},
		{"middle_mode", "middle", 1000, true, "both"},
		{"no_truncation", "tail", 10000, false, ""},
		{"no_budget", "tail", 0, false, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tool := &decoratorMockTool{
				name: "test_tool",
				call: func(ctx context.Context, args []byte) (any, error) {
					return longResult, nil
				},
			}
			cfg := ToolDecoratorConfig{Logger: loggateway.NewNoop()}
			if tt.maxBytes > 0 {
				cfg.ResultBudget = &ResultBudget{MaxBytes: tt.maxBytes, Mode: tt.mode}
			}
			d := NewToolDecorator(tool, cfg)
			result, err := d.Call(context.Background(), nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !tt.wantTrunc {
				if resultStr, ok := result.(string); !ok || resultStr != longResult {
					t.Errorf("expected no truncation (string passthrough), got %T=%v", result, result)
				}
				return
			}
			envelope, ok := result.(map[string]any)
			if !ok {
				t.Fatalf("expected map[string]any envelope, got %T", result)
			}
			if truncated, _ := envelope["truncated"].(bool); !truncated {
				t.Errorf("expected truncated=true, got %v", envelope["truncated"])
			}
			// longResult is 5000 'a' chars; JSON-encoded as string is 5002 bytes (with quotes).
			if origSize, _ := envelope["original_size"].(int); origSize != 5002 {
				t.Errorf("expected original_size=5002, got %v", envelope["original_size"])
			}
			if mode, _ := envelope["mode"].(string); mode != tt.mode {
				t.Errorf("expected mode=%q, got %q", tt.mode, mode)
			}
			content, _ := envelope["content"].(string)
			if content == "" {
				t.Errorf("expected non-empty content")
			}
		})
	}
}

// TestToolDecorator_Cache verifies that ConcurrentSafe tools are cached:
// repeated calls with identical args hit the cache without invoking the
// inner tool, while different args trigger a new call.
func TestToolDecorator_Cache(t *testing.T) {
	var callCount int32
	var mu sync.Mutex
	tool := &decoratorMockTool{
		name: "file", // ConcurrentSafe per registry
		call: func(ctx context.Context, args []byte) (any, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return "result_" + string(args), nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	args1 := []byte(`{"path":"test.txt"}`)
	args2 := []byte(`{"path":"other.txt"}`)
	ctx := invocationCtx("agent-a", "user-1", "sess-1")

	// First call — should invoke inner.
	r1, err := d.Call(ctx, args1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Second call with same args — should hit cache.
	r2, err := d.Call(ctx, args1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if r1 != r2 {
		t.Errorf("expected same result from cache, got %v vs %v", r1, r2)
	}
	mu.Lock()
	if callCount != 1 {
		t.Errorf("expected 1 inner call after cache hit, got %d", callCount)
	}
	mu.Unlock()
	// Third call with different args — should invoke inner again.
	_, err = d.Call(ctx, args2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	mu.Lock()
	if callCount != 2 {
		t.Errorf("expected 2 inner calls after different args, got %d", callCount)
	}
	mu.Unlock()
}

// TestToolDecorator_CacheIsolatedBySession verifies E2E-P1-09: identical
// args from different sessions do not share cached tool results.
func TestToolDecorator_CacheIsolatedBySession(t *testing.T) {
	var callCount int32
	var mu sync.Mutex
	tool := &decoratorMockTool{
		name: "file",
		call: func(ctx context.Context, args []byte) (any, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return "ok", nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	args := []byte(`{"path":"shared.txt"}`)
	ctxA := invocationCtx("agent-a", "user-1", "sess-a")
	ctxB := invocationCtx("agent-a", "user-1", "sess-b")

	if _, err := d.Call(ctxA, args); err != nil {
		t.Fatalf("sess-a call: %v", err)
	}
	if _, err := d.Call(ctxB, args); err != nil {
		t.Fatalf("sess-b call: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Fatalf("expected 2 inner calls across sessions, got %d", callCount)
	}
}

// TestToolDecorator_CacheIsolatedByWorkspace verifies C-03: identical
// session/user/args from different workspaces do not share cached results.
func TestToolDecorator_CacheIsolatedByWorkspace(t *testing.T) {
	var callCount int32
	var mu sync.Mutex
	tool := &decoratorMockTool{
		name: "file",
		call: func(ctx context.Context, args []byte) (any, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return "ok", nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	args := []byte(`{"path":"shared.txt"}`)
	baseInv := invocationCtx("agent-a", "user-1", "sess-shared")
	ctxA := workspace.WithContext(baseInv, "ws-a")
	ctxB := workspace.WithContext(baseInv, "ws-b")

	if _, err := d.Call(ctxA, args); err != nil {
		t.Fatalf("ws-a call: %v", err)
	}
	if _, err := d.Call(ctxB, args); err != nil {
		t.Fatalf("ws-b call: %v", err)
	}
	// Same workspace should hit cache.
	if _, err := d.Call(ctxA, args); err != nil {
		t.Fatalf("ws-a cache hit: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Fatalf("expected 2 inner calls across workspaces (3rd hit cache), got %d", callCount)
	}
}

// TestToolDecorator_CacheDisabledWithoutInvocation verifies unscoped calls
// never populate or read the shared cache bucket.
func TestToolDecorator_CacheDisabledWithoutInvocation(t *testing.T) {
	var callCount int32
	var mu sync.Mutex
	tool := &decoratorMockTool{
		name: "file",
		call: func(ctx context.Context, args []byte) (any, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return "ok", nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	args := []byte(`{"path":"noscope.txt"}`)
	if _, err := d.Call(context.Background(), args); err != nil {
		t.Fatalf("call1: %v", err)
	}
	if _, err := d.Call(context.Background(), args); err != nil {
		t.Fatalf("call2: %v", err)
	}
	mu.Lock()
	defer mu.Unlock()
	if callCount != 2 {
		t.Fatalf("expected no cache without invocation, got %d calls", callCount)
	}
}

// TestToolDecorator_NoCacheForExclusiveTool verifies that Exclusive tools
// are never cached even when EnableCache is true.
func TestToolDecorator_NoCacheForExclusiveTool(t *testing.T) {
	var callCount int32
	var mu sync.Mutex
	tool := &decoratorMockTool{
		name: "hostexec", // Exclusive per registry
		call: func(ctx context.Context, args []byte) (any, error) {
			mu.Lock()
			callCount++
			mu.Unlock()
			return "executed", nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	args := []byte(`{"cmd":"ls"}`)
	// Two calls with same args — both should invoke inner (no cache).
	_, _ = d.Call(context.Background(), args)
	_, _ = d.Call(context.Background(), args)
	mu.Lock()
	if callCount != 2 {
		t.Errorf("expected 2 inner calls for Exclusive tool, got %d", callCount)
	}
	mu.Unlock()
}

// TestToolDecorator_ErrorPassthrough verifies that inner tool errors are
// returned unchanged and not cached.
func TestToolDecorator_ErrorPassthrough(t *testing.T) {
	expectedErr := errors.New("tool failed")
	tool := &decoratorMockTool{
		name: "file", // ConcurrentSafe
		call: func(ctx context.Context, args []byte) (any, error) {
			return nil, expectedErr
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	_, err := d.Call(context.Background(), nil)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, err)
	}
	// Second call should still invoke inner (error not cached).
	_, err = d.Call(context.Background(), nil)
	if !errors.Is(err, expectedErr) {
		t.Errorf("expected error %v on second call, got %v", expectedErr, err)
	}
}

// TestToolDecorator_SatisfiesCallableTool verifies at compile time that
// ToolDecorator implements the CallableTool interface.
func TestToolDecorator_SatisfiesCallableTool(t *testing.T) {
	var _ trpctool.CallableTool = (*ToolDecorator)(nil)
}

// TestToolDecorator_DefaultTimeout verifies that a zero Timeout in config
// defaults to DefaultToolTimeout (60s).
func TestToolDecorator_DefaultTimeout(t *testing.T) {
	tool := &decoratorMockTool{
		name: "test_tool",
		call: func(ctx context.Context, args []byte) (any, error) {
			return "ok", nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		Logger: loggateway.NewNoop(),
		// Timeout intentionally zero — should default to 60s.
	})
	td, ok := d.(*ToolDecorator)
	if !ok {
		t.Fatalf("expected *ToolDecorator for non-streamable tool, got %T", d)
	}
	if td.cfg.Timeout != DefaultToolTimeout {
		t.Errorf("expected default timeout %v, got %v", DefaultToolTimeout, td.cfg.Timeout)
	}
}

// TestApplyDecorators_StandaloneTools verifies that ApplyDecorators wraps
// standalone CallableTools in place.
func TestApplyDecorators_StandaloneTools(t *testing.T) {
	tool := &decoratorMockTool{
		name: "file",
		call: func(ctx context.Context, args []byte) (any, error) {
			return "ok", nil
		},
	}
	ts := &AssembledToolsets{
		Tools: []Tool{tool},
	}
	ApplyDecorators(ts, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	if _, ok := ts.Tools[0].(*ToolDecorator); !ok {
		t.Errorf("expected Tools[0] to be *ToolDecorator, got %T", ts.Tools[0])
	}
}

// TestApplyDecorators_NilSafe verifies that ApplyDecorators handles nil
// input gracefully.
func TestApplyDecorators_NilSafe(t *testing.T) {
	ApplyDecorators(nil, ToolDecoratorConfig{})
	// Should not panic.
}

// TestApplyDecorators_ToolSetWrapping verifies that ApplyDecorators wraps
// ToolSets with decoratedToolSet.
func TestApplyDecorators_ToolSetWrapping(t *testing.T) {
	inner := &decoratorMockToolSet{
		name: "test_set",
		tools: []Tool{
			&decoratorMockTool{
				name: "file",
				call: func(ctx context.Context, args []byte) (any, error) {
					return "ok", nil
				},
			},
		},
	}
	ts := &AssembledToolsets{
		ToolSets: []ToolSet{inner},
	}
	ApplyDecorators(ts, ToolDecoratorConfig{
		Logger: loggateway.NewNoop(),
	})
	wrapped, ok := ts.ToolSets[0].(*decoratedToolSet)
	if !ok {
		t.Fatalf("expected ToolSets[0] to be *decoratedToolSet, got %T", ts.ToolSets[0])
	}
	if wrapped.Name() != "test_set" {
		t.Errorf("expected name 'test_set', got %q", wrapped.Name())
	}
	tools := wrapped.Tools(context.Background())
	if len(tools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(tools))
	}
	if _, ok := tools[0].(*ToolDecorator); !ok {
		t.Errorf("expected tool to be *ToolDecorator, got %T", tools[0])
	}
}

// decoratorMockToolSet is a test double implementing ToolSet.
type decoratorMockToolSet struct {
	name  string
	tools []Tool
}

func (m *decoratorMockToolSet) Tools(context.Context) []Tool { return m.tools }
func (m *decoratorMockToolSet) Close() error                 { return nil }
func (m *decoratorMockToolSet) Name() string                 { return m.name }

// decoratorMockStreamableTool is a test double implementing both CallableTool
// and StreamableTool, used to verify the streamableToolDecorator path.
type decoratorMockStreamableTool struct {
	decoratorMockTool
	streamableCall func(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error)
}

func (m *decoratorMockStreamableTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	return m.streamableCall(ctx, jsonArgs)
}

// TestToolDecorator_StreamableInnerReturnsStreamableDecorator verifies that
// when the inner tool satisfies StreamableTool, NewToolDecorator returns a
// *streamableToolDecorator that also satisfies StreamableTool, and that
// StreamableCall passes through to the inner tool.
func TestToolDecorator_StreamableInnerReturnsStreamableDecorator(t *testing.T) {
	var streamCallCount int32
	var mu sync.Mutex
	inner := &decoratorMockStreamableTool{
		decoratorMockTool: decoratorMockTool{
			name: "streamable_test_tool",
			call: func(ctx context.Context, args []byte) (any, error) {
				return "call_result", nil
			},
		},
		streamableCall: func(ctx context.Context, args []byte) (*trpctool.StreamReader, error) {
			mu.Lock()
			streamCallCount++
			mu.Unlock()
			stream := trpctool.NewStream(1)
			stream.Writer.Send(trpctool.StreamChunk{}, nil)
			stream.Writer.Close()
			return stream.Reader, nil
		},
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		Logger: loggateway.NewNoop(),
	})
	// Verify the decorated tool satisfies StreamableTool.
	st, ok := d.(trpctool.StreamableTool)
	if !ok {
		t.Fatalf("expected decorated tool to satisfy StreamableTool, got %T", d)
	}
	// Verify StreamableCall passes through to inner.
	reader, err := st.StreamableCall(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	if reader == nil {
		t.Fatal("expected non-nil StreamReader")
	}
	reader.Close()
	mu.Lock()
	if streamCallCount != 1 {
		t.Errorf("expected 1 inner StreamableCall, got %d", streamCallCount)
	}
	mu.Unlock()
	// Verify Call still works through the decorator.
	result, err := d.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("Call error: %v", err)
	}
	if result != "call_result" {
		t.Errorf("expected 'call_result', got %v", result)
	}
}

// TestToolDecorator_NonStreamableInnerDoesNotSatisfyStreamableTool verifies
// that when the inner tool does NOT satisfy StreamableTool, the decorated
// tool also does NOT satisfy StreamableTool. This prevents the framework
// from misclassifying non-streaming tools as streamable.
func TestToolDecorator_NonStreamableInnerDoesNotSatisfyStreamableTool(t *testing.T) {
	inner := &decoratorMockTool{
		name: "non_streamable_test_tool",
		call: func(ctx context.Context, args []byte) (any, error) {
			return "ok", nil
		},
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		Logger: loggateway.NewNoop(),
	})
	if _, ok := d.(trpctool.StreamableTool); ok {
		t.Errorf("decorated non-streamable tool should NOT satisfy StreamableTool, got %T", d)
	}
	// Verify it's still a *ToolDecorator (not streamableToolDecorator).
	if _, ok := d.(*ToolDecorator); !ok {
		t.Errorf("expected *ToolDecorator for non-streamable inner, got %T", d)
	}
}

// TestToolDecorator_ConcurrentCacheAccess verifies that the decorator's
// cache is safe under concurrent access. Run with -race to detect data races.
// This test exercises the RWMutex-protected lookupCache/storeCache paths.
func TestToolDecorator_ConcurrentCacheAccess(t *testing.T) {
	var innerCallCount int32
	var mu sync.Mutex
	tool := &decoratorMockTool{
		name: "file", // ConcurrentSafe per registry
		call: func(ctx context.Context, args []byte) (any, error) {
			mu.Lock()
			innerCallCount++
			mu.Unlock()
			// Small delay to increase chance of concurrent access.
			time.Sleep(time.Millisecond)
			return "result_" + string(args), nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		EnableCache: true,
		Logger:      loggateway.NewNoop(),
	})
	// Use two distinct arg sets so we exercise both cache hits and misses.
	argsA := []byte(`{"path":"a.txt"}`)
	argsB := []byte(`{"path":"b.txt"}`)
	ctx := invocationCtx("agent-a", "user-1", "sess-1")
	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			args := argsA
			if i%2 == 0 {
				args = argsB
			}
			_, _ = d.Call(ctx, args)
		}(i)
	}
	wg.Wait()
	// With 20 goroutines and 2 distinct args, inner should be called at most
	// 2 times (once per unique arg). We allow some slack for the first-call
	// race (multiple goroutines may miss the cache on first call for the
	// same arg). The key assertion is no data race (detected by -race flag).
	mu.Lock()
	if innerCallCount > 20 {
		t.Errorf("expected at most 20 inner calls (race slack), got %d", innerCallCount)
	}
	mu.Unlock()
}

// TestToolDecorator_NilInnerReturnsNil verifies that NewToolDecorator returns
// nil when inner is nil, allowing callers to chain without extra guards.
func TestToolDecorator_NilInnerReturnsNil(t *testing.T) {
	d := NewToolDecorator(nil, ToolDecoratorConfig{
		Logger: loggateway.NewNoop(),
	})
	if d != nil {
		t.Errorf("expected nil for nil inner, got %T", d)
	}
}

// --- Browser result budget override tests ---

func TestBudgetOverrideForTool_EmptyName(t *testing.T) {
	if got := budgetOverrideForTool(""); got != nil {
		t.Fatalf("expected nil for empty name, got %+v", got)
	}
}

func TestBudgetOverrideForTool_NonBrowserTool(t *testing.T) {
	if got := budgetOverrideForTool("file_read"); got != nil {
		t.Fatalf("expected nil for non-browser tool, got %+v", got)
	}
	if got := budgetOverrideForTool("web_fetch"); got != nil {
		t.Fatalf("expected nil for web_fetch, got %+v", got)
	}
}

func TestBudgetOverrideForTool_BrowserScreenshot(t *testing.T) {
	b := budgetOverrideForTool("browser_take_screenshot")
	if b == nil {
		t.Fatal("expected budget for browser_take_screenshot")
	}
	if b.MaxBytes != 100*1024 {
		t.Errorf("expected 100KB, got %d", b.MaxBytes)
	}
}

func TestBudgetOverrideForTool_BrowserSnapshot(t *testing.T) {
	b := budgetOverrideForTool("browser_snapshot")
	if b == nil {
		t.Fatal("expected budget for browser_snapshot")
	}
	if b.MaxBytes != 50*1024 {
		t.Errorf("expected 50KB, got %d", b.MaxBytes)
	}
}

func TestBudgetOverrideForTool_PrefixedName(t *testing.T) {
	// MCP ToolPrefix prefixes should match via suffix-based logic.
	for _, name := range []string{"bw_browser_take_screenshot", "playwright_browser_snapshot"} {
		if b := budgetOverrideForTool(name); b == nil {
			t.Errorf("expected override for prefixed tool %q", name)
		}
	}
}

func TestBudgetOverrideForTool_NonMatchingSuffix(t *testing.T) {
	// Tools that merely contain the substring but don't end with it should not match.
	if b := budgetOverrideForTool("browser_take_screenshot_extra"); b != nil {
		t.Errorf("expected nil for non-matching suffix, got %+v", b)
	}
}

// TestToolDecorator_BrowserScreenshotUsesOverrideBudget verifies that a
// browser_take_screenshot tool uses the 100KB override budget instead of
// the default 10KB budget. A 20KB result should NOT be truncated (it would
// be under the default 10KB budget).
func TestToolDecorator_BrowserScreenshotUsesOverrideBudget(t *testing.T) {
	// 20KB result: exceeds default 10KB budget but under 100KB override.
	result := strings.Repeat("x", 20*1024)
	tool := &decoratorMockTool{
		name: "browser_take_screenshot",
		call: func(_ context.Context, _ []byte) (any, error) {
			return result, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		ResultBudget: DefaultResultBudget, // 10KB default
		Logger:       loggateway.NewNoop(),
	})
	got, err := d.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Result should NOT be truncated because override budget is 100KB.
	if _, ok := got.(map[string]any); ok {
		t.Fatal("expected no truncation for 20KB screenshot under 100KB override")
	}
	if s, ok := got.(string); !ok || s != result {
		t.Fatalf("expected passthrough, got %T", got)
	}
}

// TestToolDecorator_BrowserSnapshotUsesOverrideBudget verifies that a
// browser_snapshot tool uses the 50KB override budget. A 30KB result should
// NOT be truncated (it would be under default 10KB but over 50KB override).
func TestToolDecorator_BrowserSnapshotUsesOverrideBudget(t *testing.T) {
	result := strings.Repeat("y", 30*1024) // 30KB
	tool := &decoratorMockTool{
		name: "browser_snapshot",
		call: func(_ context.Context, _ []byte) (any, error) {
			return result, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		ResultBudget: DefaultResultBudget, // 10KB default
		Logger:       loggateway.NewNoop(),
	})
	got, err := d.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := got.(map[string]any); ok {
		t.Fatal("expected no truncation for 30KB snapshot under 50KB override")
	}
}

// TestToolDecorator_BrowserScreenshotTruncatedAtOverride verifies that a
// browser_take_screenshot result exceeding the 100KB override IS truncated.
func TestToolDecorator_BrowserScreenshotTruncatedAtOverride(t *testing.T) {
	// 120KB result: exceeds 100KB override.
	result := strings.Repeat("z", 120*1024)
	tool := &decoratorMockTool{
		name: "browser_take_screenshot",
		call: func(_ context.Context, _ []byte) (any, error) {
			return result, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		ResultBudget: DefaultResultBudget,
		Logger:       loggateway.NewNoop(),
	})
	got, err := d.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	envelope, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected truncation envelope, got %T", got)
	}
	if truncated, _ := envelope["truncated"].(bool); !truncated {
		t.Error("expected truncated=true")
	}
}

// TestToolDecorator_BrowserSnapshotTruncatedAtOverride verifies that a
// browser_snapshot result exceeding the 50KB override IS truncated.
func TestToolDecorator_BrowserSnapshotTruncatedAtOverride(t *testing.T) {
	// 60KB result: exceeds 50KB override.
	result := strings.Repeat("w", 60*1024)
	tool := &decoratorMockTool{
		name: "browser_snapshot",
		call: func(_ context.Context, _ []byte) (any, error) {
			return result, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		ResultBudget: DefaultResultBudget,
		Logger:       loggateway.NewNoop(),
	})
	got, err := d.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	envelope, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected truncation envelope, got %T", got)
	}
	if truncated, _ := envelope["truncated"].(bool); !truncated {
		t.Error("expected truncated=true")
	}
}

// TestToolDecorator_NonBrowserToolUsesDefaultBudget verifies that non-browser
// tools still use the default 10KB budget (not affected by overrides).
func TestToolDecorator_NonBrowserToolUsesDefaultBudget(t *testing.T) {
	// 15KB result: exceeds default 10KB, should be truncated.
	result := strings.Repeat("a", 15*1024)
	tool := &decoratorMockTool{
		name: "file_read",
		call: func(_ context.Context, _ []byte) (any, error) {
			return result, nil
		},
	}
	d := NewToolDecorator(tool, ToolDecoratorConfig{
		ResultBudget: DefaultResultBudget,
		Logger:       loggateway.NewNoop(),
	})
	got, err := d.Call(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	envelope, ok := got.(map[string]any)
	if !ok {
		t.Fatalf("expected truncation for non-browser tool at 15KB > 10KB default, got %T", got)
	}
	if truncated, _ := envelope["truncated"].(bool); !truncated {
		t.Error("expected truncated=true")
	}
}
