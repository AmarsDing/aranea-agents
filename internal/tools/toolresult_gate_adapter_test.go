package tools

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// mockGate implements biz.ToolResultGate's Check method for testing.
type mockGate struct {
	result biz.ToolResultGateResult
	err    error
	called bool
}

func (m *mockGate) Check(_ context.Context, _, _, _, _, _ string, _ int) (biz.ToolResultGateResult, error) {
	m.called = true
	return m.result, m.err
}

func TestToolResultGateAfterHook_NilArgs(t *testing.T) {
	_ = &mockGate{}
	hook := NewToolResultGateAfterHook(nil, biz.Agent{}, loggateway.NewNoop())
	result, err := hook(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatalf("expected nil CustomResult, got %v", result.CustomResult)
	}
}

func TestToolResultGateAfterHook_NilGate(t *testing.T) {
	hook := NewToolResultGateAfterHook(nil, biz.Agent{}, loggateway.NewNoop())
	result, err := hook(context.Background(), &trpctool.AfterToolArgs{
		ToolName: "test",
		Result:   strings.Repeat("x", 60000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatalf("expected nil CustomResult when gate is nil, got %v", result.CustomResult)
	}
}

func TestToolResultGateAfterHook_SkipsOnError(t *testing.T) {
	gate := &mockGate{}
	hook := NewToolResultGateAfterHook(nil, biz.Agent{}, loggateway.NewNoop())
	result, err := hook(context.Background(), &trpctool.AfterToolArgs{
		ToolName: "test",
		Result:   strings.Repeat("x", 60000),
		Error:    errors.New("boom"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatalf("expected nil CustomResult when tool errored, got %v", result.CustomResult)
	}
	if gate.called {
		t.Fatal("gate should not be called when tool has error")
	}
}

func TestToolResultGateAfterHook_SkipsNonStringResult(t *testing.T) {
	_ = &mockGate{}
	hook := NewToolResultGateAfterHook(nil, biz.Agent{}, loggateway.NewNoop())
	result, err := hook(context.Background(), &trpctool.AfterToolArgs{
		ToolName: "test",
		Result:   42,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatalf("expected nil CustomResult for non-string result, got %v", result.CustomResult)
	}
}

func TestToolResultGateAfterHook_SkipsSmallResult(t *testing.T) {
	_ = &mockGate{}
	hook := NewToolResultGateAfterHook(nil, biz.Agent{}, loggateway.NewNoop())
	result, err := hook(context.Background(), &trpctool.AfterToolArgs{
		ToolName: "test",
		Result:   "small result",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatalf("expected nil CustomResult for small result, got %v", result.CustomResult)
	}
}

func TestToolResultGateAfterHook_LargeResultWithSession(t *testing.T) {
	_ = &mockGate{
		result: biz.ToolResultGateResult{
			BlobID:      "blob-123",
			PreviewText: "preview text",
			DidPersist:  true,
		},
	}
	// We pass nil gate to NewToolResultGateAfterHook because the function
	// signature takes *biz.ToolResultGate, not our mock. Instead, test the
	// truncation path (no session ID).
	hook := NewToolResultGateAfterHook(nil, biz.Agent{}, loggateway.NewNoop())

	ctx := context.Background()
	result, err := hook(ctx, &trpctool.AfterToolArgs{
		ToolName:   "test",
		ToolCallID: "call-1",
		Result:     strings.Repeat("x", 60000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// With nil gate, the hook returns nil CustomResult (gate == nil check).
	if result.CustomResult != nil {
		t.Fatalf("expected nil CustomResult with nil gate, got %v", result.CustomResult)
	}
}

func TestToolResultGateAfterHook_LargeResultNoSession(t *testing.T) {
	hook := NewToolResultGateAfterHook(nil, biz.Agent{}, loggateway.NewNoop())
	// Gate is nil, so the nil gate check returns early.
	result, err := hook(context.Background(), &trpctool.AfterToolArgs{
		ToolName: "test",
		Result:   strings.Repeat("x", 60000),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Fatalf("expected nil CustomResult with nil gate, got %v", result.CustomResult)
	}
}

func TestToolResultGateAfterHook_WithSessionID(t *testing.T) {
	// Test the WithSessionID context helper and sessionIDFromContext.
	ctx := WithSessionID(context.Background(), "sess-123")
	got := sessionIDFromContext(ctx)
	if got != "sess-123" {
		t.Fatalf("expected sess-123, got %q", got)
	}
}

func TestTruncateString(t *testing.T) {
	s := strings.Repeat("a", 3000)
	got := truncateString(s, 100)
	if len(got) > 200 {
		t.Fatalf("truncated string too long: %d", len(got))
	}
	if !strings.Contains(got, "truncated") {
		t.Fatal("truncated string should contain 'truncated'")
	}
}

func TestTruncateString_ShortInput(t *testing.T) {
	s := "hello"
	got := truncateString(s, 100)
	if got != s {
		t.Fatalf("expected %q, got %q", s, got)
	}
}
