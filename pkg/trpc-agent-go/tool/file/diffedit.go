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
	"errors"
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/internal/fileref"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// Limits for fragment-level editing (design §13.4).
const (
	maxEditsPerCall     = 20
	maxEditSearchBytes  = 64 * 1024
	maxEditReplaceBytes = 256 * 1024
	maxPatchBytes       = 256 * 1024
)

// Structured error identifiers returned in tool responses so the
// model can recover without a round trip (design §13.3).
const (
	errEditNotUnique          = "edit_not_unique"
	errEditNotFound           = "edit_not_found"
	errHunkMismatch           = "hunk_mismatch"
	errFileModifiedExternally = "file_modified_externally"
)

// diffEditItem is one SEARCH/REPLACE fragment.
type diffEditItem struct {
	Search     string `json:"search" jsonschema:"description=Text to find; multi-line allowed"`
	Replace    string `json:"replace" jsonschema:"description=Replacement text; empty string deletes the match"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=Replace every occurrence instead of requiring a unique match"`
}

// diffEditRequest represents the input for the diff edit operation.
type diffEditRequest struct {
	FileName        string         `json:"file_name" jsonschema:"description=Relative file path under base_directory"`
	Edits           []diffEditItem `json:"edits" jsonschema:"description=Search/replace fragments applied atomically in order"`
	ExpectedMtimeMs *int64         `json:"expected_mtime_ms,omitempty" jsonschema:"description=Optional optimistic lock from a prior read_file mtime_ms"`
}

// diffEditResponse represents the output from the diff edit operation.
// Recoverable failures are reported through the structured Error
// fields with a nil Go error so the model can self-correct.
type diffEditResponse struct {
	BaseDirectory string   `json:"base_directory"`
	FileName      string   `json:"file_name"`
	AppliedEdits  int      `json:"applied_edits,omitempty"`
	MtimeMs       int64    `json:"mtime_ms,omitempty"`
	Message       string   `json:"message"`
	Error         string   `json:"error,omitempty"`
	EditIndex     *int     `json:"edit_index,omitempty"`
	MatchCount    int      `json:"match_count,omitempty"`
	MatchLines    []int    `json:"match_lines,omitempty"`
	Hint          string   `json:"hint,omitempty"`
}

// diffEdit applies multiple search/replace fragments atomically: all
// edits are applied in memory first and the file is written only when
// every edit succeeds.
func (f *fileToolSet) diffEdit(
	ctx context.Context,
	req *diffEditRequest,
) (*diffEditResponse, error) {
	rsp := &diffEditResponse{
		BaseDirectory: f.baseDir,
		FileName:      req.FileName,
	}
	if err := f.validateDiffEditRequest(req); err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}

	snap, err := f.loadEditSnapshot(
		ctx,
		req.FileName,
		req.ExpectedMtimeMs,
	)
	if err != nil {
		var modErr *fileModifiedExternallyError
		if errors.As(err, &modErr) {
			rsp.Error = errFileModifiedExternally
			rsp.Hint = "Call read_file again before editing"
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, nil
		}
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}

	content := snap.content
	applied := 0
	for i, edit := range req.Edits {
		if edit.Search == edit.Replace {
			continue
		}
		actual := textfile.FindActualString(content, edit.Search)
		if actual == "" {
			idx := i
			rsp.Error = errEditNotFound
			rsp.EditIndex = &idx
			rsp.Hint = "Re-read the file and verify the search text"
			rsp.Message = fmt.Sprintf(
				"Error: edit %d search text not found in '%s'",
				i,
				req.FileName,
			)
			return rsp, nil
		}
		count := strings.Count(content, actual)
		if count > 1 && !edit.ReplaceAll {
			idx := i
			rsp.Error = errEditNotUnique
			rsp.EditIndex = &idx
			rsp.MatchCount = count
			rsp.MatchLines = matchStartLines(content, actual)
			rsp.Hint = "Add more context to search or set replace_all=true"
			rsp.Message = fmt.Sprintf(
				"Error: edit %d search text matches %d locations",
				i,
				count,
			)
			return rsp, nil
		}
		replacement := textfile.PreserveQuoteStyle(
			edit.Search,
			actual,
			edit.Replace,
		)
		if edit.ReplaceAll {
			content = strings.ReplaceAll(content, actual, replacement)
		} else {
			content = strings.Replace(content, actual, replacement, 1)
		}
		applied++
	}

	if applied == 0 {
		rsp.Message = "no changes made"
		return rsp, nil
	}
	mtimeMs, err := f.commitEditSnapshot(ctx, snap, content)
	if err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.AppliedEdits = applied
	rsp.MtimeMs = mtimeMs
	rsp.Message = fmt.Sprintf(
		"Successfully applied %d edit(s) to '%s'",
		applied,
		req.FileName,
	)
	return rsp, nil
}

func (f *fileToolSet) validateDiffEditRequest(req *diffEditRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}
	if strings.TrimSpace(req.FileName) == "" {
		return fmt.Errorf("file name cannot be empty")
	}
	ref, err := fileref.Parse(req.FileName)
	if err != nil {
		return err
	}
	if ref.Scheme != "" {
		return fmt.Errorf(
			"diff_edit does not support %s:// refs",
			ref.Scheme,
		)
	}
	if len(req.Edits) == 0 {
		return fmt.Errorf("edits cannot be empty")
	}
	if len(req.Edits) > maxEditsPerCall {
		return fmt.Errorf(
			"too many edits: %d > %d",
			len(req.Edits),
			maxEditsPerCall,
		)
	}
	for i, edit := range req.Edits {
		if edit.Search == "" {
			return fmt.Errorf("edit %d: search cannot be empty", i)
		}
		if len(edit.Search) > maxEditSearchBytes {
			return fmt.Errorf(
				"edit %d: search is too large: %d > %d",
				i,
				len(edit.Search),
				maxEditSearchBytes,
			)
		}
		if len(edit.Replace) > maxEditReplaceBytes {
			return fmt.Errorf(
				"edit %d: replace is too large: %d > %d",
				i,
				len(edit.Replace),
				maxEditReplaceBytes,
			)
		}
	}
	return nil
}

// matchStartLines returns the 1-based line numbers where each
// occurrence of sub starts in content.
func matchStartLines(content string, sub string) []int {
	var lines []int
	offset := 0
	for {
		idx := strings.Index(content[offset:], sub)
		if idx < 0 {
			break
		}
		abs := offset + idx
		lines = append(lines, 1+strings.Count(content[:abs], "\n"))
		offset = abs + len(sub)
	}
	return lines
}

// diffEditTool returns a callable tool for fragment-level editing.
func (f *fileToolSet) diffEditTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.diffEdit,
		function.WithName("diff_edit"),
		function.WithDescription(
			"Edit a text file with multiple search/replace fragments. "+
				"All edits apply atomically: if any fragment fails, "+
				"the file is left unchanged. Prefer this over "+
				"save_file for modifying existing files.",
		),
	)
}
