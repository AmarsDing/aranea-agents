package trpc_test

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/trpc"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type mockToolSet struct {
	nameFn  func() string
	closeFn func() error
	toolsFn func(ctx context.Context) []trpctool.Tool
}

func (m *mockToolSet) Name() string {
	if m.nameFn != nil {
		return m.nameFn()
	}
	return ""
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

type mockTool struct {
	decl *trpctool.Declaration
}

func (m *mockTool) Declaration() *trpctool.Declaration {
	return m.decl
}

func TestApplyConfirmationPolicy_ToolSets(t *testing.T) {
	innerTool := &mockTool{decl: &trpctool.Declaration{Name: "email_send", Description: "send email"}}
	ts := &tools.AssembledToolsets{
		ToolSets: []tools.ToolSet{
			&mockToolSet{
				nameFn: func() string { return "email" },
				toolsFn: func(_ context.Context) []trpctool.Tool {
					return []trpctool.Tool{innerTool}
				},
			},
		},
	}
	trpc.ApplyConfirmationPolicy(ts, map[string]bool{"email_send": true})

	wrapped := ts.ToolSets[0]
	wrappedTools := wrapped.Tools(context.Background())
	if len(wrappedTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(wrappedTools))
	}
	d := wrappedTools[0].Declaration()
	if d == nil {
		t.Fatal("declaration should not be nil")
	}
	if !strings.Contains(d.Description, "Requires explicit user approval") {
		t.Fatalf("description = %q, should contain approval text", d.Description)
	}
}

func TestApplyConfirmationPolicy_ToolSetsNotRequired(t *testing.T) {
	innerTool := &mockTool{decl: &trpctool.Declaration{Name: "read_file", Description: "read a file"}}
	ts := &tools.AssembledToolsets{
		ToolSets: []tools.ToolSet{
			&mockToolSet{
				nameFn: func() string { return "filesystem" },
				toolsFn: func(_ context.Context) []trpctool.Tool {
					return []trpctool.Tool{innerTool}
				},
			},
		},
	}
	trpc.ApplyConfirmationPolicy(ts, map[string]bool{"shell_exec": true})

	wrapped := ts.ToolSets[0]
	wrappedTools := wrapped.Tools(context.Background())
	if len(wrappedTools) != 1 {
		t.Fatalf("expected 1 tool, got %d", len(wrappedTools))
	}
	d := wrappedTools[0].Declaration()
	if d.Description != "read a file" {
		t.Fatalf("description = %q, should not be modified", d.Description)
	}
}

func TestApplyConfirmationPolicy_ToolSetsMixed(t *testing.T) {
	emailTool := &mockTool{decl: &trpctool.Declaration{Name: "email_send", Description: "send email"}}
	readTool := &mockTool{decl: &trpctool.Declaration{Name: "read_file", Description: "read a file"}}
	ts := &tools.AssembledToolsets{
		ToolSets: []tools.ToolSet{
			&mockToolSet{
				nameFn: func() string { return "mixed" },
				toolsFn: func(_ context.Context) []trpctool.Tool {
					return []trpctool.Tool{emailTool, readTool}
				},
			},
		},
	}
	trpc.ApplyConfirmationPolicy(ts, map[string]bool{"email_send": true})

	wrapped := ts.ToolSets[0]
	wrappedTools := wrapped.Tools(context.Background())
	if len(wrappedTools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(wrappedTools))
	}
	emailDecl := wrappedTools[0].Declaration()
	if !strings.Contains(emailDecl.Description, "Requires explicit user approval") {
		t.Fatalf("email_send description = %q, should contain approval text", emailDecl.Description)
	}
	readDecl := wrappedTools[1].Declaration()
	if strings.Contains(readDecl.Description, "Requires explicit user approval") {
		t.Fatalf("read_file description = %q, should not contain approval text", readDecl.Description)
	}
}

func TestApplyConfirmationPolicy_ToolSetsNilInner(t *testing.T) {
	ts := &tools.AssembledToolsets{
		ToolSets: []tools.ToolSet{nil},
	}
	trpc.ApplyConfirmationPolicy(ts, map[string]bool{"shell_exec": true})
}

func TestApplyConfirmationPolicy_ToolSetsEmptyTools(t *testing.T) {
	ts := &tools.AssembledToolsets{
		ToolSets: []tools.ToolSet{
			&mockToolSet{
				nameFn:  func() string { return "empty" },
				toolsFn: func(_ context.Context) []trpctool.Tool { return nil },
			},
		},
	}
	trpc.ApplyConfirmationPolicy(ts, map[string]bool{"shell_exec": true})
	wrapped := ts.ToolSets[0]
	result := wrapped.Tools(context.Background())
	if len(result) != 0 {
		t.Fatalf("expected 0 tools, got %d", len(result))
	}
}

func TestApplyConfirmationPolicy_NilToolsets(t *testing.T) {
	trpc.ApplyConfirmationPolicy(nil, map[string]bool{"shell_exec": true})
}

func TestApplyConfirmationPolicy_EmptyRequires(t *testing.T) {
	ts := &tools.AssembledToolsets{
		Tools: []tools.Tool{&mockTool{decl: &trpctool.Declaration{Name: "shell_exec", Description: "run shell"}}},
	}
	trpc.ApplyConfirmationPolicy(ts, nil)
	d := ts.Tools[0].Declaration()
	if d.Description != "run shell" {
		t.Fatalf("description should not change with empty requires, got %q", d.Description)
	}
}

func TestApplyConfirmationPolicy_ConfirmingToolSetName(t *testing.T) {
	ts := &tools.AssembledToolsets{
		ToolSets: []tools.ToolSet{
			&mockToolSet{
				nameFn: func() string { return "email" },
				toolsFn: func(_ context.Context) []trpctool.Tool {
					return []trpctool.Tool{&mockTool{decl: &trpctool.Declaration{Name: "email_send", Description: "send"}}}
				},
			},
		},
	}
	trpc.ApplyConfirmationPolicy(ts, map[string]bool{"email_send": true})
	if ts.ToolSets[0].Name() != "email" {
		t.Fatalf("Name() = %q, want %q", ts.ToolSets[0].Name(), "email")
	}
}

func TestApplyConfirmationPolicy_ConfirmingToolSetClose(t *testing.T) {
	closed := false
	ts := &tools.AssembledToolsets{
		ToolSets: []tools.ToolSet{
			&mockToolSet{
				closeFn: func() error { closed = true; return nil },
				toolsFn: func(_ context.Context) []trpctool.Tool {
					return []trpctool.Tool{&mockTool{decl: &trpctool.Declaration{Name: "x", Description: "d"}}}
				},
			},
		},
	}
	trpc.ApplyConfirmationPolicy(ts, map[string]bool{"x": true})
	if err := ts.ToolSets[0].Close(); err != nil {
		t.Fatalf("Close() error: %v", err)
	}
	if !closed {
		t.Fatal("expected inner Close() to be called")
	}
}
