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
	"trpc.group/trpc-go/trpc-agent-go/tool/file/patch"
)

func callPatchFile(
	t *testing.T,
	ctx context.Context,
	fts *fileToolSet,
	req *patchFileRequest,
) *patchFileResponse {
	t.Helper()
	rsp, err := fts.patchFile(ctx, req)
	require.NoError(t, err)
	return rsp
}

const sampleUnifiedPatch = `--- a/a.txt
+++ b/a.txt
@@ -1,3 +1,3 @@
 one
-two
+TWO
 three
`

func TestPatchFile_UnifiedPatch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("one\ntwo\nthree\n"))

	rsp := callPatchFile(t, context.Background(), fts, &patchFileRequest{
		FileName: "a.txt",
		Patch:    sampleUnifiedPatch,
	})
	assert.Empty(t, rsp.Error)
	assert.Equal(t, 1, rsp.AppliedHunks)
	assert.Greater(t, rsp.MtimeMs, int64(0))

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "one\nTWO\nthree\n", string(raw))
}

func TestPatchFile_StructuredHunks(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("alpha\nbeta\n"))

	rsp := callPatchFile(t, context.Background(), fts, &patchFileRequest{
		FileName: "a.txt",
		Hunks: []patch.Hunk{
			{
				OldStart: 1,
				OldLines: 2,
				NewStart: 1,
				NewLines: 2,
				Lines:    []string{" alpha", "-beta", "+BETA"},
			},
		},
	})
	assert.Empty(t, rsp.Error)
	assert.Equal(t, 1, rsp.AppliedHunks)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "alpha\nBETA\n", string(raw))
}

func TestPatchFile_RejectsBothPatchAndHunks(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))

	_, err := fts.patchFile(context.Background(), &patchFileRequest{
		FileName: "a.txt",
		Patch:    sampleUnifiedPatch,
		Hunks: []patch.Hunk{
			{OldStart: 1, OldLines: 1, NewStart: 1, NewLines: 1, Lines: []string{"-x", "+y"}},
		},
	})
	require.ErrorContains(t, err, "mutually exclusive")
}

func TestPatchFile_RejectsNeitherPatchNorHunks(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))

	_, err := fts.patchFile(context.Background(), &patchFileRequest{
		FileName: "a.txt",
	})
	require.Error(t, err)
}

func TestPatchFile_HunkMismatchStructuredError(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("one\ntwo\nthree\n"))

	rsp := callPatchFile(t, context.Background(), fts, &patchFileRequest{
		FileName: "a.txt",
		Hunks: []patch.Hunk{
			{
				OldStart: 1,
				OldLines: 1,
				NewStart: 1,
				NewLines: 1,
				Lines:    []string{"-NOTPRESENT", "+x"},
			},
		},
	})
	assert.Equal(t, "hunk_mismatch", rsp.Error)
	require.NotNil(t, rsp.HunkIndex)
	assert.Equal(t, 0, *rsp.HunkIndex)
	assert.Equal(t, []string{"-NOTPRESENT"}, rsp.ExpectedLines)
	assert.NotEmpty(t, rsp.ActualLines)
	assert.NotEmpty(t, rsp.Hint)

	// Zero side effects.
	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "one\ntwo\nthree\n", string(raw))
}

func TestPatchFile_RejectsMalformedUnifiedPatch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))

	_, err := fts.patchFile(context.Background(), &patchFileRequest{
		FileName: "a.txt",
		Patch:    "@@ this is not a hunk @@",
	})
	require.Error(t, err)
}

func TestPatchFile_RejectsOversizedPatch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))

	_, err := fts.patchFile(context.Background(), &patchFileRequest{
		FileName: "a.txt",
		Patch:    strings.Repeat("+", maxPatchBytes+1),
	})
	require.Error(t, err)
}

func TestPatchFile_ExpectedMtimeMismatch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))
	stale := int64(7)

	rsp := callPatchFile(t, context.Background(), fts, &patchFileRequest{
		FileName:        "a.txt",
		Patch:           sampleUnifiedPatch,
		ExpectedMtimeMs: &stale,
	})
	assert.Equal(t, "file_modified_externally", rsp.Error)
	assert.Contains(t, rsp.Hint, "read_file")
}

func TestPatchFile_DriftTolerance(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("header\nheader2\ntarget\nfooter\n"))

	// Declared position is wrong (1) but the old sequence matches
	// uniquely at line 3.
	rsp := callPatchFile(t, context.Background(), fts, &patchFileRequest{
		FileName: "a.txt",
		Hunks: []patch.Hunk{
			{
				OldStart: 1,
				OldLines: 1,
				NewStart: 1,
				NewLines: 1,
				Lines:    []string{"-target", "+TARGET"},
			},
		},
	})
	require.Empty(t, rsp.Error)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "header\nheader2\nTARGET\nfooter\n", string(raw))
}

func TestPatchFile_PreservesCRLF(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("one\r\ntwo\r\n"))

	rsp := callPatchFile(t, context.Background(), fts, &patchFileRequest{
		FileName: "a.txt",
		Hunks: []patch.Hunk{
			{
				OldStart: 2,
				OldLines: 1,
				NewStart: 2,
				NewLines: 1,
				Lines:    []string{"-two", "+TWO"},
			},
		},
	})
	require.Empty(t, rsp.Error)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "one\r\nTWO\r\n", string(raw))
}

func TestPatchFile_UpdatesFileViewCache(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("old\n"))

	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	rsp := callPatchFile(t, ctx, fts, &patchFileRequest{
		FileName: "a.txt",
		Hunks: []patch.Hunk{
			{
				OldStart: 1,
				OldLines: 1,
				NewStart: 1,
				NewLines: 1,
				Lines:    []string{"-old", "+new"},
			},
		},
	})
	require.Empty(t, rsp.Error)

	view, ok := toolcache.LookupFileView(inv, filepath.Join(dir, "a.txt"))
	require.True(t, ok)
	assert.Equal(t, "new\n", view.Content)
	assert.Equal(t, rsp.MtimeMs, view.MtimeMs)
}

func TestPatchFile_RejectsRef(t *testing.T) {
	fts, _ := newEditTestToolSet(t)
	_, err := fts.patchFile(context.Background(), &patchFileRequest{
		FileName: "workspace://a.txt",
		Patch:    sampleUnifiedPatch,
	})
	require.Error(t, err)
}

func TestPatchFile_FileNotFound(t *testing.T) {
	fts, _ := newEditTestToolSet(t)
	_, err := fts.patchFile(context.Background(), &patchFileRequest{
		FileName: "missing.txt",
		Patch:    sampleUnifiedPatch,
	})
	require.Error(t, err)
}
