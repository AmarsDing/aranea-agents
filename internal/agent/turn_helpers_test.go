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
	// Billing semantics: every round is a separately billed API call, so
	// prompt/cached/completion are summed across rounds.
	if result.PromptTok != 300 || result.CompletionTok != 80 {
		t.Fatalf("multi-round tokens: prompt=%d completion=%d", result.PromptTok, result.CompletionTok)
	}
	if result.CachedTok != 190 {
		t.Fatalf("CachedTok = %d, want 190 (sum across rounds)", result.CachedTok)
	}
	// Context occupancy reflects the final round only.
	if result.LastRoundPromptTok != 200 || result.LastRoundCompletionTok != 30 {
		t.Fatalf("last-round tokens: prompt=%d completion=%d", result.LastRoundPromptTok, result.LastRoundCompletionTok)
	}
	// A chunk with the same prompt is the SAME round (cumulative streaming):
	// take the per-round max for completion/cached, not another sum step.
	accumulateStreamUsage(&result, &trpcevent.Event{}, meta, 200, 35, 100)
	if result.PromptTok != 300 {
		t.Fatalf("PromptTok = %d, want 300 (same-round chunk must not double count)", result.PromptTok)
	}
	if result.CompletionTok != 85 {
		t.Fatalf("CompletionTok = %d, want 85 (50 + max(30,35))", result.CompletionTok)
	}
	if result.CachedTok != 190 {
		t.Fatalf("CachedTok = %d, want 190 (40 + max(150,100))", result.CachedTok)
	}
	// Prompt shrink (mid-run compaction) also starts a new billed round.
	accumulateStreamUsage(&result, &trpcevent.Event{}, meta, 60, 10, 0)
	if result.PromptTok != 360 || result.LastRoundPromptTok != 60 {
		t.Fatalf("post-compaction: prompt=%d lastRound=%d, want 360/60", result.PromptTok, result.LastRoundPromptTok)
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

func TestAccumulateStreamUsage_memberMultiRoundSums(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{
		TeamID:          "team-1",
		MemberAgentKeys: map[string]struct{}{"worker-b": {}},
	}
	usageEv := func(p, c, cached int) *trpcevent.Event {
		return &trpcevent.Event{
			Author: "worker-b",
			Response: &trpcmodel.Response{
				Usage: &trpcmodel.Usage{PromptTokens: p, CompletionTokens: c},
			},
		}
	}
	// Two rounds for worker-b: billing totals must sum across rounds.
	accumulateStreamUsage(&result, usageEv(100, 50, 0), meta, 100, 50, 10)
	accumulateStreamUsage(&result, usageEv(200, 30, 0), meta, 200, 30, 20)
	u, ok := result.MemberUsage["worker-b"]
	if !ok {
		t.Fatalf("member usage missing: %+v", result.MemberUsage)
	}
	if u.PromptTokens != 300 || u.CompletionTokens != 80 || u.CachedTokens != 30 {
		t.Fatalf("member multi-round: got %+v, want prompt=300 completion=80 cached=30", u)
	}
}

// TestAccumulateStreamUsage_parallelMemberInterleave 回归（2026-08-27）：并行
// 成员的累计 usage 在单条 consume 流上交错时，轮次边界必须按 author 分轨——
// A(10k)→B(8k)→A(10k 同轮重报） 不得把 A 的第一轮再计一次（虚增 10k 会经
// OnPromptTokensAccumulated 误触 run 预算闸）。
func TestAccumulateStreamUsage_parallelMemberInterleave(t *testing.T) {
	var result EventStreamResult
	meta := ProjectMeta{
		TeamID:          "team-1",
		MemberAgentKeys: map[string]struct{}{"worker-a": {}, "worker-b": {}},
	}
	usageEv := func(author string, p, c int) *trpcevent.Event {
		return &trpcevent.Event{
			Author: author,
			Response: &trpcmodel.Response{
				Usage: &trpcmodel.Usage{PromptTokens: p, CompletionTokens: c},
			},
		}
	}
	accumulateStreamUsage(&result, usageEv("worker-a", 10000, 100), meta, 10000, 100, 0)
	accumulateStreamUsage(&result, usageEv("worker-b", 8000, 80), meta, 8000, 80, 0)
	// A 同轮累计重报（provider 流式重发同轮 usage）：不得触发新轮。
	accumulateStreamUsage(&result, usageEv("worker-a", 10000, 120), meta, 10000, 120, 0)
	if result.PromptTok != 18000 {
		t.Fatalf("PromptTok = %d, want 18000 (A 同轮重报不得双计)", result.PromptTok)
	}
	if result.CompletionTok != 200 {
		t.Fatalf("CompletionTok = %d, want 200 (max(100,120) + 80)", result.CompletionTok)
	}
	ua := result.MemberUsage["worker-a"]
	if ua.PromptTokens != 10000 || ua.CompletionTokens != 120 {
		t.Fatalf("worker-a usage = %+v, want prompt=10000 completion=120", ua)
	}
	ub := result.MemberUsage["worker-b"]
	if ub.PromptTokens != 8000 || ub.CompletionTokens != 80 {
		t.Fatalf("worker-b usage = %+v, want prompt=8000 completion=80", ub)
	}
	// A 进入第二轮（prompt 增长）后再与 B 交错：A 两轮 + B 一轮。
	accumulateStreamUsage(&result, usageEv("worker-a", 12000, 50), meta, 12000, 50, 0)
	accumulateStreamUsage(&result, usageEv("worker-b", 8000, 90), meta, 8000, 90, 0)
	if result.PromptTok != 30000 {
		t.Fatalf("PromptTok = %d, want 30000 (10000+12000+8000)", result.PromptTok)
	}
	if result.CompletionTok != 260 {
		t.Fatalf("CompletionTok = %d, want 260 (120+50+90)", result.CompletionTok)
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
