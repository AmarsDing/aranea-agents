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
	"sync"

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

// pendingMutations records snapshot mutations (Append/Invalidate) keyed by
// invocation ID.
//
// Why this exists (2026-09-01 root fix): parallel tool execution gives each
// worker an invocation VIEW with a copied state map (see functioncall.go
// newParallelInvocationView). A meta-tool such as tool_load running inside a
// worker appends to / invalidates only the worker's copied snapshot, so the
// flow's canonical invocation keeps serving the stale cached list and the
// next model request reports "tool not found" for freshly activated tools.
// The view shares InvocationID with the canonical invocation, so mutations
// recorded here are consumed and applied by the next Get on any invocation
// carrying the same ID. Entries are consumed (LoadAndDelete) on the next Get;
// a residual entry can only linger when a turn is aborted between the tool
// batch and the next model call, which is rare and bounded in size.
var pendingMutations sync.Map // map[string]*pendingMutation

type pendingMutation struct {
	mu         sync.Mutex
	invalidate bool
	appended   []tool.Tool
}

func recordPending(inv *agent.Invocation, invalidate bool, t tool.Tool) {
	if inv == nil || inv.InvocationID == "" {
		return
	}
	v, _ := pendingMutations.LoadOrStore(inv.InvocationID, &pendingMutation{})
	pm := v.(*pendingMutation)
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if invalidate {
		pm.invalidate = true
		pm.appended = nil
		return
	}
	if t != nil && !pm.invalidate {
		pm.appended = append(pm.appended, t)
	}
}

func consumePending(inv *agent.Invocation) (invalidate bool, appended []tool.Tool) {
	if inv == nil || inv.InvocationID == "" {
		return false, nil
	}
	v, ok := pendingMutations.LoadAndDelete(inv.InvocationID)
	if !ok {
		return false, nil
	}
	pm := v.(*pendingMutation)
	pm.mu.Lock()
	defer pm.mu.Unlock()
	return pm.invalidate, pm.appended
}

// Get returns the cached tool snapshot for this invocation.
func Get(inv *agent.Invocation) ([]tool.Tool, bool) {
	invalidate, appended := consumePending(inv)
	if invalidate {
		// A worker invalidated its view's snapshot copy; the canonical
		// invocation still holds the stale cache — clear it here so the
		// caller rebuilds with the latest filters/activation state.
		clearState(inv)
		return nil, false
	}
	tools, ok := agent.GetStateValue[[]tool.Tool](inv, ToolsSnapshotKey)
	if !ok {
		return nil, false
	}
	if len(appended) > 0 {
		merged := mergeAppendedTools(tools, appended)
		if len(merged) != len(tools) {
			hasFiltered, _ := HasFilteredUserTools(inv)
			traceable, _ := FilteredTraceableUserToolNames(inv)
			Set(inv, merged, hasFiltered, traceable)
			tools = merged
		}
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
	clearState(inv)
	recordPending(inv, true, nil)
}

func clearState(inv *agent.Invocation) {
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
	tools, ok := agent.GetStateValue[[]tool.Tool](inv, ToolsSnapshotKey)
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
				// Already visible on this invocation; still record so a
				// canonical invocation missing it picks it up via Get.
				recordPending(inv, false, t)
				return true
			}
		}
	}
	hasFiltered, _ := HasFilteredUserTools(inv)
	traceable, _ := FilteredTraceableUserToolNames(inv)
	Set(inv, append(copyTools(tools), t), hasFiltered, traceable)
	recordPending(inv, false, t)
	return true
}

// mergeAppendedTools returns tools plus any appended tools whose declaration
// names are not already present. Order is stable: existing tools first,
// newly appended ones after, in append order.
func mergeAppendedTools(tools []tool.Tool, appended []tool.Tool) []tool.Tool {
	names := make(map[string]bool, len(tools))
	for _, existing := range tools {
		if existing == nil {
			continue
		}
		if d := existing.Declaration(); d != nil {
			names[d.Name] = true
		}
	}
	merged := tools
	for _, t := range appended {
		if t == nil {
			continue
		}
		d := t.Declaration()
		if d == nil {
			continue
		}
		if d.Name != "" && names[d.Name] {
			continue
		}
		names[d.Name] = true
		merged = append(merged, t)
	}
	return merged
}

func copyTools(tools []tool.Tool) []tool.Tool {
	return append([]tool.Tool(nil), tools...)
}

func copyStrings(values []string) []string {
	return append([]string(nil), values...)
}
