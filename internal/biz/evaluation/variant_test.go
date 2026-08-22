package evaluation

import (
	"context"
	"strings"
	"testing"
)

func TestDefaultVariantLabel(t *testing.T) {
	if got := DefaultVariantLabel("a1", "", "", ""); got != "a1" {
		t.Fatalf("agent only: %q", got)
	}
	if got := DefaultVariantLabel("a1", "gpt-4o", "", ""); got != "a1/gpt-4o" {
		t.Fatalf("with model: %q", got)
	}
	a := DefaultVariantLabel("a1", "gpt-4o", "be concise", "")
	b := DefaultVariantLabel("a1", "gpt-4o", "be concise", "")
	c := DefaultVariantLabel("a1", "gpt-4o", "be verbose", "")
	if a != b || a == c || a == "a1/gpt-4o" {
		t.Fatalf("prompt hash must be stable and distinct: %q %q %q", a, b, c)
	}
	if got := DefaultVariantLabel("a1", "", "", "none"); got != "a1/tnone" {
		t.Fatalf("tools cell: %q", got)
	}
}

func TestOverlayPrompt(t *testing.T) {
	if OverlayPrompt("", "hi") != "hi" {
		t.Fatal("empty overlay must keep user input")
	}
	got := OverlayPrompt("be terse", "list racks")
	if !strings.Contains(got, "be terse") || !strings.Contains(got, "list racks") {
		t.Fatalf("overlay missing parts: %q", got)
	}
}

func TestRunOverrideRoundTrip(t *testing.T) {
	ctx := WithRunOverride(context.Background(), RunOverride{Model: "m1", Prompt: "p1", Tools: "none"})
	ov, ok := RunOverrideFrom(ctx)
	if !ok || ov.Model != "m1" || ov.Prompt != "p1" || ov.Tools != "none" {
		t.Fatalf("override round-trip failed: ok=%v %#v", ok, ov)
	}
	if _, ok := RunOverrideFrom(context.Background()); ok {
		t.Fatal("empty ctx must have no override")
	}
}

func TestParseToolsOverride(t *testing.T) {
	none, allow := ParseToolsOverride("none")
	if !none || len(allow) != 0 {
		t.Fatalf("none: %v %#v", none, allow)
	}
	none, allow = ParseToolsOverride("knowledge_search, read_file, knowledge_search")
	if none || len(allow) != 2 || allow[0] != "knowledge_search" || allow[1] != "read_file" {
		t.Fatalf("allow: none=%v %#v", none, allow)
	}
}
