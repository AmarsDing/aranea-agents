package agent

import (
	"context"
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// Voice Fast-Path（2026-08-09）：语音轮次主 LLM 必须 per-request 关闭 thinking——
// BUILD 产物（含回调链）跨入口缓存共享，思考开关不能烘进 cache key，
// 只能在请求时按 ctx 标记改写 GenerationConfig。真机实测 deepseek-v4-flash
// 服务端默认开思考，语音 TTFT 2.5-5.6s，关闭后应 <1s。
func TestVoiceFastPathBeforeHook_DisablesThinkingWhenMarked(t *testing.T) {
	hook := newVoiceFastPathBeforeHook()
	if hook == nil {
		t.Fatal("hook must be non-nil")
	}
	fn, ok := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if !ok {
		t.Fatal("hook must implement HandleBeforeModel")
	}

	ctx := WithVoiceFastPath(context.Background())
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{}}
	if _, err := fn.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	te := args.Request.GenerationConfig.ThinkingEnabled
	if te == nil || *te != false {
		t.Fatalf("voice fast path must set ThinkingEnabled=false, got %v", te)
	}
}

func TestVoiceFastPathBeforeHook_UntouchedWithoutMarker(t *testing.T) {
	hook := newVoiceFastPathBeforeHook()
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{}}
	if _, err := fn.HandleBeforeModel(context.Background(), args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if args.Request.GenerationConfig.ThinkingEnabled != nil {
		t.Fatal("non-voice turn must keep provider default (nil ThinkingEnabled)")
	}
}

func TestVoiceFastPathBeforeHook_DisablesThinkingForDirectReply(t *testing.T) {
	hook := newVoiceFastPathBeforeHook()
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	ctx := WithThinkingDisabled(context.Background())
	args := &trpcmodel.BeforeModelArgs{Request: &trpcmodel.Request{}}
	if _, err := fn.HandleBeforeModel(ctx, args); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	te := args.Request.GenerationConfig.ThinkingEnabled
	if te == nil || *te != false {
		t.Fatalf("direct-reply fast path must set ThinkingEnabled=false, got %v", te)
	}
}

func TestVoiceFastPathBeforeHook_NilArgsSafe(t *testing.T) {
	hook := newVoiceFastPathBeforeHook()
	fn := hook.(interface {
		HandleBeforeModel(context.Context, *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error)
	})
	if _, err := fn.HandleBeforeModel(WithVoiceFastPath(context.Background()), &trpcmodel.BeforeModelArgs{}); err != nil {
		t.Fatalf("nil request must be safe: %v", err)
	}
}
