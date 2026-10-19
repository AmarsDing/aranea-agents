package tools

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"strings"
	"testing"
)

// TestToolDecorator_BrowserScreenshotUsesOverrideBudget verifies that a
// browser_take_screenshot tool uses the 100KB override budget instead of
// the default 10KB budget. A 20KB result should NOT be truncated (it would
// be under the default 10KB budget).
func TestToolDecorator_BrowserScreenshotUsesOverrideBudget(t *testing.T) {

	result := strings.Repeat("x", 20*1024)
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
	result := strings.Repeat("y", 30*1024)
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
	if _, ok := got.(map[string]any); ok {
		t.Fatal("expected no truncation for 30KB snapshot under 50KB override")
	}
}

// TestToolDecorator_BrowserScreenshotTruncatedAtOverride verifies that a
// browser_take_screenshot result exceeding the 100KB override IS truncated.
func TestToolDecorator_BrowserScreenshotTruncatedAtOverride(t *testing.T) {

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
