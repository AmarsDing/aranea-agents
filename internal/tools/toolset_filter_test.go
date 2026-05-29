package tools

import (
	"context"
	"testing"
)

type mockTool struct {
	decl *Declaration
}

func (m *mockTool) Declaration() *Declaration {
	return m.decl
}

func TestToolFilterForPrefix_EmptyPrefix(t *testing.T) {
	fn := ToolFilterForPrefix("")
	if fn != nil {
		t.Fatal("expected nil for empty prefix")
	}
}

func TestToolFilterForPrefix_WhitespacePrefix(t *testing.T) {
	fn := ToolFilterForPrefix("   ")
	if fn != nil {
		t.Fatal("expected nil for whitespace-only prefix")
	}
}

func TestToolFilterForPrefix_Match(t *testing.T) {
	fn := ToolFilterForPrefix("fs_")
	if fn == nil {
		t.Fatal("expected non-nil filter")
	}
	ctx := context.Background()
	tool := &mockTool{decl: &Declaration{Name: "fs_read_file"}}
	if !fn(ctx, tool) {
		t.Fatal("expected match for fs_read_file with prefix fs_")
	}
}

func TestToolFilterForPrefix_NoMatch(t *testing.T) {
	fn := ToolFilterForPrefix("fs_")
	ctx := context.Background()
	tool := &mockTool{decl: &Declaration{Name: "web_search"}}
	if fn(ctx, tool) {
		t.Fatal("expected no match for web_search with prefix fs_")
	}
}

func TestToolFilterForPrefix_NilTool(t *testing.T) {
	fn := ToolFilterForPrefix("fs_")
	ctx := context.Background()
	if fn(ctx, nil) {
		t.Fatal("expected false for nil tool")
	}
}

func TestToolFilterForPrefix_NilDeclaration(t *testing.T) {
	fn := ToolFilterForPrefix("fs_")
	ctx := context.Background()
	tool := &mockTool{decl: nil}
	if fn(ctx, tool) {
		t.Fatal("expected false for nil declaration")
	}
}

func TestToolFilterForPrefix_TrimmedPrefix(t *testing.T) {
	fn := ToolFilterForPrefix("  fs_  ")
	if fn == nil {
		t.Fatal("expected non-nil filter for trimmed prefix")
	}
	ctx := context.Background()
	tool := &mockTool{decl: &Declaration{Name: "fs_read_file"}}
	if !fn(ctx, tool) {
		t.Fatal("expected match after trimming prefix whitespace")
	}
}
