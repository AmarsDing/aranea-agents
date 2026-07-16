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
	"trpc.group/trpc-go/trpc-agent-go/tool/file/patch"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type patchFileRequest struct {
	FileName        string       `json:"file_name" jsonschema:"description=Relative file path under base_directory"`
	Patch           string       `json:"patch,omitempty" jsonschema:"description=Unified diff text; mutually exclusive with hunks"`
	Hunks           []patch.Hunk `json:"hunks,omitempty" jsonschema:"description=Structured hunks; mutually exclusive with patch"`
	ExpectedMtimeMs *int64       `json:"expected_mtime_ms,omitempty" jsonschema:"description=Optional optimistic lock from prior read_file mtime_ms"`
}

type patchFileResponse struct {
	BaseDirectory string   `json:"base_directory"`
	FileName      string   `json:"file_name"`
	Message       string   `json:"message"`
	MtimeMs       int64    `json:"mtime_ms,omitempty"`
	FromCache     bool     `json:"from_cache,omitempty"`
	Error         string   `json:"error,omitempty"`
	HunkIndex     int      `json:"hunk_index,omitempty"`
	ExpectedLines []string `json:"expected_lines,omitempty"`
	ActualLines   []string `json:"actual_lines,omitempty"`
	Hint          string   `json:"hint,omitempty"`
}

func (f *fileToolSet) patchFile(
	ctx context.Context,
	req *patchFileRequest,
) (*patchFileResponse, error) {
	rsp := &patchFileResponse{
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
	hasPatch := strings.TrimSpace(req.Patch) != ""
	hasHunks := len(req.Hunks) > 0
	if hasPatch == hasHunks {
		rsp.Error = "invalid_request"
		rsp.Message = "Error: provide exactly one of patch or hunks"
		return rsp, fmt.Errorf("provide exactly one of patch or hunks")
	}
	if hasPatch && len(req.Patch) > maxPatchBytes {
		rsp.Error = "invalid_request"
		rsp.Message = "Error: patch exceeds size limit"
		return rsp, fmt.Errorf("patch exceeds size limit")
	}

	var hunks []patch.Hunk
	var err error
	if hasPatch {
		hunks, err = patch.ParseUnified(req.Patch)
		if err != nil {
			rsp.Error = "invalid_patch"
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, err
		}
	} else {
		hunks = req.Hunks
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

	updated, err := patch.Apply(snap.Content, hunks)
	if err != nil {
		if me, ok := err.(*patch.MismatchError); ok {
			rsp.Error = "hunk_mismatch"
			rsp.HunkIndex = me.HunkIndex
			rsp.ExpectedLines = me.ExpectedLines
			rsp.ActualLines = me.ActualLines
			rsp.Hint = "Re-read file and regenerate patch"
		}
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}

	if err := f.commitEditSnapshot(ctx, snap, updated); err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.MtimeMs = snap.MtimeMs
	rsp.Message = fmt.Sprintf("Successfully patched '%s'", req.FileName)
	return rsp, nil
}

func (f *fileToolSet) patchFileTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.patchFile,
		function.WithName("patch_file"),
		function.WithDescription(
			"Apply a unified diff or structured hunks to a text file under "+
				"base_directory. Fails atomically when any hunk does not match.",
		),
	)
}
