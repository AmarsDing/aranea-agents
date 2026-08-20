package agent

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestDisplayMarkdownFromStream_prefersReply(t *testing.T) {
	var r EventStreamResult
	_, _ = r.Reply.WriteString("hello")
	_, _ = r.Reasoning.WriteString("think")
	got, fallback := DisplayMarkdownFromStream(r)
	if got != "hello" {
		t.Fatalf("got %q", got)
	}
	if fallback {
		t.Fatal("expected fallback=false for reply content")
	}
}

func TestDisplayMarkdownFromStream_reasoningFallback(t *testing.T) {
	var r EventStreamResult
	_, _ = r.Reasoning.WriteString("only reasoning")
	got, fallback := DisplayMarkdownFromStream(r)
	if got != "only reasoning" {
		t.Fatalf("got %q", got)
	}
	if !fallback {
		t.Fatal("expected fallback=true for reasoning-only content")
	}
}

func TestDisplayMarkdownFromStream_emptyResult(t *testing.T) {
	var r EventStreamResult
	got, fallback := DisplayMarkdownFromStream(r)
	if got != "" {
		t.Fatalf("got %q, want empty", got)
	}
	if fallback {
		t.Fatal("expected fallback=false for empty result")
	}
}

func TestEstimateTokensIfMissing_skipsWhenUsagePresent(t *testing.T) {
	in, out := EstimateTokensIfMissing(10, 5, "in", "out")
	if in != 10 || out != 5 {
		t.Fatalf("in=%d out=%d", in, out)
	}
}

// TestEstimateTokensIfMissing_estimatesPromptFromInput covers the chat-path bug:
// when promptTok=0 but completionTok>0, prompt should be estimated from inputPreview
// (not from output text).
func TestEstimateTokensIfMissing_estimatesPromptFromInput(t *testing.T) {
	in, out := EstimateTokensIfMissing(0, 5, "user input text", "reply")
	if in != RoughTokenEstimate("user input text") {
		t.Fatalf("prompt estimated wrong: in=%d want=%d", in, RoughTokenEstimate("user input text"))
	}
	if out != 5 {
		t.Fatalf("completion should be preserved: out=%d want=5", out)
	}
}

// TestEstimateTokensIfMissing_estimatesCompletionFromOutput covers the symmetric case:
// when completionTok=0 but promptTok>0, completion should be estimated from displayMarkdown.
func TestEstimateTokensIfMissing_estimatesCompletionFromOutput(t *testing.T) {
	in, out := EstimateTokensIfMissing(7, 0, "input", "assistant reply text")
	if in != 7 {
		t.Fatalf("prompt should be preserved: in=%d want=7", in)
	}
	if out != RoughTokenEstimate("assistant reply text") {
		t.Fatalf("completion estimated wrong: out=%d want=%d", out, RoughTokenEstimate("assistant reply text"))
	}
}

// TestEstimateTokensIfMissing_estimatesBothWhenAbsent covers both missing:
// prompt from input, completion from output — independently, not from combined text.
func TestEstimateTokensIfMissing_estimatesBothWhenAbsent(t *testing.T) {
	in, out := EstimateTokensIfMissing(0, 0, "user input", "assistant reply")
	wantIn := RoughTokenEstimate("user input")
	wantOut := RoughTokenEstimate("assistant reply")
	if in != wantIn {
		t.Fatalf("prompt estimated wrong: in=%d want=%d", in, wantIn)
	}
	if out != wantOut {
		t.Fatalf("completion estimated wrong: out=%d want=%d", out, wantOut)
	}
	// Critical: prompt and completion must NOT be equal when input/output differ in size.
	if in == out {
		t.Fatal("prompt and completion should differ when input/output text sizes differ (regression: old 4-branch logic estimated both from output, making them equal)")
	}
}

// TestEstimateTokensIfMissing_emptyInputReturnsZeroPrompt covers the empty-input case:
// when inputPreview="" and promptTok=0, prompt stays 0 (no spurious estimation from output).
func TestEstimateTokensIfMissing_emptyInputReturnsZeroPrompt(t *testing.T) {
	in, out := EstimateTokensIfMissing(0, 0, "", "reply text")
	if in != 0 {
		t.Fatalf("empty input should yield prompt=0, got %d (regression: old logic estimated prompt from output text)", in)
	}
	if out != RoughTokenEstimate("reply text") {
		t.Fatalf("completion estimated wrong: out=%d", out)
	}
}

