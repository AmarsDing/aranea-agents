package biz

import (
	"strings"
	"testing"
)

func TestStripFactMarks_RemovesCompleteTags(t *testing.T) {
	in := "你好，张三！很高兴认识你。\n\n" +
		`<fact type="identity" confidence="high">The user's name is 张三</fact>` + "\n" +
		`<fact type="preference" confidence="high">User likes coffee</fact>`
	got := StripFactMarks(in)
	if strings.Contains(got, "<fact") || strings.Contains(got, "</fact>") {
		t.Fatalf("expected no fact tags, got %q", got)
	}
	if !strings.Contains(got, "你好，张三！很高兴认识你。") {
		t.Fatalf("expected visible text preserved, got %q", got)
	}
}

func TestStripFactMarks_NoTags_Unchanged(t *testing.T) {
	if got := StripFactMarks("plain text"); got != "plain text" {
		t.Fatalf("expected unchanged, got %q", got)
	}
}

func TestStripFactMarks_UnclosedTag_Preserved(t *testing.T) {
	// Regex requires a closing tag; an unclosed tag (e.g. truncated stream) is
	// left untouched rather than eating the visible text after it.
	in := `answer <fact type="x" confidence="high">oops`
	if got := StripFactMarks(in); got != in {
		t.Fatalf("expected unclosed tag preserved, got %q", got)
	}
}
