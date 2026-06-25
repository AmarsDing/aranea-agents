package agent

import (
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestChoiceStreamContent_partialPreservesLeadingSpace(t *testing.T) {
	choice := trpcmodel.Choice{
		Delta: trpcmodel.Message{Content: " world"},
	}
	text, _ := ChoiceStreamContent(choice, true)
	if text != " world" {
		t.Fatalf("partial delta text = %q, want leading space preserved", text)
	}
}

func TestChoiceStreamContent_nonPartialTrims(t *testing.T) {
	choice := trpcmodel.Choice{
		Message: trpcmodel.Message{Content: "  hello  "},
	}
	text, _ := ChoiceStreamContent(choice, false)
	if text != "hello" {
		t.Fatalf("non-partial text = %q, want trimmed hello", text)
	}
}

func TestChoiceStreamContent_partialReasoningDelta(t *testing.T) {
	choice := trpcmodel.Choice{
		Delta: trpcmodel.Message{ReasoningContent: "think"},
	}
	_, reasoning := ChoiceStreamContent(choice, true)
	if reasoning != "think" {
		t.Fatalf("reasoning = %q", reasoning)
	}
}

func TestChoiceStreamContent_partialFallsBackToContentParts(t *testing.T) {
	textPart := "hello from parts"
	choice := trpcmodel.Choice{
		Delta: trpcmodel.Message{
			ContentParts: []trpcmodel.ContentPart{
				{Type: "text", Text: &textPart},
			},
		},
	}
	text, _ := ChoiceStreamContent(choice, true)
	if text != textPart {
		t.Fatalf("partial text from content_parts = %q, want %q", text, textPart)
	}
}

func TestChoiceStreamContent_nonPartialFallsBackToContentParts(t *testing.T) {
	textPart := "final from parts"
	choice := trpcmodel.Choice{
		Message: trpcmodel.Message{
			ContentParts: []trpcmodel.ContentPart{
				{Type: "text", Text: &textPart},
			},
		},
	}
	text, _ := ChoiceStreamContent(choice, false)
	if text != textPart {
		t.Fatalf("non-partial text from content_parts = %q, want %q", text, textPart)
	}
}
