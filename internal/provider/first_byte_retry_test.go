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

func (m *stallThenAnswerModel) Info() trpcmodel.Info {
	return trpcmodel.Info{Name: "stall-then-answer"}
}

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
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())

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
	case <-time.After(5 * time.Second):
		t.Fatal("no response after retry — retry did not happen or hung")
	}
	if got := atomic.LoadInt32(&inner.calls); got != 2 {
		t.Fatalf("inner calls = %d, want 2 (stall + one reconnect)", got)
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
	wrapped := WrapModelWithLivenessGuard(inner, "openai", "gpt-4o", loggateway.NewNoop())

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
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())

	// No first-byte budget in ctx → first-byte retry stays disabled; the
	// stall guard default (90s) is far beyond this test's ctx deadline, so
	// the ctx deadline wins and the attempt is NOT reconnected.
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
		t.Fatalf("inner calls = %d, want 1 (no reconnect before ctx deadline)", got)
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
	wrapped := WrapModelWithLivenessGuard(inner, "openai", "gpt-4o", loggateway.NewNoop())

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
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())
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
		t.Fatalf("inner calls = %d, want 2 (stall + one reconnect)", n)
	}
}

func TestFirstByteGuardWithRetry(t *testing.T) {
	if got := FirstByteGuardWithRetry(0, 5); got != 0 {
		t.Fatalf("zero budget = %v, want 0 (guard unchanged)", got)
	}
	// 旧单发重试口径回归：1 reconnect → 2×budget + backoff(1s) + slack(2s)。
	if got := FirstByteGuardWithRetry(30*time.Second, 1); got != 63*time.Second {
		t.Fatalf("30s budget / 1 reconnect guard = %v, want 63s", got)
	}
	// 新默认口径：5 reconnects → 6×30s + (1+2+4+8+16)s + 2s = 213s。
	if got := FirstByteGuardWithRetry(30*time.Second, 5); got != 213*time.Second {
		t.Fatalf("30s budget / 5 reconnects guard = %v, want 213s", got)
	}
}

// --- 2026-09-01 活性守卫治理新增测试 ---

// midStallThenAnswerModel: attempt 1 emits one chunk then stalls until
// cancelled; attempt 2 answers fully. Simulates a mid-stream stall.
type midStallThenAnswerModel struct {
	calls int32
}

func (m *midStallThenAnswerModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "mid-stall"} }

func (m *midStallThenAnswerModel) GenerateContent(ctx context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	n := atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response, 4)
	if n == 1 {
		ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
		go func() {
			<-ctx.Done()
			close(ch)
		}()
		return ch, nil
	}
	ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
	ch <- &trpcmodel.Response{Object: "chat.completion", Done: true}
	close(ch)
	return ch, nil
}

func TestLivenessGuard_MidStreamStallReconnects(t *testing.T) {
	inner := &midStallThenAnswerModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())

	ctx := WithStallBudget(context.Background(), 50*time.Millisecond, 3)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var got []*trpcmodel.Response
	timeout := time.After(5 * time.Second)
	for {
		select {
		case resp, ok := <-out:
			if !ok {
				goto drained
			}
			got = append(got, resp)
		case <-timeout:
			t.Fatal("stream hung — stall reconnect did not fire")
		}
	}
drained:
	// attempt1 的半截 chunk + attempt2 的两个事件都应可见（R1 半截重复
	// 属已登记的可观测性噪音）；最后一个必须是 attempt2 的 Done 事件。
	if len(got) != 3 {
		t.Fatalf("forwarded %d events, want 3 (half attempt1 + full attempt2)", len(got))
	}
	if last := got[len(got)-1]; !last.Done {
		t.Fatal("last event must be attempt2's Done response")
	}
	if n := atomic.LoadInt32(&inner.calls); n != 2 {
		t.Fatalf("inner calls = %d, want 2 (stall + one reconnect)", n)
	}
}

// streamErrorMidModel: attempt 1 emits a chunk then a stream_error; attempt 2
// answers fully. The stream_error must be swallowed and trigger a reconnect.
type streamErrorMidModel struct {
	calls int32
}

func (m *streamErrorMidModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "stream-error-mid"} }

