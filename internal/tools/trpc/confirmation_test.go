package trpc

import (
	"strings"
	"testing"

	"aranea-agents/internal/tools"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

type stubTool struct {
	name string
	desc string
}

func (s stubTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: s.name, Description: s.desc}
}

func TestApplyConfirmationPolicy_PatchesDeclaration(t *testing.T) {
	base := stubTool{name: "shell_exec", desc: "run shell"}
	ts := &tools.AssembledToolsets{Tools: []tools.Tool{base}}
	ApplyConfirmationPolicy(ts, map[string]bool{"shell_exec": true})
	d := ts.Tools[0].Declaration()
	if d == nil || d.Description == "" {
		t.Fatal("expected patched description")
	}
	if !strings.Contains(d.Description, "Requires explicit user approval") {
		t.Fatalf("description = %q", d.Description)
	}
}
