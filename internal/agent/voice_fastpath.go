package agent

import (
	"context"

	"aranea-agents/internal/agent/callbacks"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// Voice Fast-Path（2026-08-09）：语音轮次（TurnInput.Voice != nil）的主 LLM
// 调用在请求时关闭 thinking。BUILD 产物跨入口缓存共享（cache key 不含入口），
// 思考开关只能经 ctx 标记 + BeforeModel 回调在请求级改写，不得烘进构建期。
//
// 真机实测：deepseek-v4-flash 服务端默认开思考，语音 TTFT 2.5-5.6s；
// 显式 disabled 后预期 <1s。planner/compress 等深度推理调用点不经过本标记，
// 保留 provider 服务端默认（规划质量不受影响）。

type voiceFastPathKey struct{}
type thinkingDisabledKey struct{}

// WithVoiceFastPath 标记当前 ctx 为语音快速通道轮次。
func WithVoiceFastPath(ctx context.Context) context.Context {
	return context.WithValue(ctx, voiceFastPathKey{}, true)
}

// VoiceFastPathFromContext 报告当前 ctx 是否语音快速通道轮次。
func VoiceFastPathFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(voiceFastPathKey{}).(bool)
	return v
}

// WithThinkingDisabled marks this request to turn off model thinking
// (DirectReply / simple skip-intent turns). BUILD 产物跨入口缓存共享，
// 思考开关只能经 ctx + BeforeModel 在请求级改写。
func WithThinkingDisabled(ctx context.Context) context.Context {
	return context.WithValue(ctx, thinkingDisabledKey{}, true)
}

// ThinkingDisabledFromContext reports whether this request must disable thinking.
func ThinkingDisabledFromContext(ctx context.Context) bool {
	v, _ := ctx.Value(thinkingDisabledKey{}).(bool)
	return v
}

// newVoiceFastPathBeforeHook 在语音轮次或闲聊快路径把
// GenerationConfig.ThinkingEnabled 显式置 false（deepseek variant 映射为
// {"thinking":{"type":"disabled"}}）；其余轮次保持 nil（provider 服务端默认）。
// LayerDynamic：标记来自 per-request ctx。
func newVoiceFastPathBeforeHook() callbacks.Callback {
	return callbacks.NewBeforeModelHook(4, callbacks.LayerDynamic, func(ctx context.Context, args *trpcmodel.BeforeModelArgs) (*trpcmodel.BeforeModelResult, error) {
		if args == nil || args.Request == nil {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		if !VoiceFastPathFromContext(ctx) && !ThinkingDisabledFromContext(ctx) {
			return &trpcmodel.BeforeModelResult{Context: ctx}, nil
		}
		disabled := false
		args.Request.GenerationConfig.ThinkingEnabled = &disabled
		return &trpcmodel.BeforeModelResult{Context: ctx}, nil
	})
}
