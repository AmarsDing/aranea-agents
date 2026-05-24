package preview

import "testing"

func TestSanitizeStreamText_stripsThinkingTags(t *testing.T) {
	in := "hello<thinking>secret</thinking> world"
	out := SanitizeStreamText(in)
	if out != "hello world" {
		t.Fatalf("got %q", out)
	}
}

func TestSanitizeStreamText_emptyThinkingBlock(t *testing.T) {
	if SanitizeStreamText("<thinking></thinking>") != "" {
		t.Fatal("empty thinking block should vanish")
	}
	if SanitizeStreamText("  <think>  </think>  ") != "" {
		t.Fatal("empty think block should vanish")
	}
}

func TestSanitizeStreamText_malformedToolXML(t *testing.T) {
	in := "answer<tool_call name=\"x\">partial"
	out := SanitizeStreamText(in)
	if out != "answerpartial" {
		t.Fatalf("got %q", out)
	}
}

func TestSanitizeStreamText_redactedMarker(t *testing.T) {
	out := SanitizeStreamText("visible<think>hidden</think>tail")
	if out != "visibletail" {
		t.Fatalf("got %q", out)
	}
}
