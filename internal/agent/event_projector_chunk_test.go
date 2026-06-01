package agent

import (
	"context"
	"testing"

	"aranea-agents/internal/event"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestProjectChatCompletionChunkToolAndTextSameChunk(t *testing.T) {
	bus := event.NewBus()
	p := NewEventProjector(bus, nil)
	meta := ProjectMeta{SessionID: "sess-1"}

	ev := &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{
				{
					Delta: trpcmodel.Message{
						Content: "partial reply",
						ToolCalls: []trpcmodel.ToolCall{
							{ID: "tc-1", Function: trpcmodel.FunctionDefinitionParam{Name: "read_file", Arguments: []byte(`{"path":"a.go"}`)}},
						},
					},
				},
			},
		},
	}

	envelopes := p.Project(context.Background(), ev, meta)
	if len(envelopes) < 2 {
		t.Fatalf("expected tool_call + text_delta, got %d envelopes", len(envelopes))
	}
	var hasTool, hasText bool
	for _, env := range envelopes {
		switch env.Type {
		case event.EnvelopeTypeToolCall:
			hasTool = true
		case event.EnvelopeTypeTextDelta:
			if env.Content != nil && env.Content.Text != "" {
				hasText = true
			}
		}
	}
	if !hasTool || !hasText {
		t.Fatalf("hasTool=%v hasText=%v types=%v", hasTool, hasText, envelopeTypes(envelopes))
	}
}

func envelopeTypes(envs []event.Envelope) []event.EnvelopeType {
	out := make([]event.EnvelopeType, 0, len(envs))
	for _, e := range envs {
		out = append(out, e.Type)
	}
	return out
}
