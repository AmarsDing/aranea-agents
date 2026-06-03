package agent

import (
	"testing"

	"aranea-agents/internal/event"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
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

func TestEventProjector_partialDeltaPreservesSpace(t *testing.T) {
	bus := event.NewBus()
	p := NewEventProjector(bus, nil)
	meta := ProjectMeta{SessionID: "sess-1"}

	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{
				{Delta: trpcmodel.Message{Content: " x"}},
			},
		},
	}

	envelopes := p.Project(t.Context(), ev, meta)
	if len(envelopes) != 1 {
		t.Fatalf("expected 1 envelope, got %d", len(envelopes))
	}
	if envelopes[0].Content == nil || envelopes[0].Content.Text != " x" {
		t.Fatalf("text delta = %+v", envelopes[0].Content)
	}
}
