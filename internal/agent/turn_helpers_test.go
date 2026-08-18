package agent

import (
	"context"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestAccumulateStreamUsage_multiLLMRounds(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{SessionID: "s1"}
	accumulateStreamUsage(&result, &trpcevent.Event{}, meta, 100, 50, 40)
	accumulateStreamUsage(&result, &trpcevent.Event{}, meta, 200, 30, 150)
	if result.PromptTok != 200 || result.CompletionTok != 80 {
		t.Fatalf("multi-round tokens: prompt=%d completion=%d", result.PromptTok, result.CompletionTok)
	}
	if result.CachedTok != 150 {
		t.Fatalf("CachedTok = %d, want 150 (max across rounds)", result.CachedTok)
	}
	// Lower cached in a later chunk must not shrink the accumulated max.
	accumulateStreamUsage(&result, &trpcevent.Event{}, meta, 200, 35, 100)
	if result.CachedTok != 150 {
		t.Fatalf("CachedTok = %d, want 150 (monotonic max)", result.CachedTok)
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
	accumulateStreamUsage(&result, ev, meta, 100, 50, 30)
	if result.PromptTok != 100 || result.CompletionTok != 50 {
		t.Fatalf("aggregate tokens: in=%d out=%d", result.PromptTok, result.CompletionTok)
	}
	u, ok := result.MemberUsage["worker-b"]
	if !ok || u.PromptTokens != 100 || u.CompletionTokens != 50 {
		t.Fatalf("member usage: %+v ok=%v", result.MemberUsage, ok)
	}
	if u.CachedTokens != 30 {
		t.Fatalf("member CachedTokens = %d, want 30", u.CachedTokens)
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
	accumulateStreamUsage(&result, ev, meta, 10, 5, 0)
	if len(result.MemberUsage) != 0 {
		t.Fatalf("expected no member usage for team root author, got %+v", result.MemberUsage)
	}
}

func TestConsumeEventStream_skipsToolResponseInReply(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	go func() {
		defer close(events)
		events <- chatChunkEvent("hello", "", true)
		events <- toolResponseEvent(`{"exit_code":1,"output":"fail"}`)
		events <- chatChunkEvent(" world", "", true)
		events <- runnerCompletionEvent()
	}()

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if got := result.Reply.String(); got != "hello world" {
		t.Fatalf("reply = %q, want %q", got, "hello world")
	}
}

func TestConsumeEventStream_accumulatesDeltaReasoning(t *testing.T) {
	events := make(chan *trpcevent.Event, 3)
	go func() {
		defer close(events)
		events <- chatChunkEvent("", "think-a", true)
		events <- chatChunkEvent("", "think-b", true)
		events <- runnerCompletionEvent()
	}()

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if got := result.Reasoning.String(); got != "think-athink-b" {
		t.Fatalf("reasoning = %q", got)
	}
}

// S3-fix verification: final (non-partial) aggregated response carries the
// FULL reasoning/content — it must replace, not append to, the accumulated deltas.
func TestConsumeEventStream_finalResponseReplacesReasoning(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	go func() {
		defer close(events)
		events <- chatChunkEvent("", "think-a", true)
		events <- chatChunkEvent("", "think-b", true)
		// Final aggregated response carries the complete reasoning.
		events <- finalChatCompletionEvent("", "think-athink-b")
		events <- runnerCompletionEvent()
	}()

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if got := result.Reasoning.String(); got != "think-athink-b" {
		t.Fatalf("reasoning = %q, want %q", got, "think-athink-b")
	}
}

func TestConsumeEventStream_finalResponseReplacesText(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	go func() {
		defer close(events)
		events <- chatChunkEvent("hello", "", true)
		events <- chatChunkEvent(" world", "", true)
		// Final aggregated response carries the complete content.
		events <- finalChatCompletionEvent("hello world", "")
		events <- runnerCompletionEvent()
	}()

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if got := result.Reply.String(); got != "hello world" {
		t.Fatalf("reply = %q, want %q", got, "hello world")
	}
}

func finalChatCompletionEvent(text, reasoning string) *trpcevent.Event {
	return &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: false,
			Choices: []trpcmodel.Choice{{
				Message: trpcmodel.Message{
					Content:          text,
					ReasoningContent: reasoning,
				},
			}},
		},
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

func runnerCompletionEvent() *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeRunnerCompletion,
			Done:   true,
		},
	}
}

