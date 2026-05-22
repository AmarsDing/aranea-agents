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

// Edit is one search/replace operation.
type Edit struct {
	Search     string
	Replace    string
	ReplaceAll bool
}

// EditNotUniqueError is returned when search matches more than once.
type EditNotUniqueError struct {
	EditIndex  int
	MatchCount int
	MatchLines []int
	Search     string
}

func (e *EditNotUniqueError) Error() string {
	return fmt.Sprintf(
		"edit %d: found %d matches; provide more context or set replace_all",
		e.EditIndex,
		e.MatchCount,
	)
}

// EditNotFoundError is returned when search text is missing.
type EditNotFoundError struct {
	EditIndex int
	Search    string
}

func (e *EditNotFoundError) Error() string {
	return fmt.Sprintf("edit %d: search text not found", e.EditIndex)
}

// ApplyEdits applies edits sequentially to content.
// The second return value is total string replacements; the third is edits
// that actually changed content (search != replace).
func ApplyEdits(content string, edits []Edit) (string, int, int, error) {
	current := content
	total := 0
	applied := 0
	for i, edit := range edits {
		if edit.Search == edit.Replace {
			continue
		}
		applied++
		actual := textfile.FindActualString(current, edit.Search)
		if actual == "" {
			return "", 0, 0, &EditNotFoundError{EditIndex: i, Search: edit.Search}
		}
		actualNew := textfile.PreserveQuoteStyle(edit.Search, actual, edit.Replace)
		count := strings.Count(current, actual)
		if count > 1 && !edit.ReplaceAll {
			return "", 0, 0, &EditNotUniqueError{
				EditIndex:  i,
				MatchCount: count,
				MatchLines: matchLineNumbers(current, actual),
				Search:     edit.Search,
			}
		}
		replacements := 1
		if edit.ReplaceAll {
			replacements = -1
		}
		current = strings.Replace(current, actual, actualNew, replacements)
		if edit.ReplaceAll {
			total += count
		} else {
			total++
		}
	}
	return current, total, applied, nil
}

func matchLineNumbers(content string, needle string) []int {
	lines := textfile.SplitLines(content)
	var out []int
	for i, line := range lines {
		if strings.Contains(line, needle) {
			out = append(out, i+1)
		}
	}
	if len(out) > 0 {
		return out
	}
	// multi-line needle: approximate by scanning whole content
	idx := 0
	lineNum := 1
	for {
		pos := strings.Index(content[idx:], needle)
		if pos < 0 {
			break
		}
		abs := idx + pos
		lineNum = 1 + strings.Count(content[:abs], "\n")
		out = append(out, lineNum)
		idx = abs + len(needle)
	}
	return out
}
