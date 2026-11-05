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

import "fmt"

// Validate checks that hunk line counts are consistent with their
// bodies and that positions are non-negative.
func Validate(hunks []Hunk) error {
	for i, h := range hunks {
		if h.OldStart < 0 || h.NewStart < 0 ||
			h.OldLines < 0 || h.NewLines < 0 {
			return fmt.Errorf(
				"hunk %d: negative start or line count",
				i,
			)
		}
		var oldCount, newCount int
		for _, line := range h.Lines {
			if line == "" {
				return fmt.Errorf(
					"hunk %d: body line missing prefix",
					i,
				)
			}
			switch line[0] {
			case ' ':
				oldCount++
				newCount++
			case '-':
				oldCount++
			case '+':
				newCount++
			default:
				return fmt.Errorf(
					"hunk %d: invalid body line prefix %q",
					i,
					line[0],
				)
			}
		}
		if oldCount != h.OldLines || newCount != h.NewLines {
			return fmt.Errorf(
				"hunk %d: declared counts -%d,+%d do not match body -%d,+%d",
				i,
				h.OldLines,
				h.NewLines,
				oldCount,
				newCount,
			)
		}
	}
	return nil
}
