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
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/internal/toolcache"
)

func newEditTestToolSet(t *testing.T) (*fileToolSet, string) {
	t.Helper()
	dir := t.TempDir()
	set, err := NewToolSet(WithBaseDir(dir))
	require.NoError(t, err)
	return set.(*fileToolSet), dir
}

func writeTestFile(t *testing.T, dir string, name string, data []byte) string {
	t.Helper()
	p := filepath.Join(dir, name)
	require.NoError(t, os.WriteFile(p, data, 0o644))
	return p
}

func TestLoadEditSnapshot_ReadsUTF8AndCaches(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("hello\nworld\n"))

	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	snap, err := fts.loadEditSnapshot(ctx, "a.txt", nil)
	require.NoError(t, err)
	assert.Equal(t, "hello\nworld\n", snap.content)
	assert.Equal(t, "utf8", snap.encoding)
	assert.Equal(t, "\n", snap.lineEnding)
	assert.Greater(t, snap.mtimeMs, int64(0))

	view, ok := toolcache.LookupFileView(inv, snap.filePath)
	require.True(t, ok)
	assert.Equal(t, snap.content, view.Content)
	assert.Equal(t, snap.mtimeMs, view.MtimeMs)
}

func TestLoadEditSnapshot_NormalizesCRLF(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("a\r\nb\r\n"))

	snap, err := fts.loadEditSnapshot(context.Background(), "a.txt", nil)
	require.NoError(t, err)
	assert.Equal(t, "a\nb\n", snap.content)
	assert.Equal(t, "\r\n", snap.lineEnding)
}

func TestLoadEditSnapshot_DecodesUTF16LE(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	// BOM + "ab" in UTF-16LE.
	writeTestFile(t, dir, "a.txt", []byte{0xff, 0xfe, 'a', 0x00, 'b', 0x00})

	snap, err := fts.loadEditSnapshot(context.Background(), "a.txt", nil)
	require.NoError(t, err)
	assert.Equal(t, "ab", snap.content)
	assert.Equal(t, "utf16le", snap.encoding)
}

func TestLoadEditSnapshot_ExpectedMtimeMatch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("x\n"))
	st, err := os.Stat(p)
	require.NoError(t, err)
	mtime := st.ModTime().UnixMilli()

	_, err = fts.loadEditSnapshot(context.Background(), "a.txt", &mtime)
	require.NoError(t, err)
}

func TestLoadEditSnapshot_ExpectedMtimeMismatch(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("x\n"))
	stale := int64(1)

	_, err := fts.loadEditSnapshot(context.Background(), "a.txt", &stale)
	require.Error(t, err)
	var modErr *fileModifiedExternallyError
	require.ErrorAs(t, err, &modErr)
}

func TestLoadEditSnapshot_RejectsDirectory(t *testing.T) {
	fts, _ := newEditTestToolSet(t)
	_, err := fts.loadEditSnapshot(context.Background(), "", nil)
	// Empty name resolves to base dir itself, which is a directory.
	require.Error(t, err)
}

func TestLoadEditSnapshot_RejectsBinary(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "bin.dat", []byte{'a', 0x00, 'b'})

	_, err := fts.loadEditSnapshot(context.Background(), "bin.dat", nil)
	require.Error(t, err)
}

func TestLoadEditSnapshot_RejectsTooLarge(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	fts.maxFileSize = 4
	writeTestFile(t, dir, "big.txt", []byte("12345"))

	_, err := fts.loadEditSnapshot(context.Background(), "big.txt", nil)
	require.Error(t, err)
}

func TestCommitEditSnapshot_PreservesCRLF(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("a\r\nb\r\n"))

	snap, err := fts.loadEditSnapshot(context.Background(), "a.txt", nil)
	require.NoError(t, err)

	_, err = fts.commitEditSnapshot(context.Background(), snap, "a\nc\n")
	require.NoError(t, err)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, "a\r\nc\r\n", string(raw))
}

func TestCommitEditSnapshot_UTF16RoundTrip(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte{0xff, 0xfe, 'a', 0x00, 'b', 0x00})

	snap, err := fts.loadEditSnapshot(context.Background(), "a.txt", nil)
	require.NoError(t, err)

	_, err = fts.commitEditSnapshot(context.Background(), snap, "ac")
	require.NoError(t, err)

	raw, err := os.ReadFile(p)
	require.NoError(t, err)
	assert.Equal(t, []byte{0xff, 0xfe, 'a', 0x00, 'c', 0x00}, raw)
}

func TestCommitEditSnapshot_UpdatesCacheAndMtime(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("old\n"))

	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	snap, err := fts.loadEditSnapshot(ctx, "a.txt", nil)
	require.NoError(t, err)

	// Force a distinguishable new mtime.
	time.Sleep(2 * time.Millisecond)
	newMtime, err := fts.commitEditSnapshot(ctx, snap, "new\n")
	require.NoError(t, err)

	view, ok := toolcache.LookupFileView(inv, snap.filePath)
	require.True(t, ok)
	assert.Equal(t, "new\n", view.Content)
	assert.Equal(t, newMtime, view.MtimeMs)
}

func TestCommitEditSnapshot_LeavesNoTempFiles(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	writeTestFile(t, dir, "a.txt", []byte("old\n"))

	snap, err := fts.loadEditSnapshot(context.Background(), "a.txt", nil)
	require.NoError(t, err)
	_, err = fts.commitEditSnapshot(context.Background(), snap, "new\n")
	require.NoError(t, err)

	entries, err := os.ReadDir(dir)
	require.NoError(t, err)
	require.Len(t, entries, 1)
	assert.Equal(t, "a.txt", entries[0].Name())
}

func TestFileViewCache_SkipsSecondRead(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("cached\n"))
	st, err := os.Stat(p)
	require.NoError(t, err)
	origMtime := st.ModTime()

	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	first, err := fts.loadEditSnapshot(ctx, "a.txt", nil)
	require.NoError(t, err)
	assert.Equal(t, "cached\n", first.content)

	// Change content on disk but keep the original mtime: a stale
	// cache hit proves the second load skipped the disk read.
	require.NoError(t, os.WriteFile(p, []byte("changed on disk\n"), 0o644))
	require.NoError(t, os.Chtimes(p, origMtime, origMtime))

	second, err := fts.loadEditSnapshot(ctx, "a.txt", nil)
	require.NoError(t, err)
	assert.Equal(t, "cached\n", second.content)
}

func TestFileViewCache_DiskChangeInvalidatesByMtime(t *testing.T) {
	fts, dir := newEditTestToolSet(t)
	p := writeTestFile(t, dir, "a.txt", []byte("one\n"))

	inv := agent.NewInvocation()
	ctx := agent.NewInvocationContext(context.Background(), inv)

	_, err := fts.loadEditSnapshot(ctx, "a.txt", nil)
	require.NoError(t, err)

	// Real external modification bumps mtime -> cache must not hit.
	time.Sleep(2 * time.Millisecond)
	require.NoError(t, os.WriteFile(p, []byte("two\n"), 0o644))

	snap, err := fts.loadEditSnapshot(ctx, "a.txt", nil)
	require.NoError(t, err)
	assert.Equal(t, "two\n", snap.content)
}
