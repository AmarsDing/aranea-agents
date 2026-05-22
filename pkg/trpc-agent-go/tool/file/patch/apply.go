//
// Tencent is pleased to support the open source community by making
// trpc-agent-go available.
//
// Copyright (C) 2025 Tencent.  All rights reserved.
//
// trpc-agent-go is licensed under the Apache License Version 2.0.
//

package patch

import (
	"fmt"
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// HunkMismatchError describes a failed hunk validation.
type HunkMismatchError struct {
	HunkIndex     int
	ExpectedLines []string
	ActualLines   []string
}

func (e *HunkMismatchError) Error() string {
	return fmt.Sprintf("hunk %d does not match file content", e.HunkIndex)
}

// ApplyHunks applies hunks in order to content.
func ApplyHunks(content string, hunks []Hunk) (string, error) {
	if len(hunks) == 0 {
		return content, nil
	}
	lines := textfile.SplitLines(content)
	trailingNL := strings.HasSuffix(content, "\n")
	offset := 0
	for hi, hunk := range hunks {
		start := hunk.OldStart - 1 + offset
		if hunk.OldStart <= 0 {
			start = 0
		}
		pos := start
		newSeg := make([]string, 0, len(hunk.Lines))
		for _, raw := range hunk.Lines {
			if raw == "" {
				continue
			}
			kind := raw[0]
			line := raw[1:]
			switch kind {
			case ' ':
				if pos >= len(lines) || lines[pos] != line {
					return "", &HunkMismatchError{
						HunkIndex:     hi,
						ExpectedLines: []string{" " + line},
						ActualLines:   lineAt(lines, pos, ' '),
					}
				}
				newSeg = append(newSeg, line)
				pos++
			case '-':
				if pos >= len(lines) || lines[pos] != line {
					return "", &HunkMismatchError{
						HunkIndex:     hi,
						ExpectedLines: []string{"-" + line},
						ActualLines:   lineAt(lines, pos, '-'),
					}
				}
				pos++
			case '+':
				newSeg = append(newSeg, line)
			default:
				return "", fmt.Errorf("hunk %d: invalid line prefix %q", hi, kind)
			}
		}
		oldLen := pos - start
		lines = append(lines[:start], append(newSeg, lines[pos:]...)...)
		offset += len(newSeg) - oldLen
	}
	out := strings.Join(lines, "\n")
	if trailingNL && len(lines) > 0 {
		out += "\n"
	}
	return out, nil
}

func lineAt(lines []string, idx int, prefix byte) []string {
	if idx < 0 || idx >= len(lines) {
		return nil
	}
	return []string{string(prefix) + lines[idx]}
}
