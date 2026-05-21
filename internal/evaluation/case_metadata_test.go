package evaluation

import "testing"

func TestHasScriptedSimulation(t *testing.T) {
	meta := CaseMetadata{UserSimulation: &UserSimMetadata{Script: []string{"hi", "bye"}}}
	if !meta.HasScriptedSimulation() {
		t.Fatal("expected scripted simulation")
	}
	if meta.HasLLMSimulation() {
		t.Fatal("script should not count as LLM sim")
	}
}

func TestHasLLMSimulation(t *testing.T) {
	meta := CaseMetadata{UserSimulation: &UserSimMetadata{UseLLM: true}}
	if !meta.HasLLMSimulation() {
		t.Fatal("expected LLM simulation")
	}
	planOnly := CaseMetadata{UserSimulation: &UserSimMetadata{ConversationPlan: "reach goal X"}}
	if !planOnly.HasLLMSimulation() {
		t.Fatal("conversation plan without script implies LLM sim")
	}
}

func TestExpectedToolEntries(t *testing.T) {
	meta := CaseMetadata{ExpectedTools: []string{"search", "calc"}}
	entries := meta.expectedToolEntries()
	if len(entries) != 2 || entries[0].Name != "search" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}
