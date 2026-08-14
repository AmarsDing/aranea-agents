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
	name  string
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

// streamableAliasTool adds StreamableCall to aliasTool. It exists as a
// separate type (rather than defining StreamableCall on *aliasTool) so that
// only aliases whose inner tool is streamable satisfy StreamableTool —
// preventing the framework from misclassifying non-streaming aliases and
// routing them to StreamableCall (2026-07-18 regression fix; mirrors the
// streamableToolDecorator pattern in decorator.go).
type streamableAliasTool struct {
	*aliasTool
}

// StreamableCall delegates to the inner streamable tool. Construction is
// guarded by ApplyRuntimeNameAliases, which only builds this variant when the
// inner tool satisfies StreamableTool.
func (a *streamableAliasTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	if a == nil || a.aliasTool == nil || a.inner == nil {
		return nil, fmt.Errorf("tool alias %q: inner tool is nil", aliasNameOrUnknown(a.aliasTool))
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

// InnerTool returns the wrapped tool, enabling filter penetration — e.g. the
// deferred-tool ToolFilter can check the underlying tool's activation state
// when a deferred tool is wrapped by an alias.
func (a *aliasTool) InnerTool() trpctool.Tool {
	if a == nil {
		return nil
	}
	return a.inner
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
// canonical targets. Assemble calls this on every agent build (not just once at
// wire/init time); the check is a cheap in-memory map diff, so repeated calls
// are fine and drift is caught at the first affected build (TPM-P1-01).
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

// aliasExpandableToolSetNames are the runtime Name() values of the builtin
// ToolSets assembled by Assemble. Every canonical target in
// alias.RuntimeToolNameAliases is a builtin tool declaration name, so
// expanding any other ToolSet (MCP servers, browser, openapi — named after
// user config) can never resolve an alias. Skipping them avoids a
// tools/list network roundtrip per agent build (MCP ToolSet.Tools refreshes
// over the wire when its cache is stale).
var aliasExpandableToolSetNames = map[string]bool{
	"file":           true, // trpc file toolset (default name "file")
	"hostexec":       true, // trpc hostexec toolset (defaultToolSetName)
	"claudecode":     true, // trpc claudecode composite toolset
	"google":         true, // trpc google search toolset (Name() == "google")
	"arxiv_search":   true,
	"wikipedia":      true,
	"email":          true,
	"working_memory": true,
	"deliverable":    true,
	"client":         true, // clientbridge.ToolSetName
	"coding":         true, // codingbridge.ToolSetName
}

// ApplyRuntimeNameAliases registers alias declarations for tools already mounted.
func ApplyRuntimeNameAliases(ctx context.Context, out *AssembledToolsets) {
	if out == nil {
		return
	}
	byName := make(map[string]Tool)
	for _, ts := range out.ToolSets {
		if ts == nil || !aliasExpandableToolSetNames[ts.Name()] {
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
		// Resolve chained aliases (e.g. "shell" → "shell_exec" → "exec_command").
		// Follow the chain until we find a mounted tool or hit a cycle.
		resolved := canonical
		visited := map[string]bool{alias: true}
		for {
			if visited[resolved] {
				break // cycle guard
			}
			visited[resolved] = true
			if target, ok := byName[resolved]; ok {
				base := &aliasTool{name: alias, inner: target}
				var aliasT Tool = base
				if _, streamable := target.(StreamableTool); streamable {
					aliasT = &streamableAliasTool{aliasTool: base}
				}
				out.Tools = append(out.Tools, aliasT)
				byName[alias] = target // allow further aliases to chain off this one
				break
			}
			next, ok := RuntimeToolNameAliases[resolved]
			if !ok {
				break // no further alias chain
			}
			resolved = next
		}
	}
}
