package runtime

// deps_finish_observer_test.go —— Q5 批 D3 漏斗钉板（94 三件套）：
//  1. LLMFinishObserver 经 RoundTrip/RoundTripForSession 透挂到
//     rt.OnAbnormalFinish；零值 hooks=pass-through（零行为变更）。
//  2. session 绑定在漏斗一次性完成——wrapper 侧恒空 session、emit 侧不
//     线程化 session 装配；且绑定发生在 EventBus 早退分支之前（决策记录
//     的 session 归属不依赖事件总线装配与否）。

import (
	"context"
	"net/http"
	"testing"

	"aranea-agents/internal/provider"
)

func TestRoundTrip_FinishObserverHookPassThrough(t *testing.T) {
	called := 0
	hooks := provider.FinishObserverHooks{
		OnAbnormalFinish: func(context.Context, provider.FinishObservation) { called++ },
	}
	d := TurnDeps{LLMHTTP: &http.Client{}, LLMFinishObserver: hooks}
	rt := d.RoundTrip()
	if rt.OnAbnormalFinish == nil {
		t.Fatal("RoundTrip must carry LLMFinishObserver.OnAbnormalFinish")
	}
	rt.OnAbnormalFinish(context.Background(), provider.FinishObservation{FinishReason: "length"})
	if called != 1 {
		t.Fatalf("hook called %d times, want 1", called)
	}
}

func TestRoundTrip_NilFinishObserverPassThrough(t *testing.T) {
	d := TurnDeps{}
	if rt := d.RoundTrip(); rt.OnAbnormalFinish != nil {
		t.Fatal("零值 hooks 必须 pass-through（OnAbnormalFinish 保持 nil）")
	}
	if rt := d.RoundTripForSession("s-1"); rt.OnAbnormalFinish != nil {
		t.Fatal("零值 hooks 的 RoundTripForSession 同样 pass-through")
	}
}

func TestRoundTripForSession_StampsSessionOnceAtFunnel(t *testing.T) {
	var got provider.FinishObservation
	calls := 0
	hooks := provider.FinishObserverHooks{
		OnAbnormalFinish: func(_ context.Context, o provider.FinishObservation) {
			got = o
			calls++
		},
	}
	// EventBus nil 早退分支：hook 绑定不依赖事件总线装配（钉 D3 设计条款）。
	d := TurnDeps{LLMHTTP: &http.Client{}, LLMFinishObserver: hooks}
	rt := d.RoundTripForSession("sess-42")
	if rt.OnAbnormalFinish == nil {
		t.Fatal("hook must survive the EventBus-nil early return")
	}
	rt.OnAbnormalFinish(context.Background(), provider.FinishObservation{
		SawFinishReason: true, FinishReason: "length",
	})
	if calls != 1 {
		t.Fatalf("hook called %d times, want 1", calls)
	}
	if got.SessionID != "sess-42" {
		t.Errorf("SessionID = %q, want sess-42（漏斗一次性绑定）", got.SessionID)
	}
	if got.FinishReason != "length" || !got.SawFinishReason {
		t.Errorf("原始事实必须保留, got finish=(%v,%q)", got.SawFinishReason, got.FinishReason)
	}
}
