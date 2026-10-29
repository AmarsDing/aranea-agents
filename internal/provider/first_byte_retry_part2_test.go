package provider

import (
	"aranea-agents/pkg/loggateway"
	"context"
	"sync/atomic"
	"testing"
	"time"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

func TestLivenessGuard_LongHealthyStreamNotKilled(t *testing.T) {
	inner := &slowHealthyModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "openai", "gpt-4o", loggateway.NewNoop())

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

	ctx := WithStallBudget(context.Background(), 5*time.Second, 2)
	p = stallPolicyFromContext(ctx)
	if p.timeout != 5*time.Second || p.maxReconnects != 2 {
		t.Fatalf("injected = (%v, %d), want (5s, 2)", p.timeout, p.maxReconnects)
	}

	ctx = WithStallBudget(context.Background(), 0, -1)
	p = stallPolicyFromContext(ctx)
	if p.timeout != DefaultStallTimeout || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("non-positive fields = (%v, %d), want defaults", p.timeout, p.maxReconnects)
	}
}

// --- 2026-10-29 每模型兜底默认值（目录 config_json 下沉）测试 ---

// 优先级钉死：ctx 注入 > 每模型兜底 > 包默认值；首字节维度无注入且无兜底
// 时保持「不监控」旧语义（0）。
func TestLivenessGuardFallbacks_Resolution(t *testing.T) {
	// (a) 无 ctx 注入 + 兜底已配 → 用兜底。
	m := &livenessGuardModel{opts: LivenessGuardOptions{
		FirstByteTimeoutFallback:   300 * time.Second,
		StallTimeoutFallback:       180 * time.Second,
		StallMaxReconnectsFallback: 2,
	}}
	if got := m.firstByteBudget(context.Background()); got != 300*time.Second {
		t.Fatalf("fallback first-byte = %v, want 300s", got)
	}
	if p := m.stallPolicy(context.Background()); p.timeout != 180*time.Second || p.maxReconnects != 2 {
		t.Fatalf("fallback stall = (%v, %d), want (180s, 2)", p.timeout, p.maxReconnects)
	}

	// (b) ctx 注入 + 兜底已配 → ctx 优先。
	ctx := WithFirstByteRetryBudget(context.Background(), 5*time.Second)
	ctx = WithStallBudget(ctx, 7*time.Second, 3)
	if got := m.firstByteBudget(ctx); got != 5*time.Second {
		t.Fatalf("ctx first-byte = %v, want 5s (ctx wins over fallback)", got)
	}
	if p := m.stallPolicy(ctx); p.timeout != 7*time.Second || p.maxReconnects != 3 {
		t.Fatalf("ctx stall = (%v, %d), want (7s, 3) (ctx wins over fallback)", p.timeout, p.maxReconnects)
	}

	// (c) 两者皆无 → 包默认值；首字节维度保持「不监控」旧语义（0）。
	plain := &livenessGuardModel{}
	if got := plain.firstByteBudget(context.Background()); got != 0 {
		t.Fatalf("no-budget first-byte = %v, want 0 (dimension unmonitored)", got)
	}
	if p := plain.stallPolicy(context.Background()); p.timeout != DefaultStallTimeout || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("no-budget stall = (%v, %d), want package defaults", p.timeout, p.maxReconnects)
	}

	// (d) 兜底为零值 → 包默认值，与历史行为完全一致。
	zero := &livenessGuardModel{opts: LivenessGuardOptions{}}
	if got := zero.firstByteBudget(context.Background()); got != 0 {
		t.Fatalf("zero-fallback first-byte = %v, want 0", got)
	}
	if p := zero.stallPolicy(context.Background()); p.timeout != DefaultStallTimeout || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("zero-fallback stall = (%v, %d), want package defaults", p.timeout, p.maxReconnects)
	}

	// 单字段兜底：另一字段落包默认值。
	partial := &livenessGuardModel{opts: LivenessGuardOptions{StallTimeoutFallback: 45 * time.Second}}
	if p := partial.stallPolicy(context.Background()); p.timeout != 45*time.Second || p.maxReconnects != DefaultStallMaxReconnects {
		t.Fatalf("partial-fallback stall = (%v, %d), want (45s, %d)", p.timeout, p.maxReconnects, DefaultStallMaxReconnects)
	}
}

// 无 ctx 注入（辅助调用链形态）：兜底首字节预算必须驱动重连。
func TestLivenessGuard_FallbackFirstByteBudgetReconnects(t *testing.T) {
	inner := &stallThenAnswerModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop(),
		LivenessGuardOptions{FirstByteTimeoutFallback: 50 * time.Millisecond})

	out, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case resp, ok := <-out:
		if !ok || resp == nil {
			t.Fatal("expected retried response, got closed/empty channel")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no response — fallback first-byte reconnect did not fire")
	}
	if got := atomic.LoadInt32(&inner.calls); got != 2 {
		t.Fatalf("inner calls = %d, want 2 (stall + one reconnect via fallback)", got)
	}
}

// ctx 注入 10s 首字节预算 ≫ 兜底 50ms：ctx 优先，兜底不得提前开火；
// 300ms 的 ctx 截止先关闭 attempt，期间不得发生重连。
func TestLivenessGuard_CtxBudgetOverridesFallback(t *testing.T) {
	inner := &stallThenAnswerModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop(),
		LivenessGuardOptions{FirstByteTimeoutFallback: 50 * time.Millisecond})

	ctx, cancel := context.WithTimeout(WithFirstByteRetryBudget(context.Background(), 10*time.Second), 300*time.Millisecond)
	defer cancel()
	out, err := wrapped.GenerateContent(ctx, &trpcmodel.Request{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	select {
	case <-out:
	case <-time.After(2 * time.Second):
		t.Fatal("channel not closed after ctx deadline")
	}
	if got := atomic.LoadInt32(&inner.calls); got != 1 {
		t.Fatalf("inner calls = %d, want 1 (ctx budget wins, fallback must not fire)", got)
	}
}

// 无 ctx 注入：兜底 stall 预算必须驱动中段静默重连。
func TestLivenessGuard_FallbackStallPolicyReconnects(t *testing.T) {
	inner := &midStallThenAnswerModel{}
	wrapped := WrapModelWithLivenessGuard(inner, "deepseek", "deepseek-v4-flash", loggateway.NewNoop(),
		LivenessGuardOptions{StallTimeoutFallback: 50 * time.Millisecond, StallMaxReconnectsFallback: 3})

	out, err := wrapped.GenerateContent(context.Background(), &trpcmodel.Request{})
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
			t.Fatal("stream hung — fallback stall reconnect did not fire")
		}
	}
drained:
	if len(got) != 3 {
		t.Fatalf("forwarded %d events, want 3 (half attempt1 + full attempt2)", len(got))
	}
	if last := got[len(got)-1]; !last.Done {
		t.Fatal("last event must be attempt2's Done response")
	}
	if n := atomic.LoadInt32(&inner.calls); n != 2 {
		t.Fatalf("inner calls = %d, want 2 (stall + one reconnect via fallback)", n)
	}
}
