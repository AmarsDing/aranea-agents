package agent

import (
	"context"
	"encoding/json"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// spirit_usage_record.go — M83 LBG-6：spirit 编排面 LLM 调用（planner 分解 /
// allocator 冷启动匹配）的用量落账。
//
// 背景：这些调用走 CallOpenAICompatChat(Stream) 直连，token 消耗此前被 `_`
// 丢弃、完全未落账——成本核算存在盲区，effort 路由决策也不可见。本文件提供
// 统一的 best-effort 落账助手，复用既有 RecordAuxLLMUsage 管道（不新建管道），
// effort 落入 metadata_json["effort"]（jsonb 键追加，无 schema 变更）。
//
// 红线约束：
//   - best-effort：落账失败仅 Warn 日志，绝不阻断编排主链路。
//   - 零 token 跳过：失败调用（err 时 CallOpenAICompatChat 返回 0 token）不产生
//     空行，与 team recordAuxUsage 口径一致。
//   - 每次重试尝试独立落账：每次尝试都是一次真实计费调用。
//   - recorder 为 nil（旧测试构造路径）时整体跳过，行为与旧版一致。

// SpiritAuxUsageRecorder 是 spirit 编排面用量落账的最小端口（消费方定义）。
// 生产实现为 UsageUsecase（经 wire 迟绑定注入）。
type SpiritAuxUsageRecorder interface {
	RecordAuxLLMUsage(ctx context.Context, in biz.AuxLLMUsageInput) error
}

// SpiritAuxUsageRecorderFunc 把函数适配为 SpiritAuxUsageRecorder（wire 迟绑定
// 用，与 subagent.UsageRecorderFunc 同模式）。
type SpiritAuxUsageRecorderFunc func(ctx context.Context, in biz.AuxLLMUsageInput) error

// RecordAuxLLMUsage implements SpiritAuxUsageRecorder.
func (f SpiritAuxUsageRecorderFunc) RecordAuxLLMUsage(ctx context.Context, in biz.AuxLLMUsageInput) error {
	return f(ctx, in)
}

// recordSpiritAuxUsage best-effort 落账一次 spirit 编排 LLM 调用。
// 调用方负责填写 in 的业务字段；本函数只负责守卫与错误收敛。
func recordSpiritAuxUsage(ctx context.Context, rec SpiritAuxUsageRecorder, lg loggateway.Logger, stepID string, in biz.AuxLLMUsageInput) {
	if rec == nil {
		return
	}
	if in.PromptTok <= 0 && in.CompletionTok <= 0 {
		return
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	// 编排 ctx 可能在长调用后临近取消；落账用独立 deadline（与既有
	// recordChatIngressUsage / team 落账同一模式）。
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if err := rec.RecordAuxLLMUsage(recCtx, in); err != nil {
		lg.Warn("spirit 编排用量落库失败",
			loggateway.StepID(stepID),
			loggateway.Err(err),
			loggateway.Str("usage_kind", in.Kind),
			loggateway.Str("effort", in.Effort),
		)
	}
}

// spiritAuxMeta 构建落账 metadata 基座：ctx 上有 TraceEmitter 时复用其
// trace 关联（与 team recordMemberUsage 同模式），否则退化为 source 标记。
func spiritAuxMeta(ctx context.Context, source string) string {
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		return em.MetadataJSON()
	}
	b, _ := json.Marshal(map[string]any{"source": source})
	return string(b)
}

// spiritAuxCallStatus 把调用错误映射为 usage 行状态。
func spiritAuxCallStatus(err error) string {
	if err != nil {
		return "failed"
	}
	return biz.TokenUsageStatusSuccess
}

// spiritAuxErrMsg 提取错误消息（nil-safe）。
func spiritAuxErrMsg(err error) string {
	if err == nil {
		return ""
	}
	return err.Error()
}
