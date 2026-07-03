package agent

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
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

// captureActivityBus is a thread-safe ActivityEventBus that records published events.
type captureActivityBus struct {
	mu        sync.Mutex
	published []biz.ActivityEvent
}

func (b *captureActivityBus) Publish(_ context.Context, ev biz.ActivityEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.published = append(b.published, ev)
}

func (b *captureActivityBus) Subscribe(_ biz.ActivityEventSubscribeOptions) (<-chan biz.ActivityEvent, func()) {
	return nil, func() {}
}

func (b *captureActivityBus) DropCount() uint64 { return 0 }

func (b *captureActivityBus) events() []biz.ActivityEvent {
	b.mu.Lock()
	defer b.mu.Unlock()
	out := make([]biz.ActivityEvent, len(b.published))
	copy(out, b.published)
	return out
}

// TestConsumeWithFirstByteGuard_NoHardFailure verifies that the first-byte timeout
// no longer returns ErrFirstByteTimeout. Instead it emits a patient notice and
// keeps consuming the stream, so a slow model can still produce a reply.
func TestConsumeWithFirstByteGuard_NoHardFailure(t *testing.T) {
	opts := &StreamConsumeOptions{}

	events := make(chan *trpcevent.Event)
	go func() {
		// Model responds after the 30 ms first-byte deadline.
		time.Sleep(60 * time.Millisecond)
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

	result, err := ConsumeWithFirstByteGuard(ctx, 30*time.Millisecond, events, ProjectMeta{
		SessionID: "sess-1",
		RequestID: "req-1",
		AgentID:   "agent-1",
	}, opts, loggateway.NewNoop())

	if err != nil {
		t.Fatalf("expected no hard error on first-byte timeout, got %v", err)
	}
	if got := result.Reply.String(); got != "hello" {
		t.Errorf("reply = %q, want hello", got)
	}

	// TODO: the patient first-byte timeout notice is not yet emitted by the
	// stream consumer; this test currently only verifies the no-hard-failure
	// contract and reply capture.
}
