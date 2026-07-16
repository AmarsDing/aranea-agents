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
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

type diffEditRequest struct {
	FileName        string          `json:"file_name" jsonschema:"description=Relative file path under base_directory"`
	Edits           []diffEditItem  `json:"edits" jsonschema:"description=Ordered search/replace edits applied atomically"`
	ExpectedMtimeMs *int64          `json:"expected_mtime_ms,omitempty" jsonschema:"description=Optional optimistic lock from prior read_file mtime_ms"`
}

type diffEditItem struct {
	Search     string `json:"search" jsonschema:"description=Text to find; multi-line allowed"`
	Replace    string `json:"replace" jsonschema:"description=Replacement text; empty string deletes"`
	ReplaceAll bool   `json:"replace_all,omitempty" jsonschema:"description=When true, replace all matches for this edit"`
}

type diffEditResponse struct {
	BaseDirectory string `json:"base_directory"`
	FileName      string `json:"file_name"`
	Message       string `json:"message"`
	MtimeMs       int64  `json:"mtime_ms,omitempty"`
	FromCache     bool   `json:"from_cache,omitempty"`
	Error         string `json:"error,omitempty"`
	EditIndex     int    `json:"edit_index,omitempty"`
	MatchCount    int    `json:"match_count,omitempty"`
	MatchLines    []int  `json:"match_lines,omitempty"`
	Hint          string `json:"hint,omitempty"`
}

func (f *fileToolSet) diffEdit(
	ctx context.Context,
	req *diffEditRequest,
) (*diffEditResponse, error) {
	rsp := &diffEditResponse{
		BaseDirectory: f.baseDir,
	}
	if req == nil {
		rsp.Error = "invalid_request"
		rsp.Message = "Error: request cannot be nil"
		return rsp, fmt.Errorf("request cannot be nil")
	}
	rsp.FileName = req.FileName
	if strings.TrimSpace(req.FileName) == "" {
		rsp.Error = "invalid_request"
		rsp.Message = "Error: file_name is required"
		return rsp, fmt.Errorf("file_name is required")
	}
	if len(req.Edits) == 0 {
		rsp.Error = "invalid_request"
		rsp.Message = "Error: edits cannot be empty"
		return rsp, fmt.Errorf("edits cannot be empty")
	}
	if len(req.Edits) > maxEditsPerCall {
		rsp.Error = "invalid_request"
		rsp.Message = fmt.Sprintf("Error: at most %d edits per call", maxEditsPerCall)
		return rsp, fmt.Errorf("at most %d edits per call", maxEditsPerCall)
	}
	for i, ed := range req.Edits {
		if len(ed.Search) > maxEditSearchBytes {
			rsp.Error = "invalid_request"
			rsp.EditIndex = i
			rsp.Message = fmt.Sprintf("Error: search block too large at edit %d", i)
			return rsp, fmt.Errorf("search too large at edit %d", i)
		}
		if len(ed.Replace) > maxEditReplaceBytes {
			rsp.Error = "invalid_request"
			rsp.EditIndex = i
			rsp.Message = fmt.Sprintf("Error: replace block too large at edit %d", i)
			return rsp, fmt.Errorf("replace too large at edit %d", i)
		}
	}

	snap, err := f.loadEditSnapshot(ctx, req.FileName, req.ExpectedMtimeMs)
	if err != nil {
		if strings.Contains(err.Error(), errFileModifiedExt) {
			rsp.Error = errFileModifiedExt
			rsp.Hint = "Call read_file again before editing"
		}
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.FromCache = snap.FromCache

	content := snap.Content
	// Create-new-file path: single edit with empty search on missing/empty file.
	if len(req.Edits) == 1 && req.Edits[0].Search == "" {
		if snap.Exists && strings.TrimSpace(content) != "" {
			rsp.Error = "invalid_request"
			rsp.Message = "Error: empty search only allowed when creating a new or empty file"
			return rsp, fmt.Errorf("empty search only for new/empty file")
		}
		content = req.Edits[0].Replace
	} else {
		for i, ed := range req.Edits {
			if ed.Search == "" {
				rsp.Error = "invalid_request"
				rsp.EditIndex = i
				rsp.Message = fmt.Sprintf("Error: search cannot be empty at edit %d", i)
				return rsp, fmt.Errorf("search cannot be empty at edit %d", i)
			}
			actual := textfile.FindActualString(content, ed.Search)
			if actual == "" {
				rsp.Error = "edit_not_found"
				rsp.EditIndex = i
				rsp.MatchCount = 0
				rsp.Hint = "Re-read the file and provide an exact search block"
				rsp.Message = fmt.Sprintf("Error: search not found at edit %d", i)
				return rsp, fmt.Errorf("search not found at edit %d", i)
			}
			count := strings.Count(content, actual)
			if count > 1 && !ed.ReplaceAll {
				rsp.Error = "edit_not_unique"
				rsp.EditIndex = i
				rsp.MatchCount = count
				rsp.MatchLines = lineNumbersOfMatches(content, actual)
				rsp.Hint = "Add more context to search or set replace_all=true"
				rsp.Message = fmt.Sprintf(
					"Error: edit %d matched %d times; refine search or set replace_all",
					i, count,
				)
				return rsp, fmt.Errorf("edit_not_unique at edit %d", i)
			}
			replacement := textfile.PreserveQuoteStyle(ed.Search, actual, ed.Replace)
			n := 1
			if ed.ReplaceAll {
				n = -1
			}
			content = strings.Replace(content, actual, replacement, n)
		}
	}

	if err := f.commitEditSnapshot(ctx, snap, content); err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.MtimeMs = snap.MtimeMs
	rsp.Message = fmt.Sprintf(
		"Successfully applied %d edit(s) to '%s'",
		len(req.Edits),
		req.FileName,
	)
	return rsp, nil
}

func (f *fileToolSet) diffEditTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.diffEdit,
		function.WithName("diff_edit"),
		function.WithDescription(
			"Apply one or more search/replace edits to a text file under "+
				"base_directory. All edits are validated in memory and committed "+
				"atomically. Prefer this over save_file for existing source files.",
		),
	)
}
