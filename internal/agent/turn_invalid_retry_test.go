package agent

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/event"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// invalidRetryCaptureBus 捕获 flow_log 事件（镜像 internal/event 测试的
// captureMonitorBus——test-only 类型不可跨包复用）。
type invalidRetryCaptureBus struct {
	mu  sync.Mutex
	evs []contract.MonitorEvent
}

func (b *invalidRetryCaptureBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	b.mu.Lock()
	b.evs = append(b.evs, ev)
	b.mu.Unlock()
}

func (b *invalidRetryCaptureBus) Subscribe(_ contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	return nil, func() {}
}

func (b *invalidRetryCaptureBus) DropCount() uint64 { return 0 }

func (b *invalidRetryCaptureBus) stepCounts() map[string]int {
	b.mu.Lock()
	defer b.mu.Unlock()
	m := make(map[string]int)
	for _, ev := range b.evs {
		if step, ok := ev.Metadata["step_id"].(string); ok {
			m[step]++
		}
	}
	return m
}

func newInvalidRetryEmitter(bus contract.MonitorBus) *event.TraceEmitter {
	return event.NewTraceEmitter(&event.Infra{MonitorEventBus: bus}, event.TraceContext{
		TraceID:   "tr_invalid",
		SessionID: "sess_invalid",
		RunID:     "run_invalid",
		Domain:    event.TraceDomainChat,
	}, loggateway.NewNoop())
}

func fastInvalidRetryOpts() InvalidRetryOptions {
	return InvalidRetryOptions{
		Enabled:     true,
		MaxRetries:  DefaultInvalidMaxRetries,
		BackoffBase: time.Millisecond,
		BackoffCap:  2 * time.Millisecond,
		JitterFn:    func(_ int, base time.Duration) time.Duration { return base },
	}
}

func invalidResult(kind StreamInvalidKind, hasContent, dispatched, doom bool) EventStreamResult {
	r := EventStreamResult{InvalidKind: kind, HasContent: hasContent, ToolCallDispatched: dispatched, DoomLoopDetected: doom}
	if doom {
		r.DoomLoopDetected = true
		r.HasError = true
	}
	if kind != StreamInvalidNone && kind != StreamInvalidMaxTokens && kind != StreamInvalidNoFinishReason {
		r.HasError = true
	}
	return r
}

// TestRunTurnWithInvalidRetry_DisabledPassthrough pin：默认关闭时单次直通，
// 且不重试（OFF 状态行为与接线前逐字节一致的核心断言）。
func TestRunTurnWithInvalidRetry_DisabledPassthrough(t *testing.T) {
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		if attemptNo != 1 {
			t.Fatalf("disabled mode must be single attempt, got attempt %d", attemptNo)
		}
		return invalidResult(StreamInvalidNoFinishReason, false, false, false), nil
	}, InvalidRetryOptions{}, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("disabled mode calls=%d, want 1", calls)
	}
	if res.InvalidKind != StreamInvalidNoFinishReason {
		t.Fatalf("result kind must pass through, got %v", res.InvalidKind)
	}
	if res.InvalidRetryAttempts != 0 {
		t.Fatalf("disabled mode attempts=%d, want 0", res.InvalidRetryAttempts)
	}
}

// TestRunTurnWithInvalidRetry_HardErrorPassthrough pin：硬错误（首字节超时等）
// 一律透传不重试。
func TestRunTurnWithInvalidRetry_HardErrorPassthrough(t *testing.T) {
	calls := 0
	sentinel := errors.New("first byte timeout after 30s")
	_, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return EventStreamResult{FirstByteTimedOut: true, HasError: true}, sentinel
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if !errors.Is(err, sentinel) {
		t.Fatalf("err=%v, want sentinel passthrough", err)
	}
	if calls != 1 {
		t.Fatalf("hard error must not retry, calls=%d", calls)
	}
}

