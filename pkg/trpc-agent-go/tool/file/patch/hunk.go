//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

// Package patch applies structured hunks and semantic edits to text content.
package patch

import (
	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// Hunk is a unified-diff style change block.
type Hunk struct {
	OldStart int      `json:"oldStart"`
	OldLines int      `json:"oldLines"`
	NewStart int      `json:"newStart"`
	NewLines int      `json:"newLines"`
	Lines    []string `json:"lines"`
}

// BuildStructured computes a minimal hunk diff between old and new content.
func BuildStructured(oldContent string, newContent string) []Hunk {
	if oldContent == newContent {
		return nil
	}
	oldLines := textfile.SplitLines(oldContent)
	newLines := textfile.SplitLines(newContent)
	prefix := 0
	for prefix < len(oldLines) && prefix < len(newLines) && oldLines[prefix] == newLines[prefix] {
		prefix++
	}
	oldSuffixLimit := len(oldLines) - prefix
	newSuffixLimit := len(newLines) - prefix
	suffix := 0
	for suffix < oldSuffixLimit && suffix < newSuffixLimit {
		if oldLines[len(oldLines)-1-suffix] != newLines[len(newLines)-1-suffix] {
			break
		}
		suffix++
	}
	oldMid := oldLines[prefix : len(oldLines)-suffix]
	newMid := newLines[prefix : len(newLines)-suffix]
	lines := make([]string, 0, len(oldMid)+len(newMid))
	for _, line := range oldMid {
		lines = append(lines, "-"+line)
	}
	for _, line := range newMid {
		lines = append(lines, "+"+line)
	}
	oldStart := prefix + 1
	newStart := prefix + 1
	if len(oldLines) == 0 {
		oldStart = 0
	}
	if len(newLines) == 0 {
		newStart = 0
	}
	return []Hunk{{
		OldStart: oldStart,
		OldLines: len(oldMid),
		NewStart: newStart,
		NewLines: len(newMid),
		Lines:    lines,
	}}
}
