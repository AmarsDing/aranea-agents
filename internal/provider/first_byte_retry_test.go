package provider

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// stallThenAnswerModel simulates a provider whose first call stalls until
// cancelled (first-byte silence) and whose second call answers immediately.
type stallThenAnswerModel struct {
	calls int32
}

func (m *stallThenAnswerModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "stall-then-answer"} }

func (m *stallThenAnswerModel) GenerateContent(ctx context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	n := atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response, 1)
	if n == 1 {
		// Stall: produce nothing until the caller cancels, then close so the
		// decorator's background drain terminates.
		go func() {
			<-ctx.Done()
			close(ch)
		}()
		return ch, nil
	}
	ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
	close(ch)
	return ch, nil
}

func TestFirstByteRetry_SilentAttemptRetriedOnce(t *testing.T) {
	inner := &stallThenAnswerModel{}
	wrapped := WrapModelWithFirstByteRetry(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())

	ctx := WithFirstByteRetryBudget(context.Background(), 50*time.Millisecond)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case resp, ok := <-out:
		if !ok || resp == nil {
			t.Fatal("expected retried response, got closed/empty channel")
		}
	case <-time.After(3 * time.Second):
		t.Fatal("no response after retry — retry did not happen or hung")
	}
	if got := atomic.LoadInt32(&inner.calls); got != 2 {
		t.Fatalf("inner calls = %d, want 2 (stall + one retry)", got)
	}
}

// immediateModel answers on the first call; no retry must fire.
type immediateModel struct {
	calls int32
}

func (m *immediateModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "immediate"} }

func (m *immediateModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response, 1)
	ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
	close(ch)
	return ch, nil
}

func TestFirstByteRetry_FastAttemptNotRetried(t *testing.T) {
	inner := &immediateModel{}
	wrapped := WrapModelWithFirstByteRetry(inner, "openai", "gpt-4o", loggateway.NewNoop())

	ctx := WithFirstByteRetryBudget(context.Background(), time.Second)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp, ok := <-out; !ok || resp == nil {
		t.Fatal("expected first-attempt response")
	}
	if got := atomic.LoadInt32(&inner.calls); got != 1 {
		t.Fatalf("inner calls = %d, want 1 (no retry)", got)
	}
}

func TestFirstByteRetry_NoBudgetPassThrough(t *testing.T) {
	inner := &stallThenAnswerModel{}
	wrapped := WrapModelWithFirstByteRetry(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())

	// No budget in ctx → decorator is a pass-through; the stall is NOT cut.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Channel stays silent until ctx deadline closes attempt 1.
	select {
	case <-out:
		// closed after ctx timeout — fine
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after ctx deadline")
	}
	if got := atomic.LoadInt32(&inner.calls); got != 1 {
		t.Fatalf("inner calls = %d, want 1 (retry disabled without budget)", got)
	}
}

// errorFirstModel returns an API error event as the FIRST event — this is not
// a stall and must be forwarded without retry.
type errorFirstModel struct {
	calls int32
}

func (m *errorFirstModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "error-first"} }

func (m *errorFirstModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response, 1)
	ch <- &trpcmodel.Response{Error: &trpcmodel.ResponseError{Message: "rate limited"}}
	close(ch)
	return ch, nil
}

func TestFirstByteRetry_ErrorEventNotRetried(t *testing.T) {
	inner := &errorFirstModel{}
	wrapped := WrapModelWithFirstByteRetry(inner, "openai", "gpt-4o", loggateway.NewNoop())

	ctx := WithFirstByteRetryBudget(context.Background(), time.Second)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	resp, ok := <-out
	if !ok || resp == nil || resp.Error == nil {
		t.Fatal("expected the error event to be forwarded")
	}
	if got := atomic.LoadInt32(&inner.calls); got != 1 {
		t.Fatalf("inner calls = %d, want 1 (error event is not a stall)", got)
	}
}

// stallIterModel is the IterModel flavor of stallThenAnswerModel.
type stallIterModel struct {
	calls int32
}

func (m *stallIterModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "stall-iter"} }

func (m *stallIterModel) GenerateContent(ctx context.Context, req *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	return (&stallThenAnswerModel{calls: m.calls}).GenerateContent(ctx, req)
}

func (m *stallIterModel) GenerateContentIter(ctx context.Context, _ *trpcmodel.Request) (trpcmodel.Seq[*trpcmodel.Response], error) {
	n := atomic.AddInt32(&m.calls, 1)
	return func(yield func(*trpcmodel.Response) bool) {
		if n == 1 {
			<-ctx.Done() // stall until cancelled
			return
		}
		yield(&trpcmodel.Response{Object: "chat.completion.chunk"})
	}, nil
}

func TestFirstByteRetry_IterModelPreservedAndRetried(t *testing.T) {
	inner := &stallIterModel{}
	wrapped := WrapModelWithFirstByteRetry(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())
	iter, ok := wrapped.(trpcmodel.IterModel)
	if !ok {
		t.Fatal("wrapper must preserve IterModel when inner implements it")
	}

	ctx := WithFirstByteRetryBudget(context.Background(), 50*time.Millisecond)
	seq, err := iter.GenerateContentIter(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	got := false
	seq(func(resp *trpcmodel.Response) bool {
		got = resp != nil
		return false
	})
	if !got {
		t.Fatal("expected retried response via iter path")
	}
	if n := atomic.LoadInt32(&inner.calls); n != 2 {
		t.Fatalf("inner calls = %d, want 2 (stall + one retry)", n)
	}
}

func TestFirstByteGuardWithRetry(t *testing.T) {
	if got := FirstByteGuardWithRetry(0); got != 0 {
		t.Fatalf("zero budget = %v, want 0 (guard unchanged)", got)
	}
	if got := FirstByteGuardWithRetry(30 * time.Second); got != 62*time.Second {
		t.Fatalf("30s budget guard = %v, want 62s (2×budget + 2s slack)", got)
	}
}
