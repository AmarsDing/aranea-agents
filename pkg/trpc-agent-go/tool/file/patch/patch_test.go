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
	"testing"
)

func TestApplySimpleHunk(t *testing.T) {
	content := "a\nb\nc\n"
	hunks := []Hunk{{
		OldStart: 2,
		OldLines: 1,
		NewStart: 2,
		NewLines: 1,
		Lines:    []string{"-b", "+B"},
	}}
	out, err := Apply(content, hunks)
	if err != nil {
		t.Fatal(err)
	}
	if out != "a\nB\nc\n" {
		t.Fatalf("got %q", out)
	}
}

func TestApplyMismatch(t *testing.T) {
	content := "a\nb\nc\n"
	hunks := []Hunk{{
		OldStart: 2,
		OldLines: 1,
		NewStart: 2,
		NewLines: 1,
		Lines:    []string{"-x", "+y"},
	}}
	_, err := Apply(content, hunks)
	if err == nil {
		t.Fatal("expected mismatch")
	}
	if _, ok := err.(*MismatchError); !ok {
		t.Fatalf("want MismatchError, got %T %v", err, err)
	}
}

func TestParseUnified(t *testing.T) {
	raw := "--- a/f\n+++ b/f\n@@ -1,3 +1,3 @@\n a\n-b\n+B\n c\n"
	hunks, err := ParseUnified(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(hunks) != 1 {
		t.Fatalf("hunks=%d", len(hunks))
	}
	out, err := Apply("a\nb\nc\n", hunks)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "B") {
		t.Fatalf("got %q", out)
	}
}
