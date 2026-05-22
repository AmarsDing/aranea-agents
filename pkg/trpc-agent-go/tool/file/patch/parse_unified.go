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
	"regexp"
	"strconv"
	"strings"
)

var hunkHeaderRE = regexp.MustCompile(`^@@\s+-(\d+)(?:,(\d+))?\s+\+(\d+)(?:,(\d+))?\s+@@`)

// ParseUnified parses a single-file unified diff body into hunks.
func ParseUnified(patch string) ([]Hunk, error) {
	patch = strings.ReplaceAll(patch, "\r\n", "\n")
	patch = strings.TrimSpace(patch)
	if patch == "" {
		return nil, fmt.Errorf("patch is empty")
	}
	lines := strings.Split(patch, "\n")
	var hunks []Hunk
	var current *Hunk
	flush := func() error {
		if current == nil {
			return nil
		}
		if len(current.Lines) == 0 {
			return fmt.Errorf("hunk at old_start %d has no lines", current.OldStart)
		}
		hunks = append(hunks, *current)
		current = nil
		return nil
	}
	for _, line := range lines {
		if strings.HasPrefix(line, "@@") {
			if err := flush(); err != nil {
				return nil, err
			}
			m := hunkHeaderRE.FindStringSubmatch(line)
			if m == nil {
				return nil, fmt.Errorf("invalid hunk header: %s", line)
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
			current = &Hunk{
				OldStart: oldStart,
				OldLines: oldLines,
				NewStart: newStart,
				NewLines: newLines,
				Lines:    make([]string, 0),
			}
			continue
		}
		if current == nil {
			if strings.HasPrefix(line, "---") || strings.HasPrefix(line, "+++") {
				continue
			}
			return nil, fmt.Errorf("expected hunk header, got: %s", line)
		}
		if line == "" {
			current.Lines = append(current.Lines, " ")
			continue
		}
		if line[0] != ' ' && line[0] != '-' && line[0] != '+' {
			return nil, fmt.Errorf("invalid hunk line: %s", line)
		}
		current.Lines = append(current.Lines, line)
	}
	if err := flush(); err != nil {
		return nil, err
	}
	if len(hunks) == 0 {
		return nil, fmt.Errorf("no hunks found in patch")
	}
	return hunks, nil
}
