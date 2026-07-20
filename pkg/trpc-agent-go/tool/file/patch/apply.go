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
	"strings"

	"trpc.group/trpc-go/trpc-agent-go/tool/internal/textfile"
)

// Apply applies hunks to content in memory and returns the updated
// content. Hunks are matched against original-file coordinates; when
// the declared position does not match, a unique match elsewhere is
// accepted (line-number drift tolerance). On mismatch a MismatchError
// is returned and the content is left untouched.
func Apply(content string, hunks []Hunk) (string, error) {
	if len(hunks) == 0 {
		return content, nil
	}
	trailingNewline := strings.HasSuffix(content, "\n")
	lines := textfile.SplitTextLines(content)

	out := make([]string, 0, len(lines))
	cursor := 0
	for idx, h := range hunks {
		oldSeq, newSeq := sequences(h)
		pos := h.OldStart - 1
		if pos < cursor {
			pos = cursor
		}
		matchPos, ok := locate(lines, oldSeq, pos, cursor)
		if !ok {
			return "", &MismatchError{
				HunkIndex: idx,
				Expected:  oldSeq,
				Actual:    window(lines, pos, len(oldSeq)),
			}
		}
		out = append(out, lines[cursor:matchPos]...)
		out = append(out, newSeq...)
		cursor = matchPos + len(oldSeq)
	}
	out = append(out, lines[cursor:]...)

	result := strings.Join(out, "\n")
	if trailingNewline && result != "" {
		result += "\n"
	}
	return result, nil
}

// sequences splits hunk body lines into the old-side sequence
// (context + deletions) and the new-side sequence (context +
// additions), prefixes stripped.
func sequences(h Hunk) (oldSeq []string, newSeq []string) {
	for _, line := range h.Lines {
		if line == "" {
			continue
		}
		switch line[0] {
		case ' ':
			oldSeq = append(oldSeq, line[1:])
			newSeq = append(newSeq, line[1:])
		case '-':
			oldSeq = append(oldSeq, line[1:])
		case '+':
			newSeq = append(newSeq, line[1:])
		}
	}
	return oldSeq, newSeq
}

// locate finds where oldSeq matches in lines. The declared position
// wins when it matches; otherwise a unique match at or after cursor
// is accepted. Returns (position, true) or (0, false).
func locate(
	lines []string,
	oldSeq []string,
	pos int,
	cursor int,
) (int, bool) {
	if len(oldSeq) == 0 {
		if pos > len(lines) {
			pos = len(lines)
		}
		return pos, true
	}
	if matchAt(lines, oldSeq, pos) {
		return pos, true
	}
	found := -1
	for i := cursor; i+len(oldSeq) <= len(lines); i++ {
		if !matchAt(lines, oldSeq, i) {
			continue
		}
		if found >= 0 {
			// Ambiguous: more than one candidate.
			return 0, false
		}
		found = i
	}
	if found < 0 {
		return 0, false
	}
	return found, true
}

func matchAt(lines []string, oldSeq []string, pos int) bool {
	if pos < 0 || pos+len(oldSeq) > len(lines) {
		return false
	}
	for j := range oldSeq {
		if lines[pos+j] != oldSeq[j] {
			return false
		}
	}
	return true
}

// window returns up to n lines from pos for error reporting.
func window(lines []string, pos int, n int) []string {
	if pos < 0 {
		pos = 0
	}
	if pos > len(lines) {
		pos = len(lines)
	}
	end := pos + n
	if end > len(lines) {
		end = len(lines)
	}
	out := make([]string, 0, end-pos)
	return append(out, lines[pos:end]...)
}
