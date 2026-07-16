//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

// Package patch applies unified diffs and structured hunks to text content.
package patch

import (
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// Hunk is a structured unified-diff hunk.
type Hunk struct {
	OldStart int      `json:"old_start"`
	OldLines int      `json:"old_lines"`
	NewStart int      `json:"new_start"`
	NewLines int      `json:"new_lines"`
	Lines    []string `json:"lines"`
}

// MismatchError describes a hunk that does not match the current file.
type MismatchError struct {
	HunkIndex     int
	ExpectedLines []string
	ActualLines   []string
}

func (e *MismatchError) Error() string {
	return fmt.Sprintf(
		"hunk_mismatch at hunk_index=%d",
		e.HunkIndex,
	)
}

// ParseUnified parses a unified diff body into hunks.
// Accepts optional ---/+++ headers; only @@ hunks are required.
func ParseUnified(patchText string) ([]Hunk, error) {
	patchText = textfile.NormalizeNewlines(patchText)
	lines := strings.Split(patchText, "\n")
	var hunks []Hunk
	var cur *Hunk
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			oldStart, oldLines, newStart, newLines, err := parseHunkHeader(line)
			if err != nil {
				return nil, err
			}
			hunks = append(hunks, Hunk{
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
			})
			cur = &hunks[len(hunks)-1]
			continue
		}
		if cur == nil {
			continue
		}
		if line == "" && cur == &hunks[len(hunks)-1] {
			// trailing empty from Split — ignore only if no body yet and end
			continue
		}
		if len(line) == 0 {
			cur.Lines = append(cur.Lines, " ")
			continue
		}
		switch line[0] {
		case ' ', '-', '+', '\\':
			cur.Lines = append(cur.Lines, line)
		default:
			// ignore file headers and noise outside hunk body markers
		}
	}
	if len(hunks) == 0 {
		return nil, fmt.Errorf("no hunks found in patch")
	}
	return hunks, nil
}

func parseHunkHeader(line string) (oldStart, oldLines, newStart, newLines int, err error) {
	// @@ -l,s +l,s @@
	line = strings.TrimSpace(line)
	if !strings.HasPrefix(line, "@@") {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header: %s", line)
	}
	rest := strings.TrimPrefix(line, "@@")
	rest = strings.TrimSpace(rest)
	parts := strings.Split(rest, "@@")
	if len(parts) == 0 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header: %s", line)
	}
	rangePart := strings.TrimSpace(parts[0])
	fields := strings.Fields(rangePart)
	if len(fields) < 2 {
		return 0, 0, 0, 0, fmt.Errorf("invalid hunk header ranges: %s", line)
	}
	oldStart, oldLines, err = parseRange(fields[0], '-')
	if err != nil {
		return 0, 0, 0, 0, err
	}
	newStart, newLines, err = parseRange(fields[1], '+')
	if err != nil {
		return 0, 0, 0, 0, err
	}
	return oldStart, oldLines, newStart, newLines, nil
}

func parseRange(field string, sign byte) (start, count int, err error) {
	field = strings.TrimSpace(field)
	if len(field) == 0 || field[0] != sign {
		return 0, 0, fmt.Errorf("invalid range %q", field)
	}
	field = field[1:]
	if i := strings.IndexByte(field, ','); i >= 0 {
		if _, err := fmt.Sscanf(field, "%d,%d", &start, &count); err != nil {
			return 0, 0, fmt.Errorf("invalid range %q: %w", field, err)
		}
		return start, count, nil
	}
	if _, err := fmt.Sscanf(field, "%d", &start); err != nil {
		return 0, 0, fmt.Errorf("invalid range %q: %w", field, err)
	}
	return start, 1, nil
}

// Apply applies hunks to content (LF-normalized). Returns updated content.
func Apply(content string, hunks []Hunk) (string, error) {
	if len(hunks) == 0 {
		return content, nil
	}
	lines := textfile.SplitLines(content)
	hadTrailingNL := strings.HasSuffix(content, "\n")
	// Apply from last hunk to first so line offsets stay valid when OldStart is used.
	for i := len(hunks) - 1; i >= 0; i-- {
		var err error
		lines, err = applyHunk(lines, hunks[i], i)
		if err != nil {
			return "", err
		}
	}
	out := strings.Join(lines, "\n")
	if hadTrailingNL || len(lines) > 0 {
		// Preserve trailing newline when original had one or result non-empty with NL convention.
		if hadTrailingNL {
			out += "\n"
		}
	}
	return out, nil
}

func applyHunk(lines []string, h Hunk, hunkIndex int) ([]string, error) {
	startIdx := h.OldStart - 1
	if h.OldStart == 0 {
		startIdx = 0
	}
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx > len(lines) {
		return nil, &MismatchError{
			HunkIndex:     hunkIndex,
			ExpectedLines: collectExpected(h),
			ActualLines:   nil,
		}
	}

	var result []string
	result = append(result, lines[:startIdx]...)
	pos := startIdx
	for _, raw := range h.Lines {
		if strings.HasPrefix(raw, "\\") {
			continue // "\ No newline at end of file"
		}
		if len(raw) == 0 {
			raw = " "
		}
		prefix := raw[0]
		body := raw[1:]
		switch prefix {
		case ' ', '-':
			if pos >= len(lines) || lines[pos] != body {
				actual := []string{}
				if pos < len(lines) {
					actual = []string{"-" + lines[pos]}
				}
				return nil, &MismatchError{
					HunkIndex:     hunkIndex,
					ExpectedLines: []string{raw},
					ActualLines:   actual,
				}
			}
			if prefix == ' ' {
				result = append(result, body)
			}
			pos++
		case '+':
			result = append(result, body)
		default:
			return nil, fmt.Errorf("invalid hunk line prefix in hunk %d: %q", hunkIndex, raw)
		}
	}
	result = append(result, lines[pos:]...)
	return result, nil
}

func collectExpected(h Hunk) []string {
	out := make([]string, 0, len(h.Lines))
	for _, l := range h.Lines {
		if strings.HasPrefix(l, "-") || strings.HasPrefix(l, " ") {
			out = append(out, l)
		}
	}
	return out
}