func (m *streamErrorMidModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	n := atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response, 4)
	if n == 1 {
		ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
		ch <- &trpcmodel.Response{Error: &trpcmodel.ResponseError{Type: trpcmodel.ErrorTypeStreamError, Message: "connection reset"}}
		close(ch)
		return ch, nil
	}
	ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
	ch <- &trpcmodel.Response{Object: "chat.completion", Done: true}
	close(ch)
	return ch, nil
}

func TestLivenessGuard_MidStreamErrorReconnects(t *testing.T) {
	inner := &streamErrorMidModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "openai", "gpt-4o", loggateway.NewNoop())

	ctx := WithStallBudget(context.Background(), time.Second, 3)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var sawErr bool
	var last *trpcmodel.Response
	timeout := time.After(5 * time.Second)
	for {
		select {
		case resp, ok := <-out:
			if !ok {
				goto drained
			}
			if resp.Error != nil {
				sawErr = true
			}
			last = resp
		case <-timeout:
			t.Fatal("stream hung — stream_error reconnect did not fire")
		}
	}
drained:
	if sawErr {
		t.Fatal("mid-stream stream_error must be swallowed for reconnect, not forwarded")
	}
	if last == nil || !last.Done {
		t.Fatal("last event must be attempt2's Done response")
	}
	if n := atomic.LoadInt32(&inner.calls); n != 2 {
		t.Fatalf("inner calls = %d, want 2 (stream_error + one reconnect)", n)
	}
}

// semanticErrorModel always emits a semantic (non-stream) error mid-stream:
// must be forwarded without reconnect.
type semanticErrorModel struct {
	calls int32
}

func (m *semanticErrorModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "semantic-error"} }

func (m *semanticErrorModel) GenerateContent(_ context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response, 2)
	ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
	ch <- &trpcmodel.Response{Error: &trpcmodel.ResponseError{Type: trpcmodel.ErrorTypeAPIError, Message: "insufficient balance"}}
	close(ch)
	return ch, nil
}

func TestLivenessGuard_SemanticErrorNotRetried(t *testing.T) {
	inner := &semanticErrorModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "openai", "gpt-4o", loggateway.NewNoop())

	ctx := WithStallBudget(context.Background(), time.Second, 3)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var errResp *trpcmodel.Response
	for resp := range out {
		if resp.Error != nil {
			errResp = resp
		}
	}
	if errResp == nil {
		t.Fatal("semantic error must be forwarded")
	}
	// B2 归一化：Object 为空的错误响应必须被置为 ObjectTypeError。
	if errResp.Object != trpcmodel.ObjectTypeError {
		t.Fatalf("error Object = %q, want %q (normalized)", errResp.Object, trpcmodel.ObjectTypeError)
	}
	if n := atomic.LoadInt32(&inner.calls); n != 1 {
		t.Fatalf("inner calls = %d, want 1 (semantic error never retried)", n)
	}
}

// alwaysStallModel never produces anything; every attempt stalls until
// cancelled. Reconnects must exhaust and the stream tail must carry a
// terminal error response.
type alwaysStallModel struct {
	calls int32
}

func (m *alwaysStallModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "always-stall"} }

func (m *alwaysStallModel) GenerateContent(ctx context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response)
	go func() {
		<-ctx.Done()
		close(ch)
	}()
	return ch, nil
}

func TestLivenessGuard_ExhaustedEmitsTerminalError(t *testing.T) {
	inner := &alwaysStallModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop())

	ctx := WithFirstByteRetryBudget(context.Background(), 30*time.Millisecond)
	ctx = WithStallBudget(ctx, 30*time.Millisecond, 2)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var last *trpcmodel.Response
	timeout := time.After(10 * time.Second)
	for {
		select {
		case resp, ok := <-out:
			if !ok {
				goto drained
			}
			last = resp
		case <-timeout:
			t.Fatal("stream hung — exhaustion terminal error not emitted")
		}
	}
drained:
	if last == nil || last.Error == nil {
		t.Fatal("stream tail must carry a terminal error response")
	}
	if last.Error.Type != trpcmodel.ErrorTypeStreamError {
		t.Fatalf("terminal error type = %q, want %q", last.Error.Type, trpcmodel.ErrorTypeStreamError)
	}
	if last.Object != trpcmodel.ObjectTypeError {
		t.Fatalf("terminal error Object = %q, want %q", last.Object, trpcmodel.ObjectTypeError)
	}
	if !last.Done {
		t.Fatal("terminal error response must be Done")
	}
	if n := atomic.LoadInt32(&inner.calls); n != 3 {
		t.Fatalf("inner calls = %d, want 3 (1 initial + 2 reconnects)", n)
	}
}

