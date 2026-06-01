package webresearch_test

import (
	"testing"

	"aranea-agents/internal/tools/webresearch"
)

func TestTruncateUTF8_maxZero(t *testing.T) {
	got := webresearch.TruncateUTF8("hello", 0)
	if got != "hello" {
		t.Fatalf("TruncateUTF8 = %q, want %q", got, "hello")
	}
}

func TestTruncateUTF8_emptyString(t *testing.T) {
	got := webresearch.TruncateUTF8("", 10)
	if got != "" {
		t.Fatalf("TruncateUTF8 = %q, want empty", got)
	}
}

func TestTruncateUTF8_pureASCII(t *testing.T) {
	got := webresearch.TruncateUTF8("hello world", 5)
	if len(got) > 5+20 {
		t.Fatalf("result too long: %q (len=%d)", got, len(got))
	}
	if got == "" {
		t.Fatal("result should not be empty")
	}
}

func TestTruncateUTF8_exactlyMax(t *testing.T) {
	s := "hello"
	got := webresearch.TruncateUTF8(s, len(s))
	if got != "hello" {
		t.Fatalf("TruncateUTF8 = %q, want %q when exactly at max", got, "hello")
	}
}

func TestTruncateUTF8_maxOne(t *testing.T) {
	got := webresearch.TruncateUTF8("abc", 1)
	if got != "a\n...[truncated]" {
		t.Fatalf("TruncateUTF8 = %q", got)
	}
}

func TestTruncateUTF8_multiByteTruncation(t *testing.T) {
	s := "你好世界"
	got := webresearch.TruncateUTF8(s, 3)
	if got == "" {
		t.Fatal("result should not be empty")
	}
	for _, r := range got {
		if r == 0xFFFD {
			t.Fatal("result contains replacement character, invalid UTF-8 boundary")
		}
	}
}

func TestTruncateUTF8_whitespaceTrimmed(t *testing.T) {
	got := webresearch.TruncateUTF8("  hello  ", 5)
	if got != "hello" {
		t.Fatalf("TruncateUTF8 = %q, want %q (trimmed)", got, "hello")
	}
}

func TestTruncateStr_shorterThanN(t *testing.T) {
	got := webresearch.TruncateStr("hi", 10)
	if got != "hi" {
		t.Fatalf("TruncateStr = %q, want %q", got, "hi")
	}
}

func TestTruncateStr_exactlyN(t *testing.T) {
	s := "hello"
	got := webresearch.TruncateStr(s, len(s))
	if got != "hello" {
		t.Fatalf("TruncateStr = %q, want %q", got, "hello")
	}
}

func TestTruncateStr_longerThanN(t *testing.T) {
	got := webresearch.TruncateStr("hello world", 5)
	if got != "hello..." {
		t.Fatalf("TruncateStr = %q, want hello...", got)
	}
}

func TestTruncateStr_empty(t *testing.T) {
	got := webresearch.TruncateStr("", 5)
	if got != "" {
		t.Fatalf("TruncateStr = %q, want empty", got)
	}
}

func TestFirstNonEmpty_emptyList(t *testing.T) {
	got := webresearch.FirstNonEmpty()
	if got != "" {
		t.Fatalf("FirstNonEmpty() = %q, want empty", got)
	}
}

func TestFirstNonEmpty_allEmpty(t *testing.T) {
	got := webresearch.FirstNonEmpty("", "  ", "")
	if got != "" {
		t.Fatalf("FirstNonEmpty = %q, want empty", got)
	}
}

func TestFirstNonEmpty_middleHasValue(t *testing.T) {
	got := webresearch.FirstNonEmpty("", "found", "also")
	if got != "found" {
		t.Fatalf("FirstNonEmpty = %q, want found", got)
	}
}

func TestFirstNonEmpty_firstHasValue(t *testing.T) {
	got := webresearch.FirstNonEmpty("first", "second")
	if got != "first" {
		t.Fatalf("FirstNonEmpty = %q, want first", got)
	}
}
