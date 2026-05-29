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
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/internal/toolcache"
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

const (
	maxEditsPerCall     = 20
	maxPatchBytes       = 256 * 1024
	maxEditSearchBytes  = 64 * 1024
	maxEditReplaceBytes = 256 * 1024
)

// editFileSnapshot is an in-memory view of a workspace text file for editing.
type editFileSnapshot struct {
	Exists     bool
	AbsPath    string
	RelPath    string
	Content    string
	Mode       os.FileMode
	MtimeMs    int64
	Encoding   string
	LineEnding string
}

func (f *fileToolSet) loadEditSnapshot(
	ctx context.Context,
	relPath string,
	expectedMtimeMs *int64,
) (*editFileSnapshot, error) {
	filePath, err := f.resolvePath(relPath)
	if err != nil {
		return nil, err
	}
	if view, ok := toolcache.LookupFileViewFromContext(ctx, filePath); ok {
		if expectedMtimeMs != nil && view.MtimeMs != *expectedMtimeMs {
			return nil, fmt.Errorf(
				"file_modified_externally: mtime mismatch; call read_file again",
			)
		}
		st, statErr := os.Stat(filePath)
		if statErr == nil {
			if fileMtimeMs(st) == view.MtimeMs {
				return &editFileSnapshot{
					Exists:     true,
					AbsPath:    filePath,
					RelPath:    relPath,
					Content:    view.Content,
					Mode:       view.Mode,
					MtimeMs:    view.MtimeMs,
					Encoding:   view.Encoding,
					LineEnding: view.LineEnding,
				}, nil
			}
			toolcache.InvalidateFileViewFromContext(ctx, filePath)
		} else if os.IsNotExist(statErr) {
			return &editFileSnapshot{
				Exists:     true,
				AbsPath:    filePath,
				RelPath:    relPath,
				Content:    view.Content,
				Mode:       view.Mode,
				MtimeMs:    view.MtimeMs,
				Encoding:   view.Encoding,
				LineEnding: view.LineEnding,
			}, nil
		} else {
			return nil, fmt.Errorf("accessing file '%s': %w", relPath, statErr)
		}
	}
	st, err := os.Stat(filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return &editFileSnapshot{
				Exists:  false,
				AbsPath: filePath,
				RelPath: relPath,
			}, nil
		}
		return nil, fmt.Errorf("accessing file '%s': %w", relPath, err)
	}
	if st.IsDir() {
		return nil, fmt.Errorf("target path '%s' is a directory", relPath)
	}
	if st.Size() > f.maxFileSize {
		return nil, fmt.Errorf("file is too large: %d > %d", st.Size(), f.maxFileSize)
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("reading file '%s': %w", relPath, err)
	}
	mtimeMs := fileMtimeMs(st)
	if expectedMtimeMs != nil && mtimeMs != *expectedMtimeMs {
		return nil, fmt.Errorf(
			"file_modified_externally: mtime mismatch; call read_file again",
		)
	}
	if textfile.IsProbablyBinary(raw) {
		return nil, fmt.Errorf("cannot edit binary file '%s'", relPath)
	}
	if strings.HasSuffix(strings.ToLower(filePath), ".ipynb") {
		return nil, fmt.Errorf("cannot edit notebook '%s' with file tools", relPath)
	}
	content, encoding, err := textfile.DecodeBytes(raw)
	if err != nil {
		return nil, fmt.Errorf("decoding file '%s': %w", relPath, err)
	}
	snap := &editFileSnapshot{
		Exists:     true,
		AbsPath:    filePath,
		RelPath:    relPath,
		Content:    content,
		Mode:       st.Mode(),
		MtimeMs:    mtimeMs,
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
		return fmt.Errorf("snapshot is nil")
	}
	encoded, err := textfile.EncodeBytes(newContent, snap.Encoding, snap.LineEnding)
	if err != nil {
		return err
	}
	mode := snap.Mode
	if mode == 0 {
		mode = f.createFileMode
	}
	if err := os.MkdirAll(filepath.Dir(snap.AbsPath), f.createDirMode); err != nil {
		return fmt.Errorf("creating directory: %w", err)
	}
	if err := os.WriteFile(snap.AbsPath, encoded, mode); err != nil {
		return fmt.Errorf("writing file '%s': %w", snap.RelPath, err)
	}
	st, err := os.Stat(snap.AbsPath)
	if err != nil {
		return err
	}
	snap.Exists = true
	snap.Content = newContent
	snap.MtimeMs = fileMtimeMs(st)
	snap.Mode = st.Mode()
	f.storeFileView(ctx, snap)
	return nil
}

func (f *fileToolSet) storeFileView(ctx context.Context, snap *editFileSnapshot) {
	if snap == nil || !snap.Exists {
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

func fileMtimeMs(st os.FileInfo) int64 {
	if st == nil {
		return 0
	}
	return st.ModTime().UnixMilli()
}

func storeFileViewAfterRead(
	ctx context.Context,
	filePath string,
	raw []byte,
	st os.FileInfo,
) {
	if st == nil || st.IsDir() || textfile.IsProbablyBinary(raw) {
		return
	}
	content, encoding, err := textfile.DecodeBytes(raw)
	if err != nil {
		return
	}
	toolcache.StoreFileViewFromContext(ctx, filePath, toolcache.FileView{
		Content:    content,
		MtimeMs:    fileMtimeMs(st),
		Encoding:   encoding,
		LineEnding: textfile.DetectLineEnding(raw),
		Mode:       st.Mode(),
	})
}

func storeSaveFileView(ctx context.Context, filePath string, _ string, _ os.FileMode) {
	st, err := os.Stat(filePath)
	if err != nil {
		return
	}
	raw, err := os.ReadFile(filePath)
	if err != nil {
		return
	}
	storeFileViewAfterRead(ctx, filePath, raw, st)
}

func optionalExpectedMtime(v *int64) *int64 {
	if v == nil || *v == 0 {
		return nil
	}
	out := *v
	return &out
}