// slowHealthyModel emits one event every 20ms for 300ms total: a healthy
// long stream that must NOT be killed by the stall guard (90s default far
// exceeds the 20ms gap).
type slowHealthyModel struct {
	calls int32
}

func (m *slowHealthyModel) Info() trpcmodel.Info { return trpcmodel.Info{Name: "slow-healthy"} }

func (m *slowHealthyModel) GenerateContent(ctx context.Context, _ *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	atomic.AddInt32(&m.calls, 1)
	ch := make(chan *trpcmodel.Response, 32)
	go func() {
		defer close(ch)
		for i := 0; i < 15; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(20 * time.Millisecond):
			}
			ch <- &trpcmodel.Response{Object: "chat.completion.chunk"}
		}
	}()
	return ch, nil
}

func TestLivenessGuard_LongHealthyStreamNotKilled(t *testing.T) {
	inner := &slowHealthyModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "openai", "gpt-4o", loggateway.NewNoop())

	// stall 预算 100ms ≫ 事件间隔 20ms：持续产出即活性健康，不得触发重连。
	ctx := WithStallBudget(context.Background(), 100*time.Millisecond, 3)
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	timeout := time.After(5 * time.Second)
	for {
		select {
		case _, ok := <-out:
			if !ok {
				goto drained
			}
			count++
		case <-timeout:
			t.Fatal("healthy stream killed or hung")
		}
	}
drained:
	if count != 15 {
		t.Fatalf("forwarded %d events, want 15 (no reconnect interference)", count)
	}
	if n := atomic.LoadInt32(&inner.calls); n != 1 {
		t.Fatalf("inner calls = %d, want 1 (healthy stream never reconnected)", n)
	}
}

func TestStallPolicyFromConfigJSON(t *testing.T) {
	if d, n := StallPolicyFromConfigJSON(""); d != 0 || n != 0 {
		t.Fatalf("empty config = (%v, %d), want (0, 0)", d, n)
	}
	if d, n := StallPolicyFromConfigJSON(`{}`); d != 0 || n != 0 {
		t.Fatalf("empty object = (%v, %d), want (0, 0)", d, n)
	}
	if d, n := StallPolicyFromConfigJSON(`{not json`); d != 0 || n != 0 {
		t.Fatalf("invalid json = (%v, %d), want (0, 0)", d, n)
	}
	d, n := StallPolicyFromConfigJSON(`{"stall_timeout_sec":120,"stall_max_attempts":8}`)
	if d != 120*time.Second || n != 8 {
		t.Fatalf("full config = (%v, %d), want (120s, 8)", d, n)
	}
	// 单字段：另一字段保持未配置（0 = 回退包默认）。
	d, n = StallPolicyFromConfigJSON(`{"stall_timeout_sec":45}`)
	if d != 45*time.Second || n != 0 {
		t.Fatalf("partial config = (%v, %d), want (45s, 0)", d, n)
	}
}

func TestWithStallBudgetDefaults(t *testing.T) {
	p := stallPolicyFromContext(context.Background())
	if p.timeout != DefaultStallTimeout || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("absent ctx = (%v, %d), want defaults (%v, %d)", p.timeout, p.maxReconnects, DefaultStallTimeout, DefaultStallMaxReconnects)
	}
	// 显式注入覆盖默认。
	ctx := WithStallBudget(context.Background(), 5*time.Second, 2)
	p = stallPolicyFromContext(ctx)
	if p.timeout != 5*time.Second || p.maxReconnects != 2 {
		t.Fatalf("injected = (%v, %d), want (5s, 2)", p.timeout, p.maxReconnects)
	}
	// 非正字段回退包默认。
	ctx = WithStallBudget(context.Background(), 0, -1)
	p = stallPolicyFromContext(ctx)
	if p.timeout != DefaultStallTimeout || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("non-positive fields = (%v, %d), want defaults", p.timeout, p.maxReconnects)
	}
}
