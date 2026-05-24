package compress

import "testing"

func TestParseMemoryExtractJSON(t *testing.T) {
	raw := "```json\n{\"facts\":[{\"statement\":\"User prefers dark mode\",\"topics\":[\"preference\"]}]}\n```"
	facts, err := ParseMemoryExtractJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].Statement != "User prefers dark mode" {
		t.Fatalf("unexpected facts: %+v", facts)
	}
}

func TestParseMemoryExtractJSON_SkipsEmptyStatements(t *testing.T) {
	facts, err := ParseMemoryExtractJSON(`{"facts":[{"statement":""},{"statement":"ok"}]}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 || facts[0].Statement != "ok" {
		t.Fatalf("unexpected facts: %+v", facts)
	}
}

func TestBuildMemoryExtractTranscript(t *testing.T) {
	got := BuildMemoryExtractTranscript([]struct{ Role, Content string }{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi"},
		{Role: "tool", Content: "ignored"},
	})
	if got != "USER: hello\nASSISTANT: hi" {
		t.Fatalf("transcript=%q", got)
	}
}
