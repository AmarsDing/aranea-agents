package tools

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// RuntimeToolNameAliases maps legacy/UI/catalog names to mounted declaration names.
// Policy resolution uses biz.toolPolicyKeyAliases; this map applies at runtime so
// LLM calls using common aliases still resolve.
var RuntimeToolNameAliases = map[string]string{
	"write_file":       "save_file",
	"edit_file":        "replace_content",
	"list_files":       "list_file",
	"workspace_search": "search_content",
	"shell":            "exec_command",
	"shell_exec":       "exec_command",
	"todo":             "todo_write",
	"gemini_fetch":     "gemini_web_fetch",
	"wikipedia":        "wikipedia_search",
	"email":            "send_email",
	"await_reply":      "await_user_reply",
	"web_search":       "duckduckgo_search",
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

func (a *aliasTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	if ct, ok := a.inner.(CallableTool); ok {
		return ct.Call(ctx, jsonArgs)
	}
	return nil, nil
}

func (a *aliasTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	if st, ok := a.inner.(StreamableTool); ok {
		return st.StreamableCall(ctx, jsonArgs)
	}
	return nil, nil
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
