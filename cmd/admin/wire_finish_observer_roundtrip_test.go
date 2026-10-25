package main

// wire_finish_observer_roundtrip_test.go —— Q5 批 D3-f 对抗评审 C3 修复批
// 钉板：接线静默丢失不可再绿过。被钉面=唯一穿透函数
// newObservedFinishRoundTrip（wire.go 主本 + wire_gen.go 手镜共四调用点）：
// 任一调用点退回自造字面量并删 OnAbnormalFinish 键值 → 编译绿但钩子面
// 静默丢失 → 重构回归必有人同步本钉板的函数契约。删掉函数内字段 → 本钉
// 板直接红。

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/provider"
)

func TestNewObservedFinishRoundTrip_HookPassesThroughAndTimeoutKept(t *testing.T) {
	var called int32
	hooks := provider.FinishObserverHooks{
		OnAbnormalFinish: func(context.Context, provider.FinishObservation) { atomic.AddInt32(&called, 1) },
	}
	rt := newObservedFinishRoundTrip(provider.LLMLeakGuardTimeout, hooks)
	if rt == nil || rt.HTTP == nil {
		t.Fatal("RoundTrip / HTTP client must be non-nil")
	}
	if rt.HTTP.Timeout != provider.LLMLeakGuardTimeout {
		t.Errorf("timeout = %v, want provider.LLMLeakGuardTimeout（参数面原样承接）", rt.HTTP.Timeout)
	}
	if rt.OnAbnormalFinish == nil {
		t.Fatal("OnAbnormalFinish must be installed from hooks（接线缺失即观测静默全灭）")
	}
	rt.OnAbnormalFinish(context.Background(), provider.FinishObservation{})
	if atomic.LoadInt32(&called) != 1 {
		t.Fatalf("installed hook must be callable exactly once per call, called=%d", called)
	}
}

func TestNewObservedFinishRoundTrip_ZeroHooksStaysNil(t *testing.T) {
	rt := newObservedFinishRoundTrip(120*time.Second, provider.FinishObserverHooks{})
	if rt.OnAbnormalFinish != nil {
		t.Error("zero hooks must yield nil OnAbnormalFinish（pass-through 设计语义）")
	}
	if rt.HTTP.Timeout != 120*time.Second {
		t.Errorf("timeout = %v, want 120s（builder 面存量差异参数化承接）", rt.HTTP.Timeout)
	}
}
