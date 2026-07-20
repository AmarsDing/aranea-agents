//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//
//

package patch

import (
	"regexp"
	"strconv"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// hunkHeaderRE matches "@@ -oldStart[,oldLines] +newStart[,newLines] @@".
var hunkHeaderRE = regexp.MustCompile(
	`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`,
)

// ParseUnified parses unified diff text into hunks. File header lines
// (diff/index/---/+++/etc.) are skipped. Newlines are normalized to LF
// before parsing. A "\ No newline at end of file" marker is ignored.
func ParseUnified(text string) ([]Hunk, error) {
	normalized := textfile.NormalizeNewlines(strings.TrimSpace(text))
	if normalized == "" {
		return nil, nil
	}
	lines := strings.Split(normalized, "\n")
	var hunks []Hunk
	i := 0
	for i < len(lines) {
		line := lines[i]
		if !strings.HasPrefix(line, "@@") {
			// Skip file headers and anything outside a hunk.
			i++
			continue
		}
		h, err := parseHunkHeader(line, i+1)
		if err != nil {
			return nil, err
		}
		i++
		oldRemain := h.OldLines
		newRemain := h.NewLines
		for i < len(lines) && (oldRemain > 0 || newRemain > 0) {
			body := lines[i]
			if body == "" {
				// git emits truly empty lines for empty context lines.
				body = " "
			}
			switch body[0] {
			case ' ':
				oldRemain--
				newRemain--
			case '-':
				oldRemain--
			case '+':
				newRemain--
			case '\\':
				// "\ No newline at end of file": ignore.
				i++
				continue
			default:
				return nil, &ParseError{
					Line:    i + 1,
					Message: "unexpected hunk body line: " + body,
				}
			}
			h.Lines = append(h.Lines, body)
			i++
		}
		if oldRemain != 0 || newRemain != 0 {
			return nil, &ParseError{
				Line:    i,
				Message: "hunk body shorter than declared line counts",
			}
		}
		hunks = append(hunks, h)
	}
	return hunks, nil
}

func parseHunkHeader(line string, lineNo int) (Hunk, error) {
	m := hunkHeaderRE.FindStringSubmatch(line)
	if m == nil {
		return Hunk{}, &ParseError{
			Line:    lineNo,
			Message: "malformed hunk header: " + line,
		}
	}
	oldStart, _ := strconv.Atoi(m[1])
	oldLines := 1
	if m[2] != "" {
		oldLines, _ = strconv.Atoi(m[2])
	}
	newStart, _ := strconv.Atoi(m[3])
	newLines := 1
	if m[4] != "" {
		newLines, _ = strconv.Atoi(m[4])
	}
	return Hunk{
		OldStart: oldStart,
		OldLines: oldLines,
		NewStart: newStart,
		NewLines: newLines,
	}, nil
}
