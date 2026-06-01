package modelregistry

import "testing"

func TestBuildCapabilityChips_ToolCall(t *testing.T) {
	m := Model{ToolCall: true}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "tool_call" {
		t.Fatalf("expected tool_call chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_Reasoning(t *testing.T) {
	m := Model{Reasoning: true}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "reasoning" {
		t.Fatalf("expected reasoning chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_Attachment(t *testing.T) {
	m := Model{Attachment: true}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "attachment" {
		t.Fatalf("expected attachment chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_StructuredOutput(t *testing.T) {
	tf := true
	m := Model{StructuredOutput: &tf}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "structured_output" {
		t.Fatalf("expected structured_output chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_StructuredOutputFalse(t *testing.T) {
	ff := false
	m := Model{StructuredOutput: &ff}
	chips := BuildCapabilityChips(m)
	found := false
	for _, c := range chips {
		if c.Key == "structured_output" {
			found = true
		}
	}
	if found {
		t.Fatal("should not include structured_output when false")
	}
}

func TestBuildCapabilityChips_Temperature(t *testing.T) {
	tf := true
	m := Model{Temperature: &tf}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "temperature" {
		t.Fatalf("expected temperature chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_OpenWeights(t *testing.T) {
	m := Model{OpenWeights: true}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "open_weights" {
		t.Fatalf("expected open_weights chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_Deprecated(t *testing.T) {
	m := Model{Status: "deprecated"}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "deprecated" {
		t.Fatalf("expected deprecated chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_Beta(t *testing.T) {
	m := Model{Status: "beta"}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "beta" {
		t.Fatalf("expected beta chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_Alpha(t *testing.T) {
	m := Model{Status: "alpha"}
	chips := BuildCapabilityChips(m)
	if len(chips) != 1 || chips[0].Key != "alpha" {
		t.Fatalf("expected alpha chip, got %v", chips)
	}
}

func TestBuildCapabilityChips_Vision(t *testing.T) {
	m := Model{Modalities: Modalities{Input: []string{"image", "text"}}}
	chips := BuildCapabilityChips(m)
	found := false
	for _, c := range chips {
		if c.Key == "vision" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected vision chip for image input modality")
	}
}

func TestBuildCapabilityChips_Video(t *testing.T) {
	m := Model{Modalities: Modalities{Input: []string{"video"}}}
	chips := BuildCapabilityChips(m)
	found := false
	for _, c := range chips {
		if c.Key == "vision" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected vision chip for video input modality")
	}
}

func TestBuildCapabilityChips_Multiple(t *testing.T) {
	tf := true
	m := Model{
		ToolCall:         true,
		Reasoning:        true,
		StructuredOutput: &tf,
		Status:           "beta",
		Modalities:       Modalities{Input: []string{"image"}},
	}
	chips := BuildCapabilityChips(m)
	if len(chips) < 5 {
		t.Fatalf("expected at least 5 chips, got %d: %v", len(chips), chips)
	}
}

func TestBuildCapabilityChips_Empty(t *testing.T) {
	m := Model{}
	chips := BuildCapabilityChips(m)
	if len(chips) != 0 {
		t.Fatalf("expected 0 chips for empty model, got %d", len(chips))
	}
}

func TestBuildCapabilityChips_Source(t *testing.T) {
	m := Model{ToolCall: true}
	chips := BuildCapabilityChips(m)
	if chips[0].Source != "catalog" {
		t.Fatalf("expected source=catalog, got %q", chips[0].Source)
	}
}
