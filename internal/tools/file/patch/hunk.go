//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package patch provides unified-diff hunk parsing, validation, and
// in-memory application for the file editing tools.
package patch

import "fmt"

// Hunk is one section of a unified diff. Lines carries the body lines
// each prefixed with ' ' (context), '-' (deletion), or '+' (addition).
type Hunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}

// MismatchError reports that a hunk could not be applied to the
// target content. Expected holds the old-side lines the hunk wanted
// to match; Actual holds the lines found at the declared position.
type MismatchError struct {
	HunkIndex int
	Expected  []string
	Actual    []string
}

// Error implements the error interface.
func (e *MismatchError) Error() string {
	return fmt.Sprintf(
		"hunk %d does not apply: expected lines not found at declared position",
		e.HunkIndex,
	)
}

// ParseError reports malformed unified diff input.
type ParseError struct {
	Line    int
	Message string
}

// Error implements the error interface.
func (e *ParseError) Error() string {
	return fmt.Sprintf("invalid unified diff at line %d: %s", e.Line, e.Message)
}
