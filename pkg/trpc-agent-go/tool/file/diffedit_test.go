//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package file

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/internal/toolcache"
)

func callDiffEdit(
	t *testing.T,
	ctx context.Context,
	fts *fileToolSet,
	req *diffEditRequest,
) *diffEditResponse {
	t.Helper()
	rsp, err := fts.diffEdit(ctx, req)
	require.NoError(t, err)
	return rsp
}

func TestDiffEdit_SingleEdit(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("hello world\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "world", Replace: "claude"},
		},
	})
	assert.Equal(t, 1, rsp.AppliedEdits)
	assert.Empty(t, rsp.Error)
	assert.Greater(t, rsp.MtimeMs, int64(0))

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "hello claude\n", string(raw))
}

func TestDiffEdit_MultiEditAppliedInOrder(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("one two three\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "one", Replace: "1"},
			{Search: "three", Replace: "3"},
		},
	})
	assert.Equal(t, 2, rsp.AppliedEdits)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "1 two 3\n", string(raw))
}

func TestDiffEdit_FailingEditLeavesFileUntouched(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("alpha beta\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "alpha", Replace: "ALPHA"},
			{Search: "missing", Replace: "x"},
		},
	})
	assert.Equal(t, "edit_not_found", rsp.Error)
	require.NotNil(t, rsp.EditIndex)
	assert.Equal(t, 1, *rsp.EditIndex)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "alpha beta\n", string(raw))
}

func TestDiffEdit_NotUniqueReportsMatchLines(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("foo\nbar\nfoo\nbaz\nfoo\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "foo", Replace: "qux"},
		},
	})
	assert.Equal(t, "edit_not_unique", rsp.Error)
	assert.Equal(t, 3, rsp.MatchCount)
	assert.Equal(t, []int{1, 3, 5}, rsp.MatchLines)
	assert.NotEmpty(t, rsp.Hint)
}

func TestDiffEdit_ReplaceAll(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("foo bar foo\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "foo", Replace: "qux", ReplaceAll: true},
		},
	})
	assert.Equal(t, 1, rsp.AppliedEdits)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "qux bar qux\n", string(raw))
}

func TestDiffEdit_SearchEqualsReplaceIsNoop(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("same\n"))
	st, err := os.Stat(p)
	require.NoError(t, err)

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "same", Replace: "same"},
		},
	})
	assert.Empty(t, rsp.Error)
	assert.Contains(t, rsp.Message, "no changes")

	after, err := os.Stat(p)
	require.NoError(t, err)
	assert.Equal(t, st.ModTime(), after.ModTime())
}

func TestDiffEdit_QuoteFuzzyMatchPreservesStyle(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("say “hello” now\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: `say "hello" now`, Replace: `say "bye" now`},
		},
	})
	require.Empty(t, rsp.Error)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "say “bye” now\n", string(raw))
}

func TestDiffEdit_ExpectedMtimeMismatch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))
	stale := int64(42)

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName:        "a.txt",
		ExpectedMtimeMs: &stale,
		Edits: []diffEditItem{
			{Search: "x", Replace: "y"},
		},
	})
	assert.Equal(t, "file_modified_externally", rsp.Error)
	assert.Contains(t, rsp.Hint, "read_file")
}

func TestDiffEdit_PreservesCRLF(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("a\r\nb\r\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "b", Replace: "c"},
		},
	})
	require.Empty(t, rsp.Error)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "a\r\nc\r\n", string(raw))
}

func TestDiffEdit_UpdatesFileViewCache(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("old\n"))

	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	rsp := callDiffEdit(t, ctx, fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "old", Replace: "new"},
		},
	})
	require.Empty(t, rsp.Error)

	view, ok := toolcache.LookupFileView(inv, filepath.Join(dir, "a.txt"))
	require.True(t, ok)
	assert.Equal(t, "new\n", view.Content)
	assert.Equal(t, rsp.MtimeMs, view.MtimeMs)
}

func TestDiffEdit_RejectsTooManyEdits(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))

	edits := make([]diffEditItem, 0, maxEditsPerCall+1)
	for i := 0; i < maxEditsPerCall+1; i++ {
		edits = append(edits, diffEditItem{Search: "x", Replace: "y"})
	}
	_, err := fts.diffEdit(context.Background(), &diffEditRequest{
		FileName: "a.txt",
		Edits:    edits,
	})
	require.Error(t, err)
}

func TestDiffEdit_RejectsOversizedSearch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))

	_, err := fts.diffEdit(context.Background(), &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: strings.Repeat("s", maxEditSearchBytes+1), Replace: "y"},
		},
	})
	require.Error(t, err)
}

func TestDiffEdit_RejectsEmptySearch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))

	_, err := fts.diffEdit(context.Background(), &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: "", Replace: "y"},
		},
	})
	require.Error(t, err)
}

func TestDiffEdit_RejectsRef(t *testing.T) {
	fts, _ := newEditTestToolSet(t)
	_, err := fts.diffEdit(context.Background(), &diffEditRequest{
		FileName: "workspace://a.txt",
		Edits: []diffEditItem{
			{Search: "x", Replace: "y"},
		},
	})
	require.Error(t, err)
}

func TestDiffEdit_FileNotFound(t *testing.T) {
	fts, _ := newEditTestToolSet(t)
	_, err := fts.diffEdit(context.Background(), &diffEditRequest{
		FileName: "missing.txt",
		Edits: []diffEditItem{
			{Search: "x", Replace: "y"},
		},
	})
	require.Error(t, err)
}

func TestDiffEdit_DeletionWithEmptyReplace(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("keep DROP keep\n"))

	rsp := callDiffEdit(t, context.Background(), fts, &diffEditRequest{
		FileName: "a.txt",
		Edits: []diffEditItem{
			{Search: " DROP", Replace: ""},
		},
	})
	require.Empty(t, rsp.Error)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "keep keep\n", string(raw))
}
