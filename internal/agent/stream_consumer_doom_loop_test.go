package agent

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func doomLoopDeltaEvent(text string) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices: []trpcmodel.Choice{
				{Delta: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: text}},
			},
		},
	}
}

func TestStreamConsumer_DoomLoopAbortsTurn(t *testing.T) {
	events := make(chan *trpcevent.Event, 16)
	repetitive := "I need to check the file and verify the output."
	// 6 identical substantial deltas ≥ threshold (5) must trigger abort.
	for i := 0; i < 6; i++ {
		events <- doomLoopDeltaEvent(repetitive)
	}
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID:    "sess-doom",
		RequestID:    "task-doom",
		InvocationID: "turn-doom",
	}, nil, loggateway.NewNoop())

	if !result.DoomLoopDetected {
		t.Fatal("expected DoomLoopDetected=true after repetitive deltas")
	}
	if !result.HasError {
		t.Fatal("expected HasError=true when doom loop aborts the turn")
	}
}

func TestStreamConsumer_DoomLoopNoFalsePositive(t *testing.T) {
	events := make(chan *trpcevent.Event, 16)
	distinct := []string{
		"First, I will read the configuration file carefully.",
		"Now I am writing the brand new handler function here.",
		"Next step: run all the unit tests to verify behavior.",
		"The tests all pass, so I will commit the changes now.",
		"Finally, update the user documentation accordingly too.",
		"Everything is done, summarizing the work for the user.",
	}
	for _, text := range distinct {
		events <- doomLoopDeltaEvent(text)
	}
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID:    "sess-ok",
		RequestID:    "task-ok",
		InvocationID: "turn-ok",
	}, nil, loggateway.NewNoop())

	if result.DoomLoopDetected {
		t.Fatal("unexpected DoomLoopDetected for distinct deltas")
	}
	if !result.HasContent {
		t.Fatal("expected HasContent=true for normal deltas")
	}
}
