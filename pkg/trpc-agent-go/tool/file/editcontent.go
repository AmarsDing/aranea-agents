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
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/internal/toolcache"
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

const (
	maxEditsPerCall      = 20
	maxPatchBytes        = 256 * 1024
	maxEditSearchBytes   = 64 * 1024
	maxEditReplaceBytes  = 256 * 1024
	errFileModifiedExt   = "file_modified_externally"
	errNotEditableBinary = "binary or notebook files cannot be edited with this tool"
)

// editFileSnapshot is the in-memory editable view of a text file.
type editFileSnapshot struct {
	AbsPath    string
	Exists     bool
	Content    string
	Mode       os.FileMode
	MtimeMs    int64
	Encoding   string
	LineEnding string
	FromCache  bool
}

func (f *fileToolSet) loadEditSnapshot(
	ctx context.Context,
	relPath string,
	expectedMtimeMs *int64,
) (*editFileSnapshot, error) {
	absPath, err := f.resolvePath(relPath)
	if err != nil {
		return nil, err
	}
	if strings.HasSuffix(strings.ToLower(absPath), ".ipynb") {
		return nil, fmt.Errorf(errNotEditableBinary)
	}

	st, statErr := os.Stat(absPath)
	exists := statErr == nil && !st.IsDir()
	if statErr != nil && !os.IsNotExist(statErr) {
		return nil, fmt.Errorf("stat file %q: %w", relPath, statErr)
	}
	if exists && st.IsDir() {
		return nil, fmt.Errorf("target path %q is a directory", relPath)
	}

	diskMtime := int64(0)
	if exists {
		diskMtime = st.ModTime().UnixMilli()
		if f.maxFileSize > 0 && st.Size() > f.maxFileSize {
			return nil, fmt.Errorf("file is too large: %d > %d", st.Size(), f.maxFileSize)
		}
	}

	if expectedMtimeMs != nil && exists && diskMtime != *expectedMtimeMs {
		return nil, fmt.Errorf("%s: call read_file again before editing", errFileModifiedExt)
	}

	if view, ok := toolcache.LookupFileViewFromContext(ctx, absPath); ok {
		if !exists || view.MtimeMs == diskMtime {
			return &editFileSnapshot{
				AbsPath:    absPath,
				Exists:     exists || view.Content != "",
				Content:    view.Content,
				Mode:       view.Mode,
				MtimeMs:    view.MtimeMs,
				Encoding:   view.Encoding,
				LineEnding: view.LineEnding,
				FromCache:  true,
			}, nil
		}
		// Stale cache (disk mtime changed): fall through to disk read.
	}

	if !exists {
		return &editFileSnapshot{
			AbsPath:    absPath,
			Exists:     false,
			Content:    "",
			Mode:       f.createFileMode,
			MtimeMs:    0,
			Encoding:   "utf8",
			LineEnding: "\n",
		}, nil
	}

	raw, err := os.ReadFile(absPath)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", relPath, err)
	}
	if textfile.IsProbablyBinary(raw) {
		return nil, fmt.Errorf(errNotEditableBinary)
	}
	content, encoding, err := textfile.DecodeBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("decode file %q: %w", relPath, err)
	}
	snap := &editFileSnapshot{
		AbsPath:    absPath,
		Exists:     true,
		Content:    content,
		Mode:       st.Mode(),
		MtimeMs:    diskMtime,
		Encoding:   encoding,
		LineEnding: textfile.DetectLineEnding(raw),
	}
	f.storeFileView(ctx, snap)
	return snap, nil
}

func (f *fileToolSet) commitEditSnapshot(
	ctx context.Context,
	snap *editFileSnapshot,
	newContent string,
) error {
	if snap == nil {
		return fmt.Errorf("nil snapshot")
	}
	parentDir := filepath.Dir(snap.AbsPath)
	if err := os.MkdirAll(parentDir, f.createDirMode); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	mode := snap.Mode
	if mode == 0 {
		mode = f.createFileMode
	}
	encoded, err := textfile.EncodeBytes(newContent, snap.Encoding, snap.LineEnding)
	if err != nil {
		return err
	}
	if err := os.WriteFile(snap.AbsPath, encoded, mode); err != nil {
		return fmt.Errorf("writing file: %w", err)
	}
	st, err := os.Stat(snap.AbsPath)
	if err != nil {
		return fmt.Errorf("stat after write: %w", err)
	}
	snap.Content = newContent
	snap.Exists = true
	snap.MtimeMs = st.ModTime().UnixMilli()
	snap.Mode = st.Mode()
	f.storeFileView(ctx, snap)
	return nil
}

func (f *fileToolSet) storeFileView(ctx context.Context, snap *editFileSnapshot) {
	if snap == nil {
		return
	}
	toolcache.StoreFileViewFromContext(ctx, snap.AbsPath, toolcache.FileView{
		Content:    snap.Content,
		MtimeMs:    snap.MtimeMs,
		Encoding:   snap.Encoding,
		LineEnding: snap.LineEnding,
		Mode:       snap.Mode,
	})
}

func (f *fileToolSet) storeFileViewAfterRead(
	ctx context.Context,
	absPath string,
	content string,
	mtimeMs int64,
	mode os.FileMode,
	raw []byte,
) {
	encoding := "utf8"
	lineEnding := "\n"
	if len(raw) > 0 {
		decoded, enc, err := textfile.DecodeBytes(raw)
		if err == nil {
			content = decoded
			encoding = enc
		}
		lineEnding = textfile.DetectLineEnding(raw)
	}
	toolcache.StoreFileViewFromContext(ctx, absPath, toolcache.FileView{
		Content:    content,
		MtimeMs:    mtimeMs,
		Encoding:   encoding,
		LineEnding: lineEnding,
		Mode:       mode,
	})
}

func lineNumbersOfMatches(content, needle string) []int {
	if needle == "" {
		return nil
	}
	var out []int
	start := 0
	for {
		idx := strings.Index(content[start:], needle)
		if idx < 0 {
			break
		}
		abs := start + idx
		line := 1 + strings.Count(content[:abs], "\n")
		out = append(out, line)
		start = abs + len(needle)
		if start >= len(content) {
			break
		}
	}
	return out
}
