package compress

import (
	"strings"
	"testing"
)

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

func TestExtractMemoryFactsSchema_IncludesConstraintSubjectType(t *testing.T) {
	params, ok := ExtractMemoryFactsFunctionSchema["parameters"].(map[string]any)
	if !ok {
		t.Fatal("schema parameters missing")
	}
	props, ok := params["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema properties missing")
	}
	facts, ok := props["facts"].(map[string]any)
	if !ok {
		t.Fatal("schema facts missing")
	}
	items, ok := facts["items"].(map[string]any)
	if !ok {
		t.Fatal("schema facts.items missing")
	}
	itemProps, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema facts.items.properties missing")
	}
	st, ok := itemProps["subject_type"].(map[string]any)
	if !ok {
		t.Fatal("schema subject_type missing")
	}
	enumVals, ok := st["enum"].([]string)
	if !ok {
		t.Fatalf("subject_type enum unexpected type: %T", st["enum"])
	}
	found := false
	for _, v := range enumVals {
		if v == "constraint" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("subject_type enum missing constraint: %v", enumVals)
	}
}

func TestMemoryExtractSystemPromptV2_DocumentsConstraint(t *testing.T) {
	if !strings.Contains(MemoryExtractSystemPromptV2, "constraint") {
		t.Fatal("MemoryExtractSystemPromptV2 should document constraint subject_type")
	}
}
