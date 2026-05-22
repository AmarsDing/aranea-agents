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

type patchFileRequest struct {
	FileName        string       `json:"file_name" jsonschema:"description=Relative file path under base_directory"`
	Patch           string       `json:"patch,omitempty" jsonschema:"description=Unified diff text; mutually exclusive with hunks"`
	Hunks           []patch.Hunk `json:"hunks,omitempty" jsonschema:"description=Structured hunks; mutually exclusive with patch"`
	ExpectedMtimeMs *int64       `json:"expected_mtime_ms,omitempty" jsonschema:"description=Optional mtime from a prior read_file"`
}

type patchFileResponse struct {
	BaseDirectory   string       `json:"base_directory"`
	FileName        string       `json:"file_name"`
	AppliedHunks    int          `json:"applied_hunks"`
	StructuredPatch []patch.Hunk `json:"structured_patch,omitempty"`
	Message         string       `json:"message"`
	Error           string       `json:"error,omitempty"`
	HunkIndex       *int         `json:"hunk_index,omitempty"`
	ExpectedLines   []string     `json:"expected_lines,omitempty"`
	ActualLines     []string     `json:"actual_lines,omitempty"`
	Hint            string       `json:"hint,omitempty"`
}

func (f *fileToolSet) patchFile(
	ctx context.Context,
	req *patchFileRequest,
) (*patchFileResponse, error) {
	rsp := &patchFileResponse{
		BaseDirectory: f.baseDir,
		FileName:      req.FileName,
	}
	if err := validatePatchFileRequest(req); err != nil {
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
		err = fmt.Errorf("patch_file does not support %s:// refs", ref.Scheme)
		rsp.Message = "Error: " + err.Error()
		return rsp, err
	}
	hunks := req.Hunks
	if strings.TrimSpace(req.Patch) != "" {
		parsed, err := patch.ParseUnified(req.Patch)
		if err != nil {
			rsp.Message = fmt.Sprintf("Error: %v", err)
			rsp.Error = err.Error()
			return rsp, err
		}
		hunks = parsed
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
	oldContent := snap.Content
	if !snap.Exists {
		oldContent = ""
	}
	newContent, err := patch.ApplyHunks(oldContent, hunks)
	if err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		rsp.Error = err.Error()
		rsp.Hint = "Re-read file and regenerate patch"
		if he, ok := err.(*patch.HunkMismatchError); ok {
			rsp.HunkIndex = &he.HunkIndex
			rsp.ExpectedLines = he.ExpectedLines
			rsp.ActualLines = he.ActualLines
		}
		return rsp, err
	}
	if oldContent == newContent {
		rsp.Message = "No changes made"
		return rsp, nil
	}
	if !snap.Exists {
		snap = &editFileSnapshot{
			Exists:     false,
			AbsPath:    snap.AbsPath,
			RelPath:    snap.RelPath,
			Encoding:   "utf8",
			LineEnding: "\n",
		}
	}
	if err := f.commitEditSnapshot(ctx, snap, newContent); err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.AppliedHunks = len(hunks)
	rsp.StructuredPatch = patch.BuildStructured(oldContent, newContent)
	rsp.Message = fmt.Sprintf("Applied %d hunk(s) to '%s'", rsp.AppliedHunks, req.FileName)
	return rsp, nil
}

func validatePatchFileRequest(req *patchFileRequest) error {
	if req == nil {
		return fmt.Errorf("request cannot be nil")
	}
	if strings.TrimSpace(req.FileName) == "" {
		return fmt.Errorf("file_name cannot be empty")
	}
	hasPatch := strings.TrimSpace(req.Patch) != ""
	hasHunks := len(req.Hunks) > 0
	if hasPatch == hasHunks {
		return fmt.Errorf("provide exactly one of patch or hunks")
	}
	if hasPatch && len(req.Patch) > maxPatchBytes {
		return fmt.Errorf("patch exceeds max size of %d bytes", maxPatchBytes)
	}
	if hasHunks && len(req.Hunks) > maxEditsPerCall {
		return fmt.Errorf("too many hunks: max %d", maxEditsPerCall)
	}
	return nil
}

func (f *fileToolSet) patchFileTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.patchFile,
		function.WithName("patch_file"),
		function.WithDescription(
			"Apply a unified diff or structured hunk list to a text file under base_directory. "+
				"Read the file first (use mtime_ms as expected_mtime_ms when needed). "+
				"Do not invoke patch_file on the same file in parallel. "+
				"Any hunk mismatch aborts without writing. "+
				"Use diff_edit for search/replace fragments instead of diff format.",
		),
	)
}