// TestRunTurnWithInvalidRetry_RetriesEmptyNoFinish pin：空响应（无
// finish_reason 且无内容）按表重发，直到成功。
func TestRunTurnWithInvalidRetry_RetriesEmptyNoFinish(t *testing.T) {
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		if attemptNo < 3 {
			return invalidResult(StreamInvalidNoFinishReason, false, false, false), nil
		}
		return invalidResult(StreamInvalidNone, true, false, false), nil
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d, want 3 (2 retries)", calls)
	}
	if res.InvalidRetryAttempts != 2 {
		t.Fatalf("attempts=%d, want 2", res.InvalidRetryAttempts)
	}
}

// TestRunTurnWithInvalidRetry_Exhaustion pin：耗尽上限后返回最后一次结果。
func TestRunTurnWithInvalidRetry_Exhaustion(t *testing.T) {
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidHTTPEOF, false, false, false), nil
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	// 表上限 Retry(2) 与默认 MaxRetries=2 → 3 次调用。
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
	if res.InvalidRetryAttempts != 2 {
		t.Fatalf("attempts=%d, want 2", res.InvalidRetryAttempts)
	}
	if res.InvalidKind != StreamInvalidHTTPEOF {
		t.Fatalf("kind=%v, want http_eof passthrough on exhaustion", res.InvalidKind)
	}
}

// TestRunTurnWithInvalidRetry_DispatchedGate pin：已派发 tool_call 时立即
// Terminate，不重试（重复副作用闸门）。
func TestRunTurnWithInvalidRetry_DispatchedGate(t *testing.T) {
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidHTTPEOF, true, true, false), nil
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("dispatched tool_call must terminate immediately, calls=%d", calls)
	}
	if res.InvalidRetryAttempts != 0 {
		t.Fatalf("attempts=%d, want 0", res.InvalidRetryAttempts)
	}
}

// TestRunTurnWithInvalidRetry_SafetyNeverRetries pin：SAFETY 恒 Terminate。
func TestRunTurnWithInvalidRetry_SafetyNeverRetries(t *testing.T) {
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidSafety, false, false, false), nil
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("safety must never retry, calls=%d", calls)
	}
	if res.InvalidKind != StreamInvalidSafety {
		t.Fatalf("kind=%v, want safety passthrough", res.InvalidKind)
	}
}

// TestRunTurnWithInvalidRetry_DoomNeverRetries pin：doom flag 恒 Terminate。
func TestRunTurnWithInvalidRetry_DoomNeverRetries(t *testing.T) {
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		// 真实链路中 doom 时 classify 返回 None；此处同时构造「异常 kind +
		// doom flag」验证 helper 依赖 InvalidActionFor 的 override 也安全。
		r := invalidResult(StreamInvalidNone, false, false, true)
		r.InvalidKind = StreamInvalidStreamError
		return r, nil
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("doom must never retry, calls=%d", calls)
	}
	if res.InvalidRetryAttempts != 0 {
		t.Fatalf("attempts=%d, want 0", res.InvalidRetryAttempts)
	}
}

// TestRunTurnWithInvalidRetry_MaxTokensContinueOnce pin：length 截断有内容 →
// 恰好 1 次续写尝试；第二次仍是 length 则终止接受。
func TestRunTurnWithInvalidRetry_MaxTokensContinueOnce(t *testing.T) {
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidMaxTokens, true, false, false), nil
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2 (1 continue attempt)", calls)
	}
	if res.InvalidRetryAttempts != 1 {
		t.Fatalf("attempts=%d, want 1", res.InvalidRetryAttempts)
	}
}

// TestRunTurnWithInvalidRetry_CtxCancelDuringBackoff pin：退避期间父 ctx
// 取消立即穿透，不再发起下一次尝试。
func TestRunTurnWithInvalidRetry_CtxCancelDuringBackoff(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	opts := fastInvalidRetryOpts()
	opts.BackoffBase = 500 * time.Millisecond
	opts.BackoffCap = 500 * time.Millisecond
	calls := 0
	go func() {
		time.Sleep(20 * time.Millisecond)
		cancel()
	}()
	res, err := RunTurnWithInvalidRetry(ctx, func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidNoFinishReason, false, false, false), nil
	}, opts, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("cancel during backoff must stop retries, calls=%d", calls)
	}
	if res.InvalidRetryAttempts != 1 {
		t.Fatalf("attempts=%d, want 1 (committed retry)", res.InvalidRetryAttempts)
	}
}

