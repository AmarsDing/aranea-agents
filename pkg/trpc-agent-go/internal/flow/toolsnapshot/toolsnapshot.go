//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package toolsnapshot owns the invocation-scoped LLM tool snapshot keys.
package toolsnapshot

import (
	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

const (
	// ToolsSnapshotKey is the invocation state key used to cache the final tool list.
	ToolsSnapshotKey = "llmflow:tools_snapshot"
	// HasFilteredUserToolsKey caches whether the filtered snapshot has user tools.
	HasFilteredUserToolsKey           = "llmflow:has_filtered_user_tools"
	filteredTraceableUserToolNamesKey = "llmflow:filtered_traceable_user_tool_names"
)

// Get returns the cached tool snapshot for this invocation.
func Get(inv *agent.Invocation) ([]tool.Tool, bool) {
	tools, ok := agent.GetStateValue[[]tool.Tool](inv, ToolsSnapshotKey)
	if !ok {
		return nil, false
	}
	return copyTools(tools), true
}

// Set stores the cached tool snapshot for this invocation.
func Set(
	inv *agent.Invocation,
	tools []tool.Tool,
	hasFilteredUserTools bool,
	filteredTraceableUserToolNames []string,
) {
	if inv == nil {
		return
	}
	inv.SetState(ToolsSnapshotKey, copyTools(tools))
	inv.SetState(HasFilteredUserToolsKey, hasFilteredUserTools)
	inv.SetState(
		filteredTraceableUserToolNamesKey,
		copyStrings(filteredTraceableUserToolNames),
	)
}

// HasFilteredUserTools reports whether the cached snapshot has user tools.
func HasFilteredUserTools(inv *agent.Invocation) (bool, bool) {
	return agent.GetStateValue[bool](inv, HasFilteredUserToolsKey)
}

// FilteredTraceableUserToolNames reports filtered user tool names that have structure surfaces.
func FilteredTraceableUserToolNames(inv *agent.Invocation) ([]string, bool) {
	names, ok := agent.GetStateValue[[]string](inv, filteredTraceableUserToolNamesKey)
	if !ok {
		return nil, false
	}
	return copyStrings(names), true
}

// Invalidate clears the cached tool snapshot for this invocation.
func Invalidate(inv *agent.Invocation) {
	if inv == nil {
		return
	}
	inv.DeleteState(ToolsSnapshotKey)
	inv.DeleteState(HasFilteredUserToolsKey)
	inv.DeleteState(filteredTraceableUserToolNamesKey)
}

// Append adds t to the cached snapshot for this invocation so a tool
// activated mid-turn (tool_load) is visible on the next model request
// without waiting for a full rebuild. Returns false when inv is nil, t is
// nil, or no snapshot is cached yet (caller should Invalidate instead).
func Append(inv *agent.Invocation, t tool.Tool) bool {
	if inv == nil || t == nil {
		return false
	}
	tools, ok := Get(inv)
	if !ok {
		return false
	}
	name := ""
	if decl := t.Declaration(); decl != nil {
		name = decl.Name
	}
	if name != "" {
		for _, existing := range tools {
			if existing == nil {
				continue
			}
			d := existing.Declaration()
			if d != nil && d.Name == name {
				return true
			}
		}
	}
	hasFiltered, _ := HasFilteredUserTools(inv)
	traceable, _ := FilteredTraceableUserToolNames(inv)
	Set(inv, append(tools, t), hasFiltered, traceable)
	return true
}

func copyTools(tools []tool.Tool) []tool.Tool {
	return append([]tool.Tool(nil), tools...)
}

func copyStrings(values []string) []string {
	return append([]string(nil), values...)
}
