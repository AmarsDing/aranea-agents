package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

type recordingPersister struct {
	upserts []event.EnvelopeToolCall
}

func (r *recordingPersister) UpsertActivity(_ context.Context, _ ProjectMeta, tc event.EnvelopeToolCall) error {
	r.upserts = append(r.upserts, tc)
	return nil
}

func TestAccumulateStreamUsage_multiLLMRounds(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{SessionID: "s1"}
	accumulateStreamUsage(&result, &trpcevent.Event{}, meta, 100, 50)
	accumulateStreamUsage(&result, &trpcevent.Event{}, meta, 200, 30)
	if result.PromptTok != 200 || result.CompletionTok != 80 {
		t.Fatalf("multi-round tokens: prompt=%d completion=%d", result.PromptTok, result.CompletionTok)
	}
}

func TestAccumulateStreamUsage_memberByAgentKey(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{
		TeamID:          "team-1",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}, "worker-b": {}},
	}
	ev := &trpcevent.Event{
		Author: "worker-b",
		Response: &trpcmodel.Response{
			Usage: &trpcmodel.Usage{
				PromptTokens:     100,
				CompletionTokens: 50,
			},
		},
	}
	accumulateStreamUsage(&result, ev, meta, 100, 50)
	if result.PromptTok != 100 || result.CompletionTok != 50 {
		t.Fatalf("aggregate tokens: in=%d out=%d", result.PromptTok, result.CompletionTok)
	}
	u, ok := result.MemberUsage["worker-b"]
	if !ok || u.PromptTokens != 100 || u.CompletionTokens != 50 {
		t.Fatalf("member usage: %+v ok=%v", result.MemberUsage, ok)
	}
}

func TestAccumulateStreamUsage_skipsTeamRootAuthor(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{TeamID: "team-1", MemberAgentKeys: map[string]struct{}{"worker-a": {}}}
	ev := &trpcevent.Event{
		Author: "team-parallel",
		Response: &trpcmodel.Response{
			Usage: &trpcmodel.Usage{PromptTokens: 10, CompletionTokens: 5},
		},
	}
	accumulateStreamUsage(&result, ev, meta, 10, 5)
	if len(result.MemberUsage) != 0 {
		t.Fatalf("expected no member usage for team root author, got %+v", result.MemberUsage)
	}
}

func TestConsumeEventStream_skipsToolResponseInReply(t *testing.T) {
	bus := event.NewBus(nil)
	events := make(chan *trpcevent.Event, 4)
	go func() {
		defer close(events)
		events <- chatChunkEvent("hello", "", true)
		events <- toolResponseEvent(`{"exit_code":1,"output":"fail"}`)
		events <- chatChunkEvent(" world", "", true)
		events <- runnerCompletionEvent()
	}()

	result := ConsumeEventStream(context.Background(), events, bus, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if got := result.Reply.String(); got != "hello world" {
		t.Fatalf("reply = %q, want %q", got, "hello world")
	}
}

func TestConsumeEventStream_accumulatesDeltaReasoning(t *testing.T) {
	bus := event.NewBus(nil)
	events := make(chan *trpcevent.Event, 3)
	go func() {
		defer close(events)
		events <- chatChunkEvent("", "think-a", true)
		events <- chatChunkEvent("", "think-b", true)
		events <- runnerCompletionEvent()
	}()

	result := ConsumeEventStream(context.Background(), events, bus, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if got := result.Reasoning.String(); got != "think-athink-b" {
		t.Fatalf("reasoning = %q", got)
	}
}

func TestConsumeEventStream_finalizesStuckTools(t *testing.T) {
	bus := event.NewBus(nil)
	persister := &recordingPersister{}
	events := make(chan *trpcevent.Event, 3)
	go func() {
		defer close(events)
		events <- toolCallEvent("tc-stuck", "hostexec_exec_command")
		events <- chatChunkEvent("done", "", false)
		events <- runnerCompletionEvent()
	}()

	opts := &StreamConsumeOptions{ActivityPersister: persister}
	_ = ConsumeEventStream(context.Background(), events, bus, ProjectMeta{SessionID: "s1"}, opts, loggateway.NewNoop())
	if len(persister.upserts) < 2 {
		t.Fatalf("expected tool_call + finalize upserts, got %d", len(persister.upserts))
	}
	last := persister.upserts[len(persister.upserts)-1]
	if last.ID != "tc-stuck" || last.Status != "failed" {
		t.Fatalf("finalize stuck: %+v", last)
	}
}

func chatChunkEvent(text, reasoning string, partial bool) *trpcevent.Event {
	return &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: partial,
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{
					Content:          text,
					ReasoningContent: reasoning,
				},
			}},
			Timestamp: time.Now(),
		},
	}
}

func toolResponseEvent(content string) *trpcevent.Event {
	return &trpcevent.Event{
		Author: "tool",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeToolResponse,
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{
					ToolID:  "tc1",
					Content: content,
				},
			}},
		},
	}
}

func toolCallEvent(id, name string) *trpcevent.Event {
	return &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeChatCompletionChunk,
			Choices: []trpcmodel.Choice{{
				Delta: trpcmodel.Message{
					ToolCalls: []trpcmodel.ToolCall{{
						ID: id,
						Function: trpcmodel.FunctionDefinitionParam{
							Name: name,
						},
					}},
				},
			}},
		},
	}
}

func runnerCompletionEvent() *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeRunnerCompletion,
			Done:   true,
		},
	}
}