// TestRunTurnWithInvalidRetry_BeforeRetryAbort pin：前置回调失败放弃重试并
// 返回最近一次结果。
func TestRunTurnWithInvalidRetry_BeforeRetryAbort(t *testing.T) {
	opts := fastInvalidRetryOpts()
	hookErr := errors.New("rollback failed")
	opts.BeforeRetry = func(ctx context.Context, nextAttempt int) error { return hookErr }
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidNoFinishReason, false, false, false), nil
	}, opts, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 1 {
		t.Fatalf("hook abort must stop retries, calls=%d", calls)
	}
	if res.InvalidRetryAttempts != 0 {
		t.Fatalf("attempts=%d, want 0 (retry never committed)", res.InvalidRetryAttempts)
	}
}

// TestRunTurnWithInvalidRetry_EmitsInformationalFlowEvents pin：classified /
// retry / terminated 三类 flow_log 均发出（AS-EVT-01 Informational——可丢失，
// 但接线在时必须 emit）。
func TestRunTurnWithInvalidRetry_EmitsInformationalFlowEvents(t *testing.T) {
	bus := &invalidRetryCaptureBus{}
	opts := fastInvalidRetryOpts()
	opts.Emitter = newInvalidRetryEmitter(bus)
	calls := 0
	_, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidHTTPEOF, false, false, false), nil
	}, opts, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 3 {
		t.Fatalf("calls=%d, want 3", calls)
	}
	counts := bus.stepCounts()
	if counts[StepInvalidClassified] != 3 {
		t.Fatalf("classified events=%d, want 3 (one per attempt)", counts[StepInvalidClassified])
	}
	if counts[StepInvalidRetry] != 2 {
		t.Fatalf("retry events=%d, want 2", counts[StepInvalidRetry])
	}
	if counts[StepInvalidTerminated] != 1 {
		t.Fatalf("terminated events=%d, want 1", counts[StepInvalidTerminated])
	}
}

// TestRunTurnWithInvalidRetry_NilEmitterNoPanic pin：emitter 缺失时静默通过。
func TestRunTurnWithInvalidRetry_NilEmitterNoPanic(t *testing.T) {
	_, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		return invalidResult(StreamInvalidHTTPEOF, false, false, false), nil
	}, fastInvalidRetryOpts(), loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
}

// TestRunTurnWithInvalidRetry_MaxRetriesHardCap pin：opts.MaxRetries 是全表硬帽。
func TestRunTurnWithInvalidRetry_MaxRetriesHardCap(t *testing.T) {
	opts := fastInvalidRetryOpts()
	opts.MaxRetries = 1
	calls := 0
	res, err := RunTurnWithInvalidRetry(context.Background(), func(ctx context.Context, attemptNo int) (EventStreamResult, error) {
		calls++
		return invalidResult(StreamInvalidNoFinishReason, false, false, false), nil
	}, opts, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if calls != 2 {
		t.Fatalf("calls=%d, want 2 (cap=1 retry)", calls)
	}
	if res.InvalidRetryAttempts != 1 {
		t.Fatalf("attempts=%d, want 1", res.InvalidRetryAttempts)
	}
}

// TestInvalidStallReconnects pin：共账递减公式（首次全量、逐次递减、地板 0）。
func TestInvalidStallReconnects(t *testing.T) {
	cases := []struct {
		base, attempt, want int
	}{
		{5, 0, 5}, // 防御：attemptNo<1 视为首次
		{5, 1, 5},
		{5, 2, 4},
		{5, 3, 3},
		{5, 6, 0},
		{5, 7, 0}, // 地板 0 不为负
		{0, 1, 0},
		{-3, 1, 0},
		{2, 2, 1},
		{2, 3, 0},
	}
	for _, tc := range cases {
		if got := InvalidStallReconnects(tc.base, tc.attempt); got != tc.want {
			t.Fatalf("InvalidStallReconnects(%d,%d)=%d, want %d", tc.base, tc.attempt, got, tc.want)
		}
	}
}