// TestConsumeWithFirstByteGuard_SilentStallHardFails verifies that a muted
// events channel wakes on the first-byte deadline, aborts the run, and
// returns ErrFirstByteTimeout instead of waiting for the HTTP timeout.
func TestConsumeWithFirstByteGuard_SilentStallHardFails(t *testing.T) {
	events := make(chan *trpcevent.Event)
	defer close(events)

	aborted := make(chan struct{})
	opts := &StreamConsumeOptions{
		AbortOnStall: func() { close(aborted) },
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result, err := ConsumeWithFirstByteGuard(ctx, 40*time.Millisecond, events, ProjectMeta{
		SessionID: "sess-1",
		RequestID: "req-1",
		AgentID:   "agent-1",
	}, opts, loggateway.NewNoop())

	if !errors.Is(err, ErrFirstByteTimeout) {
		t.Fatalf("err = %v, want ErrFirstByteTimeout", err)
	}
	if !result.FirstByteTimedOut {
		t.Fatal("expected FirstByteTimedOut")
	}
	select {
	case <-aborted:
	case <-time.After(200 * time.Millisecond):
		t.Fatal("AbortOnStall was not called")
	}
}

func TestConsumeWithFirstByteGuard_LateChunkAfterDeadlineDoesNotCount(t *testing.T) {
	events := make(chan *trpcevent.Event)
	go func() {
		time.Sleep(80 * time.Millisecond)
		events <- &trpcevent.Event{
			Response: &trpcmodel.Response{
				Object: trpcmodel.ObjectTypeChatCompletionChunk,
				Choices: []trpcmodel.Choice{
					{Delta: trpcmodel.Message{Content: "hello"}},
				},
			},
		}
		close(events)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := ConsumeWithFirstByteGuard(ctx, 20*time.Millisecond, events, ProjectMeta{
		SessionID: "sess-1",
		RequestID: "req-1",
		AgentID:   "agent-1",
	}, &StreamConsumeOptions{}, loggateway.NewNoop())
	if !errors.Is(err, ErrFirstByteTimeout) {
		t.Fatalf("late chunk after deadline should still timeout, got %v", err)
	}
}

func TestConsumeWithFirstByteGuard_FirstTokenSucceeds(t *testing.T) {
	events := make(chan *trpcevent.Event, 1)
	events <- &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeChatCompletionChunk,
			Choices: []trpcmodel.Choice{
				{Delta: trpcmodel.Message{Content: "hello"}},
			},
		},
	}
	close(events)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	result, err := ConsumeWithFirstByteGuard(ctx, 200*time.Millisecond, events, ProjectMeta{
		SessionID: "sess-1",
		RequestID: "req-1",
		AgentID:   "agent-1",
	}, &StreamConsumeOptions{}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := result.Reply.String(); got != "hello" {
		t.Errorf("reply = %q, want hello", got)
	}
}

func TestCountsAsFirstByte_IgnoresRunnerCompletion(t *testing.T) {
	ev := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Object: trpcmodel.ObjectTypeRunnerCompletion,
			Done:   true,
		},
	}
	if !ev.IsRunnerCompletion() {
		t.Fatal("fixture is not a runner completion")
	}
	if countsAsFirstByte(ev) {
		t.Fatal("runner_completion must not count as first byte")
	}
}

func TestCountsAsFirstByte_ResponseErrorCounts(t *testing.T) {
	ev := &trpcevent.Event{
		Response: &trpcmodel.Response{
			Error: &trpcmodel.ResponseError{Message: "Insufficient Balance"},
		},
	}
	if !countsAsFirstByte(ev) {
		t.Fatal("provider error events must count as first byte so billing can surface immediately")
	}
}

// P1-C：session_turns 真实指标——LLM 轮次按 response ID 去重、工具调用按
// tool-call ID 去重（流式 delta 跨 chunk 重复同一 ID 不得重复计数）。
func TestConsumeEventStream_CountsModelAndToolCalls(t *testing.T) {
	chunk := func(id string, partial bool, delta trpcmodel.Message) *trpcevent.Event {
		return &trpcevent.Event{Response: &trpcmodel.Response{
			ID:        id,
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: partial,
			Choices:   []trpcmodel.Choice{{Delta: delta}},
		}}
	}
	toolDelta := func(callID string) trpcmodel.Message {
		return trpcmodel.Message{ToolCalls: []trpcmodel.ToolCall{{
			ID:       callID,
			Function: trpcmodel.FunctionDefinitionParam{Name: "datetime"},
		}}}
	}

	events := make(chan *trpcevent.Event, 8)
	// 第 1 轮：同一 response ID 的多个 chunk + 同一 tool call 的重复 delta。
	events <- chunk("resp-1", true, trpcmodel.Message{Content: "先"})
	events <- chunk("resp-1", true, toolDelta("call-1"))
	events <- chunk("resp-1", true, toolDelta("call-1"))
	events <- chunk("resp-1", false, toolDelta("call-1"))
	// 第 2 轮：新 response ID + 另一个 tool call。
	events <- chunk("resp-2", true, toolDelta("call-2"))
	events <- chunk("resp-2", false, trpcmodel.Message{Content: "完成"})
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID: "sess-1",
		RequestID: "req-1",
		AgentID:   "agent-1",
	}, &StreamConsumeOptions{}, loggateway.NewNoop())

	if result.ModelCallCount != 2 {
		t.Errorf("ModelCallCount = %d, want 2", result.ModelCallCount)
	}
	if result.ToolCallCount != 2 {
		t.Errorf("ToolCallCount = %d, want 2", result.ToolCallCount)
	}
	if result.FirstTokenMs < 0 {
		t.Errorf("FirstTokenMs = %d, want >= 0", result.FirstTokenMs)
	}
}

// P1-C：无 response ID 的 provider 退化为按最终（非 partial）响应计轮次。
func TestConsumeEventStream_CountsModelCallsWithoutResponseID(t *testing.T) {
	events := make(chan *trpcevent.Event, 4)
	for i := 0; i < 2; i++ {
		events <- &trpcevent.Event{Response: &trpcmodel.Response{
			Object:    trpcmodel.ObjectTypeChatCompletionChunk,
			IsPartial: true,
			Choices:   []trpcmodel.Choice{{Delta: trpcmodel.Message{Content: "x"}}},
		}}
	}
	events <- &trpcevent.Event{Response: &trpcmodel.Response{
		Object:    trpcmodel.ObjectTypeChatCompletionChunk,
		IsPartial: false,
		Choices:   []trpcmodel.Choice{{Message: trpcmodel.Message{Content: "done"}}},
	}}
	close(events)

	result := ConsumeEventStream(context.Background(), events, ProjectMeta{
		SessionID: "sess-1",
		RequestID: "req-1",
		AgentID:   "agent-1",
	}, &StreamConsumeOptions{}, loggateway.NewNoop())

	if result.ModelCallCount != 1 {
		t.Errorf("ModelCallCount = %d, want 1 (仅最终响应计数)", result.ModelCallCount)
	}
}
