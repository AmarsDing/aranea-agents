package trpc

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/tools"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubCallableTool struct {
	decl *trpctool.Declaration
}

func (s stubCallableTool) Declaration() *trpctool.Declaration {
	return s.decl
}

func (s stubCallableTool) Call(_ context.Context, _ []byte) (any, error) {
	return nil, nil
}

func TestWrapToolDeclaration_CallableTool(t *testing.T) {
	original := stubCallableTool{
		decl: &trpctool.Declaration{Name: "email_send", Description: "send email"},
	}
	result := wrapToolDeclaration(original, true)
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	d := result.Declaration()
	if d == nil {
		t.Fatal("expected non-nil declaration")
	}
	if !strings.Contains(d.Description, "Requires explicit user approval") {
		t.Fatalf("description = %q, should contain approval text", d.Description)
	}
}

func TestWrapToolDeclaration_CallableToolNotRequired(t *testing.T) {
	original := stubCallableTool{
		decl: &trpctool.Declaration{Name: "read_file", Description: "read a file"},
	}
	result := wrapToolDeclaration(original, false)
	if result != original {
		t.Fatal("non-required CallableTool should be returned as-is")
	}
}

func TestApplyConfirmationPolicy_CallableToolPatchesDeclaration(t *testing.T) {
	tool := stubCallableTool{
		decl: &trpctool.Declaration{Name: "email_send", Description: "send email"},
	}
	ts := &tools.AssembledToolsets{Tools: []tools.Tool{tool}}
	ApplyConfirmationPolicy(ts, map[string]bool{"email_send": true})
	d := ts.Tools[0].Declaration()
	if d == nil {
		t.Fatal("expected non-nil declaration")
	}
	if !strings.Contains(d.Description, "Requires explicit user approval") {
		t.Fatalf("CallableTool description should be patched, got %q", d.Description)
	}
}

func TestApplyConfirmationPolicy_MixedCallableAndNonCallable(t *testing.T) {
	callable := stubCallableTool{
		decl: &trpctool.Declaration{Name: "email_send", Description: "send email"},
	}
	nonCallable := stubTool{name: "read_file", desc: "read a file"}
	ts := &tools.AssembledToolsets{Tools: []tools.Tool{callable, nonCallable}}
	ApplyConfirmationPolicy(ts, map[string]bool{"email_send": true, "read_file": true})
	d1 := ts.Tools[0].Declaration()
	if !strings.Contains(d1.Description, "Requires explicit user approval") {
		t.Fatalf("CallableTool description = %q, should contain approval text", d1.Description)
	}
	d2 := ts.Tools[1].Declaration()
	if !strings.Contains(d2.Description, "Requires explicit user approval") {
		t.Fatalf("non-CallableTool description = %q, should contain approval text", d2.Description)
	}
}
