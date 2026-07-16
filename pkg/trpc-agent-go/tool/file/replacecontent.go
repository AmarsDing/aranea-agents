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

	"trpc.group/trpc-go/trpc-agent-go/internal/fileref"
	"trpc.group/trpc-go/trpc-agent-go/tool"
	"trpc.group/trpc-go/trpc-agent-go/tool/function"
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// replaceContentRequest represents the input for the replace content
// operation.
type replaceContentRequest struct {
	FileName string `json:"file_name" jsonschema:"description=Relative file path under base_directory to modify"`
	// OldString is replaced by NewString. It can be multi-line.
	OldString string `json:"old_string" jsonschema:"description=Existing text to replace; supports multi-line content"`
	// NewString is inserted in place of OldString. It can be multi-line.
	NewString string `json:"new_string" jsonschema:"description=Replacement text; supports multi-line content"`
	// NumReplacements limits replacements (default 1). Negative means all.
	NumReplacements int `json:"num_replacements,omitempty" jsonschema:"description=Optional replacement limit; 0 means 1 and negative means replace all matches"`
}

// replaceContentResponse represents the output from the replace content
// operation.
type replaceContentResponse struct {
	BaseDirectory string `json:"base_directory"`
	FileName      string `json:"file_name"`
	Message       string `json:"message"`
	MtimeMs       int64  `json:"mtime_ms,omitempty"`
}

// replaceContent performs the replace content operation.
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
		rsp.Message = fmt.Sprintf("Error: %v. %s", err, f.missingFileHint())
		return rsp, err
	}
	if !snap.Exists {
		rsp.Message = fmt.Sprintf(
			"Error: cannot access file '%s'. %s",
			req.FileName,
			f.missingFileHint(),
		)
		return rsp, fmt.Errorf("file not found: %s", req.FileName)
	}

	actual := textfile.FindActualString(snap.Content, req.OldString)
	if actual == "" {
		rsp.Message = fmt.Sprintf(
			"'%s' not found in '%s'",
			req.OldString,
			req.FileName,
		)
		return rsp, nil
	}
	totalCount := strings.Count(snap.Content, actual)
	numReplacements := req.NumReplacements
	if numReplacements == 0 {
		numReplacements = 1
	}
	if numReplacements < 0 || numReplacements > totalCount {
		numReplacements = totalCount
	}
	replacement := textfile.PreserveQuoteStyle(req.OldString, actual, req.NewString)
	newContent := strings.Replace(
		snap.Content,
		actual,
		replacement,
		numReplacements,
	)
	if err := f.commitEditSnapshot(ctx, snap, newContent); err != nil {
		rsp.Message = fmt.Sprintf("Error: writing file '%s': %v", req.FileName, err)
		return rsp, err
	}
	rsp.MtimeMs = snap.MtimeMs
	rsp.Message = fmt.Sprintf(
		"Successfully replaced %d of %d in '%s'",
		numReplacements,
		totalCount,
		req.FileName,
	)
	return rsp, nil
}

// replaceContentTool returns a callable tool for replacing content in a file.
func (f *fileToolSet) replaceContentTool() tool.CallableTool {
	return function.NewFunctionTool(
		f.replaceContent,
		function.WithName("replace_content"),
		function.WithDescription(
			"Replace a string in a file under base_directory. "+
				"Supports multi-line old_string/new_string.",
		),
	)
}
