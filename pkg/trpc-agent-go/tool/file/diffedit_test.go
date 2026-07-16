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
	"testing"

	"trpc.group/trpc-go/trpc-agent-go/agent"
)

func TestDiffEdit_MultiAtomic(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.go")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts, err := NewToolSet(WithBaseDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	ft := ts.(*fileToolSet)
	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	rsp, err := ft.diffEdit(ctx, &diffEditRequest{
		FileName: "a.go",
		Edits: []diffEditItem{
			{Search: "one", Replace: "ONE"},
			{Search: "three", Replace: "THREE"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if rsp.Error != "" {
		t.Fatalf("error field: %s", rsp.Error)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	want := "ONE\ntwo\nTHREE\n"
	if string(got) != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestDiffEdit_NotUnique(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.txt")
	if err := os.WriteFile(path, []byte("x\nx\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts, err := NewToolSet(WithBaseDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	ft := ts.(*fileToolSet)
	rsp, err := ft.diffEdit(context.Background(), &diffEditRequest{
		FileName: "a.txt",
		Edits:    []diffEditItem{{Search: "x", Replace: "y"}},
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if rsp.Error != "edit_not_unique" {
		t.Fatalf("got error=%q", rsp.Error)
	}
	raw, _ := os.ReadFile(path)
	if string(raw) != "x\nx\n" {
		t.Fatalf("file should be unchanged, got %q", raw)
	}
}

func TestFileViewCache_SkipsSecondRead(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "b.txt")
	if err := os.WriteFile(path, []byte("hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts, err := NewToolSet(WithBaseDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	ft := ts.(*fileToolSet)
	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	_, err = ft.readFile(ctx, &readFileRequest{FileName: "b.txt"})
	if err != nil {
		t.Fatal(err)
	}
	snap1, err := ft.loadEditSnapshot(ctx, "b.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !snap1.FromCache {
		t.Fatal("expected cache hit after read_file")
	}
	_, err = ft.diffEdit(ctx, &diffEditRequest{
		FileName: "b.txt",
		Edits:    []diffEditItem{{Search: "hello", Replace: "world"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	snap2, err := ft.loadEditSnapshot(ctx, "b.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !snap2.FromCache {
		t.Fatal("expected cache hit after diff_edit")
	}
	if snap2.Content != "world\n" {
		t.Fatalf("content=%q", snap2.Content)
	}
}

func TestPatchFile_Unified(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "c.txt")
	if err := os.WriteFile(path, []byte("a\nb\nc\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ts, err := NewToolSet(WithBaseDir(dir))
	if err != nil {
		t.Fatal(err)
	}
	ft := ts.(*fileToolSet)
	patchText := "@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"
	_, err = ft.patchFile(context.Background(), &patchFileRequest{
		FileName: "c.txt",
		Patch:    patchText,
	})
	if err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(path)
	if string(got) != "a\nB\nc\n" {
		t.Fatalf("got %q", got)
	}
}
