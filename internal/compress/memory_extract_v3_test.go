package compress

import (
	"strings"
	"testing"
)

// V3 (P1-3): whitelist vocabulary + confidence floor guidance.

func TestMemoryExtractPromptV3Version(t *testing.T) {
	if MemoryExtractPromptV3Version != "v3" {
		t.Fatalf("MemoryExtractPromptV3Version: got %q want v3", MemoryExtractPromptV3Version)
	}
}

func TestExtractMemoryFactsSchemaV3_WhitelistEnum(t *testing.T) {
	params, ok := ExtractMemoryFactsFunctionSchemaV3["parameters"].(map[string]any)
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
	got := map[string]bool{}
	for _, v := range enumVals {
		got[v] = true
	}
	for _, want := range []string{"person", "preference", "constraint", "goal", "decision", "relationship", "other"} {
		if !got[want] {
			t.Errorf("subject_type enum missing %q: %v", want, enumVals)
		}
	}
	// Whitelist-dropped kinds must no longer be offered (P1-3 gate ①).
	for _, dropped := range []string{"event", "concept"} {
		if got[dropped] {
			t.Errorf("subject_type enum must not offer whitelist-dropped kind %q", dropped)
		}
	}
}

func TestMemoryExtractSystemPromptV3_WhitelistGuidance(t *testing.T) {
	for _, kw := range []string{"goal", "decision", "relationship", "confidence"} {
		if !strings.Contains(MemoryExtractSystemPromptV3, kw) {
			t.Errorf("MemoryExtractSystemPromptV3 should document %q", kw)
		}
	}
}

// --- Adjudication prompt/parse (P1-3 operation semantics) ---

func TestBuildFactAdjudicationPrompt(t *testing.T) {
	items := []FactAdjudicationPromptItem{
		{
			Statement: "用户现在只喝茶",
			Kind:      "preference",
			Neighbors: []FactAdjudicationPromptNeighbor{
				{ID: "f1", Statement: "用户喜欢咖啡", Kind: "preference"},
			},
		},
	}
	prompt := BuildFactAdjudicationPrompt(items)
	for _, want := range []string{"用户现在只喝茶", "f1", "用户喜欢咖啡"} {
		if !strings.Contains(prompt, want) {
			t.Errorf("adjudication prompt missing %q", want)
		}
	}
}

func TestParseFactAdjudicationResponse_Valid(t *testing.T) {
	raw := `{"verdicts":[{"statement":"用户现在只喝茶","operation":"update","target_id":"f1"},{"statement":"x","operation":"add"}]}`
	verdicts, err := ParseFactAdjudicationResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(verdicts) != 2 {
		t.Fatalf("verdicts: got %d want 2", len(verdicts))
	}
	if verdicts[0].Operation != "update" || verdicts[0].TargetID != "f1" {
		t.Fatalf("verdict[0]: %+v", verdicts[0])
	}
	if verdicts[1].Operation != "add" || verdicts[1].TargetID != "" {
		t.Fatalf("verdict[1]: %+v", verdicts[1])
	}
}

func TestParseFactAdjudicationResponse_InvalidOperationDropped(t *testing.T) {
	raw := `{"verdicts":[{"statement":"a","operation":"explode","target_id":"f1"},{"statement":"b","operation":"noop"}]}`
	verdicts, err := ParseFactAdjudicationResponse(raw)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(verdicts) != 1 || verdicts[0].Statement != "b" {
		t.Fatalf("invalid operation must be dropped: %+v", verdicts)
	}
}

func TestParseFactAdjudicationResponse_Fenced(t *testing.T) {
	raw := "```json\n{\"verdicts\":[{\"statement\":\"a\",\"operation\":\"noop\"}]}\n```"
	verdicts, err := ParseFactAdjudicationResponse(raw)
	if err != nil || len(verdicts) != 1 {
		t.Fatalf("fenced parse: %v %+v", err, verdicts)
	}
}

func TestParseFactAdjudicationResponse_Empty(t *testing.T) {
	if v, err := ParseFactAdjudicationResponse(""); err != nil || len(v) != 0 {
		t.Fatalf("empty: %v %+v", err, v)
	}
}
