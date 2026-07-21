//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package toolcache

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trpc.group/trpc-go/trpc-agent-go/agent"
)

func TestFileView_StoreAndLookup(t *testing.T) {
	inv := agent.NewInvocation()
	view := FileView{
		Content:    "hello\n",
		MtimeMs:    12345,
		Encoding:   "utf8",
		LineEnding: "\n",
		Mode:       0o644,
	}
	StoreFileView(inv, "a.txt", view)

	got, ok := LookupFileView(inv, "a.txt")
	require.True(t, ok)
	assert.Equal(t, view, got)
}

func TestFileView_LookupMiss(t *testing.T) {
	inv := agent.NewInvocation()
	_, ok := LookupFileView(inv, "missing.txt")
	assert.False(t, ok)
}

func TestFileView_Overwrite(t *testing.T) {
	inv := agent.NewInvocation()
	StoreFileView(inv, "a.txt", FileView{Content: "old", MtimeMs: 1})
	StoreFileView(inv, "a.txt", FileView{Content: "new", MtimeMs: 2})

	got, ok := LookupFileView(inv, "a.txt")
	require.True(t, ok)
	assert.Equal(t, "new", got.Content)
	assert.Equal(t, int64(2), got.MtimeMs)
}

func TestFileView_MultiplePaths(t *testing.T) {
	inv := agent.NewInvocation()
	StoreFileView(inv, "a.txt", FileView{Content: "a", MtimeMs: 1})
	StoreFileView(inv, "b.txt", FileView{Content: "b", MtimeMs: 2})

	a, ok := LookupFileView(inv, "a.txt")
	require.True(t, ok)
	assert.Equal(t, "a", a.Content)
	b, ok := LookupFileView(inv, "b.txt")
	require.True(t, ok)
	assert.Equal(t, "b", b.Content)
}

func TestFileView_NilInvocationSafe(t *testing.T) {
	var inv *agent.Invocation
	StoreFileView(inv, "a.txt", FileView{Content: "x"})
	_, ok := LookupFileView(inv, "a.txt")
	assert.False(t, ok)
}

func TestFileView_EmptyPathIgnored(t *testing.T) {
	inv := agent.NewInvocation()
	StoreFileView(inv, "  ", FileView{Content: "x"})
	_, ok := LookupFileView(inv, "  ")
	assert.False(t, ok)
}

func TestFileView_FromContext(t *testing.T) {
	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	StoreFileViewFromContext(ctx, "a.txt", FileView{
		Content: "ctx",
		MtimeMs: 7,
	})
	got, ok := LookupFileViewFromContext(ctx, "a.txt")
	require.True(t, ok)
	assert.Equal(t, "ctx", got.Content)
}

func TestFileView_FromContextNoInvocation(t *testing.T) {
	ctx := context.Background()
	StoreFileViewFromContext(ctx, "a.txt", FileView{Content: "x"})
	_, ok := LookupFileViewFromContext(ctx, "a.txt")
	assert.False(t, ok)
}
