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
	"fmt"
	"os"
	"path/filepath"

	"trpc.group/trpc-go/trpc-agent-go/internal/toolcache"
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// editSnapshot is an in-memory snapshot of a file loaded for editing.
// Content is always LF-normalized text; encoding and lineEnding record
// how to serialize it back on commit.
type editSnapshot struct {
	filePath   string
	content    string
	encoding   string
	lineEnding string
	mode       os.FileMode
	mtimeMs    int64
}

// fileModifiedExternallyError reports an optimistic-lock failure: the
// file mtime on disk differs from the caller's expected mtime.
type fileModifiedExternallyError struct {
	expected int64
	actual   int64
}

// Error implements the error interface.
func (e *fileModifiedExternallyError) Error() string {
	return fmt.Sprintf(
		"file modified externally: expected mtime_ms %d, actual %d",
		e.expected,
		e.actual,
	)
}

// loadEditSnapshot loads a file for editing. When the per-invocation
// FileView cache holds a view whose mtime matches the current file
// mtime, the cached content is used and the disk read is skipped.
// Otherwise the file is read, decoded, and cached.
//
// expectedMtimeMs, when non-nil, is an optimistic lock: a mismatch
// with the on-disk mtime fails with fileModifiedExternallyError.
func (f *fileToolSet) loadEditSnapshot(
	ctx context.Context,
	fileName string,
	expectedMtimeMs *int64,
) (*editSnapshot, error) {
	filePath, err := f.resolvePath(fileName)
	if err != nil {
		return nil, err
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		return nil, fmt.Errorf(
			"accessing file '%s' under base directory '%s': %w",
			fileName,
			f.baseDir,
			err,
		)
	}
	if stat.IsDir() {
		return nil, fmt.Errorf("target path '%s' is a directory", fileName)
	}
	if stat.Size() > f.maxFileSize {
		return nil, fmt.Errorf(
			"file is too large: %d > %d",
			stat.Size(),
			f.maxFileSize,
		)
	}
	mtimeMs := stat.ModTime().UnixMilli()
	if expectedMtimeMs != nil && *expectedMtimeMs != mtimeMs {
		return nil, &fileModifiedExternallyError{
			expected: *expectedMtimeMs,
			actual:   mtimeMs,
		}
	}

	if view, ok := toolcache.LookupFileViewFromContext(ctx, filePath); ok &&
		view.MtimeMs == mtimeMs {
		return &editSnapshot{
			filePath:   filePath,
			content:    view.Content,
			encoding:   view.Encoding,
			lineEnding: view.LineEnding,
			mode:       view.Mode,
			mtimeMs:    mtimeMs,
		}, nil
	}

	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file '%s': %w", fileName, err)
	}
	if textfile.IsProbablyBinary(raw) {
		return nil, notTextFileErr("")
	}
	content, encoding, err := textfile.DecodeTextBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding file '%s': %w", fileName, err)
	}
	snap := &editSnapshot{
		filePath:   filePath,
		content:    content,
		encoding:   encoding,
		lineEnding: textfile.DetectLineEnding(raw),
		mode:       stat.Mode(),
		mtimeMs:    mtimeMs,
	}
	toolcache.StoreFileViewFromContext(ctx, filePath, toolcache.FileView{
		Content:    snap.content,
		MtimeMs:    snap.mtimeMs,
		Encoding:   snap.encoding,
		LineEnding: snap.lineEnding,
		Mode:       snap.mode,
	})
	return snap, nil
}

// commitEditSnapshot serializes newContent using the snapshot's
// encoding and line ending, writes it atomically (temp file + rename)
// preserving the original file mode, and refreshes the FileView cache.
// It returns the new file mtime in milliseconds.
func (f *fileToolSet) commitEditSnapshot(
	ctx context.Context,
	snap *editSnapshot,
	newContent string,
) (int64, error) {
	data, err := textfile.EncodeTextBytes(
		newContent,
		snap.encoding,
		snap.lineEnding,
	)
	if err != nil {
		return 0, fmt.Errorf("encoding file: %w", err)
	}
	if err := atomicWriteFile(snap.filePath, data, snap.mode); err != nil {
		return 0, fmt.Errorf("writing file: %w", err)
	}
	stat, err := os.Stat(snap.filePath)
	if err != nil {
		return 0, fmt.Errorf("stat file after write: %w", err)
	}
	mtimeMs := stat.ModTime().UnixMilli()
	toolcache.StoreFileViewFromContext(ctx, snap.filePath, toolcache.FileView{
		Content:    newContent,
		MtimeMs:    mtimeMs,
		Encoding:   snap.encoding,
		LineEnding: snap.lineEnding,
		Mode:       snap.mode,
	})
	return mtimeMs, nil
}

// atomicWriteFile writes data to a temp file in the same directory and
// renames it over filePath, so readers never observe a partial write.
func atomicWriteFile(filePath string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(filePath)
	tmp, err := os.CreateTemp(dir, ".edit-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Chmod(tmpName, mode); err != nil {
		return err
	}
	return os.Rename(tmpName, filePath)
}

// storeFileViewFromDisk refreshes the FileView cache for filePath by
// reading and decoding the file from disk. It is a best-effort helper
// used after write operations; failures are ignored.
func (f *fileToolSet) storeFileViewFromDisk(
	ctx context.Context,
	filePath string,
) {
	raw, err := os.ReadFile(filePath)
	if err != nil || textfile.IsProbablyBinary(raw) {
		return
	}
	content, encoding, err := textfile.DecodeTextBytes(raw)
	if err != nil {
		return
	}
	stat, err := os.Stat(filePath)
	if err != nil {
		return
	}
	toolcache.StoreFileViewFromContext(ctx, filePath, toolcache.FileView{
		Content:    content,
		MtimeMs:    stat.ModTime().UnixMilli(),
		Encoding:   encoding,
		LineEnding: textfile.DetectLineEnding(raw),
		Mode:       stat.Mode(),
	})
}
