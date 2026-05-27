package tools

import (
	"context"
	"fmt"

	biztool "aranea-agents/internal/biz/tool"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// RuntimeToolNameAliases maps legacy/UI/catalog names to mounted declaration names.
// Policy resolution uses biz.toolPolicyKeyAliases; this map applies at runtime so
// LLM calls using common aliases still resolve.
//
// IMPORTANT: keep aliases consistent with biz/tool/tool_policy_keys.go. When the same
// alias appears in both maps, both must point to the SAME canonical catalog tool_key
// (TPM-P1-01). `web_search` previously pointed at `duckduckgo_search` here but at
// `web_research` in biz, producing split-brain routing — now aligned to `web_research`.
var RuntimeToolNameAliases = map[string]string{
	"write_file":       "save_file",
	"edit_file":        "diff_edit",
	"list_files":       "list_file",
	"workspace_search": "search_content",
	"shell":            "shell_exec", // aligned with biz policy alias (shell_exec → exec_command handled separately)
	"shell_exec":       "exec_command",
	"todo":             "todo_write",
	"gemini_fetch":     "gemini_web_fetch",
	"wikipedia":        "wikipedia_search",
	"email":            "send_email",
	"await_reply":      "await_user_reply",
	"web_search":       "web_research", // TPM-P1-01: aligned with biz policy alias
}

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
