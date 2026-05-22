//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package file

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"trpc.group/trpc-go/trpc-agent-go/agent"
)

func TestDiffEdit_MultiEditAtomic(t *testing.T) {
	dir := t.TempDir()
	set, err := NewToolSet(WithBaseDir(dir))
	require.NoError(t, err)
	fts := set.(*fileToolSet)
	path := filepath.Join(dir, "a.txt")
	require.NoError(t, os.WriteFile(path, []byte("alpha\nbeta\ngamma\n"), 0o644))

	ctx := agent.NewInvocationContext(context.Background(), agent.NewInvocation())
	diffTool := findCallableTool(t, fts, "diff_edit")
	args, err := json.Marshal(map[string]any{
		"file_name": "a.txt",
		"edits": []map[string]string{
			{"search": "beta", "replace": "BETA"},
			{"search": "gamma", "replace": "GAMMA"},
		},
	})
	require.NoError(t, err)
	_, err = diffTool.Call(ctx, args)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Equal(t, "alpha\nBETA\nGAMMA\n", string(data))
}

func TestReadFile_ReturnsMtimeMs(t *testing.T) {
	dir := t.TempDir()
	set, err := NewToolSet(WithBaseDir(dir))
	require.NoError(t, err)
	fts := set.(*fileToolSet)
	path := filepath.Join(dir, "mtime.txt")
	require.NoError(t, os.WriteFile(path, []byte("x\n"), 0o644))
	st, err := os.Stat(path)
	require.NoError(t, err)
	wantMtime := st.ModTime().UnixMilli()

	ctx := agent.NewInvocationContext(context.Background(), agent.NewInvocation())
	readTool := findCallableTool(t, fts, "read_file")
	args, err := json.Marshal(map[string]any{"file_name": "mtime.txt"})
	require.NoError(t, err)
	raw, err := readTool.Call(ctx, args)
	require.NoError(t, err)
	rsp, ok := raw.(*readFileResponse)
	require.True(t, ok)
	require.NotNil(t, rsp.MtimeMs)
	require.Equal(t, wantMtime, *rsp.MtimeMs)
}

func TestPatchFile_Unified(t *testing.T) {
	dir := t.TempDir()
	set, err := NewToolSet(WithBaseDir(dir))
	require.NoError(t, err)
	fts := set.(*fileToolSet)
	path := filepath.Join(dir, "b.txt")
	require.NoError(t, os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644))

	ctx := agent.NewInvocationContext(context.Background(), agent.NewInvocation())
	patchTool := findCallableTool(t, fts, "patch_file")
	args, err := json.Marshal(map[string]any{
		"file_name": "b.txt",
		"patch":     "@@ -2,1 +2,1 @@\n-two\n+TWO",
	})
	require.NoError(t, err)
	_, err = patchTool.Call(ctx, args)
	require.NoError(t, err)
	data, err := os.ReadFile(path)
	require.NoError(t, err)
	require.Contains(t, string(data), "TWO")
}

func TestFileViewCache_SkipsSecondRead(t *testing.T) {
	dir := t.TempDir()
	set, err := NewToolSet(WithBaseDir(dir))
	require.NoError(t, err)
	fts := set.(*fileToolSet)
	path := filepath.Join(dir, "c.txt")
	require.NoError(t, os.WriteFile(path, []byte("hello\n"), 0o644))

	ctx := agent.NewInvocationContext(context.Background(), agent.NewInvocation())
	readTool := findCallableTool(t, fts, "read_file")
	readArgs, err := json.Marshal(map[string]any{"file_name": "c.txt"})
	require.NoError(t, err)
	_, err = readTool.Call(ctx, readArgs)
	require.NoError(t, err)

	os.Remove(path)
	diffTool := findCallableTool(t, fts, "diff_edit")
	editArgs, err := json.Marshal(map[string]any{
		"file_name": "c.txt",
		"edits":     []map[string]string{{"search": "hello", "replace": "world"}},
	})
	require.NoError(t, err)
	_, err = diffTool.Call(ctx, editArgs)
	require.NoError(t, err)
}

type callableTool interface {
	Call(context.Context, []byte) (any, error)
}

func findCallableTool(t *testing.T, fts *fileToolSet, name string) callableTool {
	t.Helper()
	for _, tl := range fts.Tools(context.Background()) {
		if tl.Declaration().Name == name {
			ct, ok := tl.(callableTool)
			require.True(t, ok)
			return ct
		}
	}
	t.Fatalf("tool %q not found", name)
	return nil
}
