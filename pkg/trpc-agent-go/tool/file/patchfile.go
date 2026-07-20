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
	"trpc.group/trpc-go/trpc-agent-go/tool/file/patch"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

// patchFileRequest represents the input for the patch file operation.
// Patch and Hunks are mutually exclusive.
type patchFileRequest struct {
	FileName string `json:"file_name" jsonschema:"description=Relative file path under base_directory"`
	Patch    string `json:"patch,omitempty" jsonschema:"description=Unified diff text; mutually exclusive with hunks"`
	Hunks    []patch.Hunk `json:"hunks,omitempty" jsonschema:"description=Structured hunks with ' ' '-' '+' prefixed body lines; mutually exclusive with patch"`
	ExpectedMtimeMs *int64 `json:"expected_mtime_ms,omitempty" jsonschema:"description=Optional optimistic lock from a prior read_file mtime_ms"`
}

// patchFileResponse represents the output from the patch file
// operation. Recoverable failures are reported through the structured
// Error fields with a nil Go error so the model can self-correct.
type patchFileResponse struct {
	BaseDirectory string   `json:"base_directory"`
	FileName      string   `json:"file_name"`
	AppliedHunks  int      `json:"applied_hunks,omitempty"`
	MtimeMs       int64    `json:"mtime_ms,omitempty"`
	Message       string   `json:"message"`
	Error         string   `json:"error,omitempty"`
	HunkIndex     *int     `json:"hunk_index,omitempty"`
	ExpectedLines []string `json:"expected_lines,omitempty"`
	ActualLines   []string `json:"actual_lines,omitempty"`
	Hint          string   `json:"hint,omitempty"`
}

// patchFile applies a unified diff or structured hunks to a file.
// All hunks are applied in memory first; the file is written
// atomically only when every hunk applies.
func (f *fileToolSet) patchFile(
	ctx context.Context,
	req *patchFileRequest,
) (*patchFileResponse, error) {
	rsp := &patchFileResponse{
		BaseDirectory: f.baseDir,
		FileName:      req.FileName,
	}
	hunks, err := f.validatePatchFileRequest(req)
	if err != nil {
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

	content, err := patch.Apply(snap.content, hunks)
	if err != nil {
		var mismatch *patch.MismatchError
		if errors.As(err, &mismatch) {
			idx := mismatch.HunkIndex
			rsp.Error = errHunkMismatch
			rsp.HunkIndex = &idx
			rsp.ExpectedLines = prefixLines(mismatch.Expected, "-")
			rsp.ActualLines = prefixLines(mismatch.Actual, "-")
			rsp.Hint = "Re-read file and regenerate patch"
			rsp.Message = fmt.Sprintf("Error: %v", err)
			return rsp, nil
		}
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	if content == snap.content {
		rsp.Message = "no changes made"
		return rsp, nil
	}

	mtimeMs, err := f.commitEditSnapshot(ctx, snap, content)
	if err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.AppliedHunks = len(hunks)
	rsp.MtimeMs = mtimeMs
	rsp.Message = fmt.Sprintf(
		"Successfully applied %d hunk(s) to '%s'",
		len(hunks),
		req.FileName,
	)
	return rsp, nil
}

func (f *fileToolSet) validatePatchFileRequest(
	req *patchFileRequest,
) ([]patch.Hunk, error) {
	if req == nil {
		return nil, fmt.Errorf("request cannot be nil")
	}
	if strings.TrimSpace(req.FileName) == "" {
		return nil, fmt.Errorf("file name cannot be empty")
	}
	ref, err := fileref.Parse(req.FileName)
	if err != nil {
		return nil, err
	}
	if ref.Scheme != "" {
		return nil, fmt.Errorf(
			"patch_file does not support %s:// refs",
			ref.Scheme,
		)
	}
	hasPatch := req.Patch != ""
	hasHunks := len(req.Hunks) > 0
	if hasPatch && hasHunks {
		return nil, fmt.Errorf(
			"patch and hunks are mutually exclusive; provide exactly one",
		)
	}
	if !hasPatch && !hasHunks {
		return nil, fmt.Errorf("either patch or hunks must be provided")
	}
	if hasPatch {
		if len(req.Patch) > maxPatchBytes {
			return nil, fmt.Errorf(
				"patch is too large: %d > %d",
				len(req.Patch),
				maxPatchBytes,
			)
		}
		hunks, err := patch.ParseUnified(req.Patch)
		if err != nil {
			return nil, err
		}
		if len(hunks) == 0 {
			return nil, fmt.Errorf("patch contains no hunks")
		}
		if err := patch.Validate(hunks); err != nil {
			return nil, err
		}
		return hunks, nil
	}
	if err := patch.Validate(req.Hunks); err != nil {
		return nil, err
	}
	return req.Hunks, nil
}

// prefixLines returns a copy of lines each prefixed with prefix, for
// structured mismatch reporting.
func prefixLines(lines []string, prefix string) []string {
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		out = append(out, prefix+line)
	}
	return out
}

// patchFileTool returns a callable tool for applying unified diffs.
func (f *fileToolSet) patchFileTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.patchFile,
		function.WithName("patch_file"),
		function.WithDescription(
			"Apply a unified diff or structured hunks to a text file. "+
				"Hunks are applied atomically with drift tolerance; "+
				"on mismatch the file is left unchanged.",
		),
	)
}
