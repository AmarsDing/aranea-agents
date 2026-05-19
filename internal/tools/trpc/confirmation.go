package trpc

import (
	"context"
	"strings"

	"aranea-agents/internal/tools"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

const confirmationDeclarationSuffix = "\n\n[Requires explicit user approval before execution.]"

// ApplyConfirmationPolicy annotates tool declarations when policy marks a tool as requiring confirmation.
func ApplyConfirmationPolicy(ts *tools.AssembledToolsets, requiresByKey map[string]bool) {
	if ts == nil || len(requiresByKey) == 0 {
		return
	}
	for i, t := range ts.Tools {
		if key := toolKeyFromTool(t); requiresByKey[key] {
			ts.Tools[i] = wrapToolDeclaration(t, true)
		}
	}
	for i, set := range ts.ToolSets {
		if set == nil {
			continue
		}
		ts.ToolSets[i] = &confirmingToolSet{inner: set, requires: requiresByKey}
	}
}

func toolKeyFromTool(t trpctool.Tool) string {
	if t == nil {
		return ""
	}
	if d := t.Declaration(); d != nil {
		return strings.TrimSpace(d.Name)
	}
	return ""
}

type confirmingToolSet struct {
	inner    trpctool.ToolSet
	requires map[string]bool
}

func (s *confirmingToolSet) Name() string {
	if s.inner == nil {
		return ""
	}
	return s.inner.Name()
}

func (s *confirmingToolSet) Close() error {
	if s.inner == nil {
		return nil
	}
	return s.inner.Close()
}

func (s *confirmingToolSet) Tools(ctx context.Context) []trpctool.Tool {
	if s.inner == nil {
		return nil
	}
	raw := s.inner.Tools(ctx)
	if len(raw) == 0 {
		return raw
	}
	out := make([]trpctool.Tool, len(raw))
	for i, t := range raw {
		key := toolKeyFromTool(t)
		out[i] = wrapToolDeclaration(t, s.requires[key])
	}
	return out
}

func wrapToolDeclaration(t trpctool.Tool, requires bool) trpctool.Tool {
	if t == nil || !requires {
		return t
	}
	if ct, ok := t.(trpctool.CallableTool); ok {
		return confirmationCallable{CallableTool: ct}
	}
	return confirmationTool{t: t}
}

type confirmationTool struct {
	t trpctool.Tool
}

func (w confirmationTool) Declaration() *trpctool.Declaration {
	return patchConfirmationDeclaration(w.t.Declaration())
}

type confirmationCallable struct {
	trpctool.CallableTool
}

func (w confirmationCallable) Declaration() *trpctool.Declaration {
	return patchConfirmationDeclaration(w.CallableTool.Declaration())
}

func patchConfirmationDeclaration(d *trpctool.Declaration) *trpctool.Declaration {
	if d == nil {
		return nil
	}
	clone := *d
	desc := strings.TrimSpace(clone.Description)
	if desc == "" {
		clone.Description = strings.TrimSpace(confirmationDeclarationSuffix)
		return &clone
	}
	if !strings.Contains(desc, "Requires explicit user approval") {
		clone.Description = desc + confirmationDeclarationSuffix
	}
	return &clone
}
