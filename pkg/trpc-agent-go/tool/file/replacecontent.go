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
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
)

type replaceContentRequest struct {
	FileName string `json:"file_name" jsonschema:"description=Relative file path under base_directory to modify"`
	OldString string `json:"old_string" jsonschema:"description=Existing text to replace; supports multi-line content"`
	NewString string `json:"new_string" jsonschema:"description=Replacement text; supports multi-line content"`
	NumReplacements int `json:"num_replacements,omitempty" jsonschema:"description=Optional replacement limit; 0 means 1 and negative means replace all matches"`
}

type replaceContentResponse struct {
	BaseDirectory string `json:"base_directory"`
	FileName      string `json:"file_name"`
	Message       string `json:"message"`
}

func (f *fileToolSet) replaceContent(
	ctx context.Context,
	req *replaceContentRequest,
) (*replaceContentResponse, error) {
	rsp := &replaceContentResponse{
		BaseDirectory: f.baseDir,
		FileName:      req.FileName,
	}
	ref, err := fileref.Parse(req.FileName)
	if err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	if ref.Scheme != "" {
		rsp.Message = fmt.Sprintf(
			"Error: replace_content does not support %s:// refs",
			ref.Scheme,
		)
		return rsp, fmt.Errorf(
			"replace_content does not support %s:// refs",
			ref.Scheme,
		)
	}
	if req.OldString == "" {
		rsp.Message = "Error: old_string cannot be empty"
		return rsp, fmt.Errorf("old_string cannot be empty")
	}
	if req.OldString == req.NewString {
		rsp.Message = "old_string equals new_string; no changes made"
		return rsp, nil
	}
	snap, err := f.loadEditSnapshot(ctx, req.FileName, nil)
	if err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	if !snap.Exists {
		rsp.Message = fmt.Sprintf(
			"Error: file '%s' not found. %s",
			req.FileName,
			f.missingFileHint(),
		)
		return rsp, fmt.Errorf("file '%s' not found", req.FileName)
	}
	totalCount := strings.Count(snap.Content, req.OldString)
	if totalCount == 0 {
		rsp.Message = fmt.Sprintf(
			"'%s' not found in '%s'",
			req.OldString,
			req.FileName,
		)
		return rsp, nil
	}
	numReplacements := req.NumReplacements
	if numReplacements == 0 {
		numReplacements = 1
	}
	if numReplacements < 0 || numReplacements > totalCount {
		numReplacements = totalCount
	}
	newContent := strings.Replace(
		snap.Content,
		req.OldString,
		req.NewString,
		numReplacements,
	)
	if err := f.commitEditSnapshot(ctx, snap, newContent); err != nil {
		rsp.Message = fmt.Sprintf("Error: %v", err)
		return rsp, err
	}
	rsp.Message = fmt.Sprintf(
		"Successfully replaced %d of %d in '%s'",
		numReplacements,
		totalCount,
		req.FileName,
	)
	return rsp, nil
}

func (f *fileToolSet) replaceContentTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.replaceContent,
		function.WithName("replace_content"),
		function.WithDescription(
			"Replace a string in a file under base_directory. "+
				"Supports multi-line old_string/new_string. "+
				"num_replacements: 0 (default) replaces the first match, a positive number replaces up to that many matches, negative replaces all matches. "+
				"Does not support workspace:// or artifact:// refs. "+
				"For precise line-range edits, prefer diff_edit instead.",
		),
	)
}
