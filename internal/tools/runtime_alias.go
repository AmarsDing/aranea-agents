package tools

import (
	"context"
	"fmt"

	"aranea-agents/internal/tools/alias"

	biztool "aranea-agents/internal/biz/tool"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// RuntimeToolNameAliases maps legacy/UI/catalog names to mounted declaration names.
// Re-exported from alias sub-package for backward compatibility.
var RuntimeToolNameAliases = alias.RuntimeToolNameAliases

type aliasTool struct {
	name string
	inner Tool
}

func (a *aliasTool) Declaration() *Declaration {
	if a == nil || a.inner == nil {
		return nil
	}
	decl := a.inner.Declaration()
	if decl == nil {
		return &Declaration{Name: a.name}
	}
	out := *decl
	out.Name = a.name
	if decl.InputSchema != nil {
		schemaCopy := *decl.InputSchema
		out.InputSchema = &schemaCopy
	}
	return &out
}

// Call invokes the inner tool. Returns explicit error when inner is not callable —
// previously returned (nil, nil) which silently dropped LLM tool calls (TPM-P1-02).
func (a *aliasTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if a == nil || a.inner == nil {
		return nil, fmt.Errorf("tool alias %q: inner tool is nil", aliasNameOrUnknown(a))
	}
	if ct, ok := a.inner.(CallableTool); ok {
		return ct.Call(ctx, jsonArgs)
	}
	return nil, fmt.Errorf("tool alias %q: inner tool is not callable", a.name)
}

// StreamableCall mirrors Call: explicit error replaces silent (nil, nil) (TPM-P1-02).
func (a *aliasTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	if a == nil || a.inner == nil {
		return nil, fmt.Errorf("tool alias %q: inner tool is nil", aliasNameOrUnknown(a))
	}
	if st, ok := a.inner.(StreamableTool); ok {
		return st.StreamableCall(ctx, jsonArgs)
	}
	return nil, fmt.Errorf("tool alias %q: inner tool is not streamable", a.name)
}

func (a *aliasTool) SkipSummarization() bool {
	type skipper interface{ SkipSummarization() bool }
	if s, ok := a.inner.(skipper); ok {
		return s.SkipSummarization()
	}
	return false
}

func (a *aliasTool) LongRunning() bool {
	type longRunner interface{ LongRunning() bool }
	if l, ok := a.inner.(longRunner); ok {
		return l.LongRunning()
	}
	return false
}

func aliasNameOrUnknown(a *aliasTool) string {
	if a == nil || a.name == "" {
		return "<unknown>"
	}
	return a.name
}

// ValidateRuntimeAliasesAgainstPolicy returns an error if any alias key appears
// in both RuntimeToolNameAliases and biz.PolicyAliases() but points at different
// canonical targets. Call this once at wire/init time so drift is caught early
// (TPM-P1-01).
func ValidateRuntimeAliasesAgainstPolicy() error {
	policy := biztool.PolicyAliases()
	for alias, runtimeCanon := range RuntimeToolNameAliases {
		policyCanon, ok := policy[alias]
		if !ok {
			continue
		}
		if policyCanon != runtimeCanon {
			return fmt.Errorf("tool alias %q drift: biz policy → %q vs runtime → %q",
				alias, policyCanon, runtimeCanon)
		}
	}
	return nil
}

// ApplyRuntimeNameAliases registers alias declarations for tools already mounted.
func ApplyRuntimeNameAliases(ctx context.Context, out *AssembledToolsets) {
	if out == nil {
		return
	}
	byName := make(map[string]Tool)
	for _, ts := range out.ToolSets {
		if ts == nil {
			continue
		}
		for _, t := range ts.Tools(ctx) {
			if t == nil || t.Declaration() == nil {
				continue
			}
			byName[t.Declaration().Name] = t
		}
	}
	for _, t := range out.Tools {
		if t == nil || t.Declaration() == nil {
			continue
		}
		byName[t.Declaration().Name] = t
	}
	for alias, canonical := range RuntimeToolNameAliases {
		if alias == canonical {
			continue
		}
		if _, exists := byName[alias]; exists {
			continue
		}
		target, ok := byName[canonical]
		if !ok {
			continue
		}
		out.Tools = append(out.Tools, &aliasTool{name: alias, inner: target})
	}
}