// runnerCompletionWithUsageEvent builds a RunnerCompletion event carrying the
// authoritative final usage payload (most accurate source).
func runnerCompletionWithUsageEvent(promptTok, completionTok int) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeRunnerCompletion,
			Done:   true,
			Usage: &trpcmodel.Usage{
				PromptTokens:     promptTok,
				CompletionTokens: completionTok,
			},
		},
	}
}

// streamingUsageEvent builds a partial chat.completion chunk carrying interim usage.
func streamingUsageEvent(promptTok, completionTok int) *trpcevent.Event {
	return &trpcevent.Event{
		Author: "agent",
		Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Usage: &trpcmodel.Usage{
				PromptTokens:     promptTok,
				CompletionTokens: completionTok,
			},
		},
	}
}

// responseErrorEvent simulates the framework's mid-flight error path which
// suppresses the final chat.completion event (TECH-DEBT(usage-source)).
func responseErrorEvent(msg string) *trpcevent.Event {
	return &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeChatCompletionChunk,
			Error:  &trpcmodel.ResponseError{Message: msg},
		},
	}
}

// --- UsageSource tracking tests (Problem 5) ---

// TestConsumeEventStream_usageSourceRunnerCompletion verifies that when the
// RunnerCompletion event carries the final usage payload, UsageSource is set
// to "runner_completion" (authoritative source).
func TestConsumeEventStream_usageSourceRunnerCompletion(t *testing.T) {
	events := make(chan *trpcevent.Event, 2)
	go func() {
		defer close(events)
		events <- streamingUsageEvent(10, 5) // interim streaming usage
		events <- runnerCompletionWithUsageEvent(100, 50)
	}()

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if result.UsageSource != "runner_completion" {
		t.Fatalf("UsageSource=%q want %q", result.UsageSource, "runner_completion")
	}
	if result.PromptTok != 100 || result.CompletionTok != 50 {
		t.Fatalf("tokens: prompt=%d completion=%d want 100/50", result.PromptTok, result.CompletionTok)
	}
}

// TestConsumeEventStream_usageSourceStreaming verifies that when only streaming
// usage is observed (no RunnerCompletion usage), UsageSource="streaming".
func TestConsumeEventStream_usageSourceStreaming(t *testing.T) {
	events := make(chan *trpcevent.Event, 2)
	go func() {
		defer close(events)
		events <- streamingUsageEvent(20, 8)
		events <- runnerCompletionEvent() // no usage payload
	}()

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if result.UsageSource != "streaming" {
		t.Fatalf("UsageSource=%q want %q", result.UsageSource, "streaming")
	}
	if result.PromptTok != 20 || result.CompletionTok != 8 {
		t.Fatalf("tokens: prompt=%d completion=%d want 20/8", result.PromptTok, result.CompletionTok)
	}
}

// TestConsumeEventStream_usageSourceEmptyOnError covers the TECH-DEBT path:
// when the stream errors mid-flight, the framework suppresses the final
// chat.completion event carrying usage. UsageSource remains "" to signal
// the missing usage for observability/diagnostics. Downstream
// EstimateTokensIfMissing will estimate from text (UsageSource="estimated").
func TestConsumeEventStream_usageSourceEmptyOnError(t *testing.T) {
	events := make(chan *trpcevent.Event, 2)
	go func() {
		defer close(events)
		events <- chatChunkEvent("partial reply", "", true)
		events <- responseErrorEvent("context deadline exceeded")
	}()

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{SessionID: "s1"}, nil, loggateway.NewNoop())
	if result.UsageSource != "" {
		t.Fatalf("UsageSource=%q want empty (TECH-DEBT path: framework suppresses usage on error)", result.UsageSource)
	}
	if !result.HasError {
		t.Fatal("HasError should be true on error event")
	}
	if result.Reply.String() != "partial reply" {
		t.Fatalf("reply=%q want %q", result.Reply.String(), "partial reply")
	}
}
