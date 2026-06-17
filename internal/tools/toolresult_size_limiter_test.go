package tools

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func TestNewOutputSizeLimiterHook_WithinLimit(t *testing.T) {
	hook := NewOutputSizeLimiterHook(1000, loggateway.NewNoop())

	args := &trpctool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "test_tool",
		Result:     "short result",
	}
	result, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Within limit — no CustomResult (pass through)
	if result.CustomResult != nil {
		t.Errorf("expected nil CustomResult for within-limit output, got %v", result.CustomResult)
	}
}

func TestNewOutputSizeLimiterHook_ExceedsLimit(t *testing.T) {
	maxChars := 100
	hook := NewOutputSizeLimiterHook(maxChars, loggateway.NewNoop())

	longResult := strings.Repeat("a", 200)
	args := &trpctool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "test_tool",
		Result:     longResult,
	}
	result, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult == nil {
		t.Fatal("expected non-nil CustomResult for exceeded output")
	}
	truncated, ok := result.CustomResult.(string)
	if !ok {
		t.Fatalf("expected string CustomResult, got %T", result.CustomResult)
	}
	// Should contain the truncation marker
	if !strings.Contains(truncated, "[output truncated:") {
		t.Error("expected truncation marker in result")
	}
	// Should start with the first maxChars of the original
	if !strings.HasPrefix(truncated, strings.Repeat("a", maxChars)) {
		t.Error("expected truncated result to start with original content")
	}
}

func TestNewOutputSizeLimiterHook_ExactLimit(t *testing.T) {
	maxChars := 100
	hook := NewOutputSizeLimiterHook(maxChars, loggateway.NewNoop())

	exactResult := strings.Repeat("x", maxChars)
	args := &trpctool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "test_tool",
		Result:     exactResult,
	}
	result, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Exact limit — should pass through unchanged
	if result.CustomResult != nil {
		t.Errorf("expected nil CustomResult for exact-limit output, got %v", result.CustomResult)
	}
}

func TestNewOutputSizeLimiterHook_NonStringResult(t *testing.T) {
	hook := NewOutputSizeLimiterHook(10, loggateway.NewNoop())

	args := &trpctool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "test_tool",
		Result:     map[string]any{"key": "value"},
	}
	result, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-string result — pass through unchanged
	if result.CustomResult != nil {
		t.Errorf("expected nil CustomResult for non-string result, got %v", result.CustomResult)
	}
}

func TestNewOutputSizeLimiterHook_ExecutionError(t *testing.T) {
	hook := NewOutputSizeLimiterHook(10, loggateway.NewNoop())

	args := &trpctool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "test_tool",
		Result:     strings.Repeat("a", 100),
		Error:      fmt.Errorf("tool execution failed"),
	}
	result, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Error present — pass through unchanged
	if result.CustomResult != nil {
		t.Errorf("expected nil CustomResult when error is present, got %v", result.CustomResult)
	}
}

func TestNewOutputSizeLimiterHook_NilArgs(t *testing.T) {
	hook := NewOutputSizeLimiterHook(10, loggateway.NewNoop())

	result, err := hook(context.Background(), nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Errorf("expected nil CustomResult for nil args, got %v", result.CustomResult)
	}
}

func TestNewOutputSizeLimiterHook_DefaultMaxChars(t *testing.T) {
	// maxChars <= 0 should default to 50000
	hook := NewOutputSizeLimiterHook(0, loggateway.NewNoop())

	// 50000 chars should be within limit
	args := &trpctool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "test_tool",
		Result:     strings.Repeat("a", 50000),
	}
	result, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult != nil {
		t.Errorf("expected nil CustomResult for within-limit output, got %v", result.CustomResult)
	}

	// 50001 chars should be truncated
	args2 := &trpctool.AfterToolArgs{
		ToolCallID: "call_2",
		ToolName:   "test_tool",
		Result:     strings.Repeat("a", 50001),
	}
	result2, err := hook(context.Background(), args2)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result2.CustomResult == nil {
		t.Fatal("expected non-nil CustomResult for exceeded output")
	}
}

func TestNewOutputSizeLimiterHook_UTF8Truncation(t *testing.T) {
	maxChars := 5
	hook := NewOutputSizeLimiterHook(maxChars, loggateway.NewNoop())

	// Multi-byte UTF-8 characters (Chinese)
	utf8Result := "你好世界你好世界" // 6 Chinese characters, each 3 bytes
	args := &trpctool.AfterToolArgs{
		ToolCallID: "call_1",
		ToolName:   "test_tool",
		Result:     utf8Result,
	}
	result, err := hook(context.Background(), args)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.CustomResult == nil {
		t.Fatal("expected non-nil CustomResult for exceeded output")
	}
	truncated, ok := result.CustomResult.(string)
	if !ok {
		t.Fatalf("expected string CustomResult, got %T", result.CustomResult)
	}
	// Should start with first 5 characters (你好世界你)
	expected := "你好世界你"
	if !strings.HasPrefix(truncated, expected) {
		t.Errorf("expected prefix %q, got %q", expected, truncated[:len(expected)])
	}
}

func TestTruncateRunes(t *testing.T) {
	tests := []struct {
		input    string
		maxChars int
		want     string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello", 3, "hel"},
		{"你好世界", 2, "你好"},
		{"", 5, ""},
	}
	for _, tt := range tests {
		got := truncateRunes(tt.input, tt.maxChars)
		if got != tt.want {
			t.Errorf("truncateRunes(%q, %d) = %q, want %q", tt.input, tt.maxChars, got, tt.want)
		}
	}
}
