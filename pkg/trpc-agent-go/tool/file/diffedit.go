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
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/internal/fileref"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/file/patch"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type diffEditRequest struct {
	FileName          string          `json:"file_name" jsonschema:"description=Relative file path under base_directory"`
	Edits             []diffEditItem  `json:"edits" jsonschema:"description=One or more search/replace edits to apply atomically"`
	ExpectedMtimeMs   *int64          `json:"expected_mtime_ms,omitempty" jsonschema:"description=Optional mtime from a prior read_file for optimistic locking"`
}

type diffEditItem struct {
	Search      string `json:"search" jsonschema:"description=Text to find in the file; multi-line allowed"`
	Replace     string `json:"replace" jsonschema:"description=Replacement text; empty string deletes matched text"`
	ReplaceAll  bool   `json:"replace_all,omitempty" jsonschema:"description=Replace all matches; default false requires a unique match"`
}

type diffEditResponse struct {
	BaseDirectory    string       `json:"base_directory"`
	FileName         string       `json:"file_name"`
	AppliedEdits     int          `json:"applied_edits"`
	TotalReplacements int         `json:"total_replacements"`
	StructuredPatch  []patch.Hunk `json:"structured_patch,omitempty"`
	Message          string       `json:"message"`
	Error            string       `json:"error,omitempty"`
	EditIndex        *int         `json:"edit_index,omitempty"`
	MatchCount       *int         `json:"match_count,omitempty"`
	MatchLines       []int        `json:"match_lines,omitempty"`
	Hint             string       `json:"hint,omitempty"`
}

func (f *fileToolSet) diffEdit(
	ctx context.Context,
	req *diffEditRequest,
) (*diffEditResponse, error) {
	rsp := &diffEditResponse{
		BaseDirectory: f.baseDir,
		FileName:      req.FileName,
	}
	if err := validateDiffEditRequest(req); err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		rsp.Error = err.Error()
		return rsp, err
	}
	ref, err := fileref.Parse(req.FileName)
	if err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	if ref.Scheme != "" {
		err = fmt.Errorf("diff_edit does not support %s:// refs", ref.Scheme)
		rsp.Message = "Error: " + err.Error()
		return rsp, err
	}
	snap, err := f.loadEditSnapshot(ctx, req.FileName, optionalExpectedMtime(req.ExpectedMtimeMs))
	if err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		rsp.Error = err.Error()
		if strings.Contains(err.Error(), "file_modified_externally") {
			rsp.Hint = "Call read_file again before editing"
		}
		return rsp, err
	}
	edits := make([]patch.Edit, len(req.Edits))
	for i, item := range req.Edits {
		edits[i] = patch.Edit{
			Search:     item.Search,
			Replace:    item.Replace,
			ReplaceAll: item.ReplaceAll,
		}
	}
	oldContent := snap.Content
	newContent := oldContent
	if !snap.Exists {
		if len(edits) != 1 || edits[0].Search != "" {
			err = fmt.Errorf("file does not exist: %s", req.FileName)
			rsp.Message = "Error: " + err.Error()
			return rsp, err
		}
		newContent = edits[0].Replace
		rsp.AppliedEdits = 1
		snap = &editFileSnapshot{
			Exists:     false,
			AbsPath:    snap.AbsPath,
			RelPath:    snap.RelPath,
			Encoding:   "utf8",
			LineEnding: "\n",
		}
	} else {
		if err := validateDiffEditEditsForExistingFile(req.Edits); err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			rsp.Error = err.Error()
			return rsp, err
		}
		var total, applied int
		newContent, total, applied, err = patch.ApplyEdits(snap.Content, edits)
		if err != nil {
			return f.diffEditErrorResponse(rsp, err)
		}
		rsp.TotalReplacements = total
		rsp.AppliedEdits = applied
	}
	if oldContent == newContent && snap.Exists {
		rsp.Message = "No changes made"
		return rsp, nil
	}
	if err := f.commitEditSnapshot(ctx, snap, newContent); err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.StructuredPatch = patch.BuildStructured(oldContent, newContent)
	rsp.Message = fmt.Sprintf(
		"Applied %d edit(s) (%d replacement(s)) to '%s'",
		rsp.AppliedEdits,
		rsp.TotalReplacements,
		req.FileName,
	)
	return rsp, nil
}

func validateDiffEditRequest(req *diffEditRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}
	if strings.TrimSpace(req.FileName) == "" {
		return fmt.Errorf("file_name cannot be empty")
	}
	if len(req.Edits) == 0 {
		return fmt.Errorf("edits cannot be empty")
	}
	if len(req.Edits) > maxEditsPerCall {
		return fmt.Errorf("too many edits: max %d", maxEditsPerCall)
	}
	for i, edit := range req.Edits {
		if len(edit.Search) > maxEditSearchBytes {
			return fmt.Errorf("edit %d search exceeds max size", i)
		}
		if len(edit.Replace) > maxEditReplaceBytes {
			return fmt.Errorf("edit %d replace exceeds max size", i)
		}
	}
	return nil
}

func validateDiffEditEditsForExistingFile(edits []diffEditItem) error {
	for i, edit := range edits {
		if edit.Search == "" {
			return fmt.Errorf("edit %d: search cannot be empty for existing file", i)
		}
	}
	return nil
}

func (f *fileToolSet) diffEditErrorResponse(
	rsp *diffEditResponse,
	err error,
) (*diffEditResponse, error) {
	rsp.Message = fmt.Sprintf("Error: %v", err)
	rsp.Error = err.Error()
	switch e := err.(type) {
	case *patch.EditNotUniqueError:
		rsp.EditIndex = &e.EditIndex
		rsp.MatchCount = &e.MatchCount
		rsp.MatchLines = e.MatchLines
		rsp.Hint = "Add more context to search or set replace_all=true"
	case *patch.EditNotFoundError:
		rsp.EditIndex = &e.EditIndex
		rsp.Hint = "Re-read the file and ensure search matches exactly"
	case *patch.HunkMismatchError:
		rsp.Hint = "Re-read file and regenerate patch"
	default:
		if strings.Contains(err.Error(), "file_modified_externally") {
			rsp.Hint = "Call read_file again before editing"
		}
	}
	return rsp, err
}

func (f *fileToolSet) diffEditTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.diffEdit,
		function.WithName("diff_edit"),
		function.WithDescription(
			"Apply one or more targeted search/replace edits to a text file under base_directory. "+
				"Read the file first (use mtime_ms as expected_mtime_ms when parallel edits are possible). "+
				"Do not invoke diff_edit on the same file in parallel. "+
				"Provide only changed fragments, not the whole file. "+
				"Prefer diff_edit over save_file for modifications. "+
				"Use patch_file when you already have unified diff output.",
		),
	)
}
