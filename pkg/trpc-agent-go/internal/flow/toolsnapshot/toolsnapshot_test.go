//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package toolsnapshot

import (
	"testing"

	"github.com/stretchr/testify/require"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

type testTool struct {
	name string
}

func (t testTool) Declaration() *tool.Declaration {
	return &tool.Declaration{Name: t.name}
}

func TestSetGetCopiesToolSlice(t *testing.T) {
	inv := &agent.Invocation{}
	first := testTool{name: "first"}
	second := testTool{name: "second"}
	tools := []tool.Tool{first}
	traceableUserToolNames := []string{"first"}
	Set(inv, tools, true, traceableUserToolNames)
	hasFiltered, ok := HasFilteredUserTools(inv)
	require.True(t, ok)
	require.True(t, hasFiltered)
	traceableNames, ok := FilteredTraceableUserToolNames(inv)
	require.True(t, ok)
	require.Equal(t, []string{"first"}, traceableNames)
	traceableUserToolNames[0] = "changed"
	traceableAgain, ok := FilteredTraceableUserToolNames(inv)
	require.True(t, ok)
	require.Equal(t, []string{"first"}, traceableAgain)
	traceableNames[0] = "returned"
	traceableAfterReturnedMutation, ok := FilteredTraceableUserToolNames(inv)
	require.True(t, ok)
	require.Equal(t, []string{"first"}, traceableAfterReturnedMutation)
	tools[0] = second
	cached, ok := Get(inv)
	require.True(t, ok)
	require.Equal(t, "first", cached[0].Declaration().Name)
	cached[0] = second
	cachedAgain, ok := Get(inv)
	require.True(t, ok)
	require.Equal(t, "first", cachedAgain[0].Declaration().Name)
}

func TestAppendAddsToolToExistingSnapshot(t *testing.T) {
	inv := &agent.Invocation{}
	first := testTool{name: "first"}
	second := testTool{name: "second"}
	Set(inv, []tool.Tool{first}, true, []string{"first"})
	if ok := Append(inv, second); !ok {
		t.Fatal("Append must succeed when a snapshot is cached")
	}
	cached, ok := Get(inv)
	require.True(t, ok)
	require.Equal(t, 2, len(cached))
	require.Equal(t, "second", cached[1].Declaration().Name)
	hasFiltered, ok := HasFilteredUserTools(inv)
	require.True(t, ok)
	require.True(t, hasFiltered)
	if ok := Append(inv, second); !ok {
		t.Fatal("Append of an already-present name is still success")
	}
	cachedAgain, _ := Get(inv)
	require.Equal(t, 2, len(cachedAgain))
}

func TestAppendWithoutSnapshotReturnsFalse(t *testing.T) {
	if Append(nil, testTool{name: "x"}) {
		t.Fatal("nil invocation must return false")
	}
	inv := &agent.Invocation{}
	if Append(inv, testTool{name: "x"}) {
		t.Fatal("missing snapshot must return false so caller can Invalidate")
	}
}

func TestSnapshotMissingAndInvalidate(t *testing.T) {
	_, ok := Get(nil)
	require.False(t, ok)
	Set(nil, []tool.Tool{testTool{name: "ignored"}}, true, []string{"ignored"})
	Invalidate(nil)
	inv := &agent.Invocation{}
	_, ok = Get(inv)
	require.False(t, ok)
	Set(inv, []tool.Tool{testTool{name: "first"}}, true, []string{"first"})
	Invalidate(inv)
	_, ok = Get(inv)
	require.False(t, ok)
	_, ok = HasFilteredUserTools(inv)
	require.False(t, ok)
	_, ok = FilteredTraceableUserToolNames(inv)
	require.False(t, ok)
}

// TestAppendOnViewVisibleToCanonicalInvocation reproduces the parallel-worker
// snapshot loss: tool_load running inside a parallel worker operates on an
// invocation VIEW (copied state map, shared InvocationID). Its Append must be
// visible to the canonical invocation's next Get, otherwise the next model
// request does not carry the freshly activated tool and the model's call
// fails with "tool not found".
//
// Note: pending mutations are consumed by the first Get carrying the same
// invocation ID. In the real flow only the canonical invocation calls Get
// (workers never resolve tools), so the canonical invocation always consumes
// its workers' mutations; this test mirrors that ordering.
func TestAppendOnViewVisibleToCanonicalInvocation(t *testing.T) {
	parent := &agent.Invocation{InvocationID: "inv-view-append"}
	base := testTool{name: "base"}
	Set(parent, []tool.Tool{base}, true, []string{"base"})

	view := parent.View()
	loaded := testTool{name: "twin_device_search"}
	require.True(t, Append(view, loaded),
		"Append on worker view must succeed (snapshot copied into view)")

	// The canonical invocation must merge the pending append on its next Get.
	parentTools, ok := Get(parent)
	require.True(t, ok)
	require.Len(t, parentTools, 2)
	require.Equal(t, "twin_device_search",
		parentTools[1].Declaration().Name)

	// The merge is persisted back to state: subsequent Gets are idempotent
	// and the pending entry has been consumed.
	parentAgain, ok := Get(parent)
	require.True(t, ok)
	require.Len(t, parentAgain, 2)

	// The worker view itself already saw the appended tool via its own state.
	viewTools, ok := Get(view)
	require.True(t, ok)
	require.Len(t, viewTools, 2)
}

// TestInvalidateOnViewForcesCanonicalRebuild ensures an Invalidate issued on a
// worker view clears the canonical invocation's cached snapshot as well.
func TestInvalidateOnViewForcesCanonicalRebuild(t *testing.T) {
	parent := &agent.Invocation{InvocationID: "inv-view-invalidate"}
	Set(parent, []tool.Tool{testTool{name: "base"}}, true, []string{"base"})

	view := parent.View()
	Invalidate(view)

	_, ok := Get(parent)
	require.False(t, ok,
		"pending invalidate from worker view must force rebuild on canonical invocation")
	_, ok = Get(parent)
	require.False(t, ok, "state cleared; still no snapshot")
}

// TestPendingMutationIsolatedByInvocationID ensures pending mutations do not
// leak across different invocation IDs.
func TestPendingMutationIsolatedByInvocationID(t *testing.T) {
	invA := &agent.Invocation{InvocationID: "inv-iso-a"}
	invB := &agent.Invocation{InvocationID: "inv-iso-b"}
	Set(invA, []tool.Tool{testTool{name: "baseA"}}, true, nil)
	Set(invB, []tool.Tool{testTool{name: "baseB"}}, true, nil)

	viewA := invA.View()
	require.True(t, Append(viewA, testTool{name: "only_a"}))

	toolsB, ok := Get(invB)
	require.True(t, ok)
	require.Len(t, toolsB, 1, "invB must not see invA's pending append")

	toolsA, ok := Get(invA)
	require.True(t, ok)
	require.Len(t, toolsA, 2)
}
