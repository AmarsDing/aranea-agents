package biz

import "context"

// graph_usage.go — 图执行（独立 Graph run，含 twinmonitor 门面触发的运行）的
// LLM 用量落账端口。
//
// 背景：独立图执行此前完全不落账——聊天轮次有 chat_turn、团队运行有
// team_member/team_turn、辅助调用有 aux_* 系列，唯独 graph 运行时内 agent
// 节点的 LLM 调用token 消耗不可见，导致 twinmonitor 任务详情
// （model_used/tokens）与成本核算双双盲区。本文件定义消费方端口；
// 适配层（internal/graph/adapter）在事件泵中累计框架事件 Response.Usage
// 并按 (Response.ID) 去重，流结束时经本端口 best-effort 落账。
//
// 红线约束：
//   - best-effort：落账失败仅 Warn 日志，绝不阻断图执行主链路。
//   - 零 token 跳过：不产生空行。
//   - 团队运行的图执行已有 team_member/team_turn 落账，消费方实现必须
//     按会话归属过滤（如 twin- 前缀），避免与团队路径双重计费。

// GraphRunUsageInput 一次图运行的聚合用量（按模型拆分后逐条落账）。
type GraphRunUsageInput struct {
	// SessionID 执行会话（twin 运行为 "twin-<uuid>"），作为落账行的
	// SessionID 供按会话聚合查询（handleGetRun 回填）。
	SessionID string
	// RunID 执行 ID（execID），落账时写入 MessageID 供交叉引用。
	RunID string
	// GraphID 图 ID，仅用于日志定位。
	GraphID string
	// Model 提供方上报的模型名（Response.Model）；未知时为空串。
	Model string
	// PromptTok/CompletionTok 该模型在本次运行内的聚合 token。
	PromptTok     int
	CompletionTok int
	// Status 运行终态（success/failed/cancelled 之一）。
	Status string
	// LLMCalls 该模型的计费调用次数（去重后的 Response.ID 数）。
	LLMCalls int
	// UsageSource 计量来源（"streaming"）。
	UsageSource string
	// MetadataJSON 附加元数据（可空，默认 "{}"）。
	MetadataJSON string
}

// GraphUsageRecorder 图运行用量落账端口（消费方定义，与
// SpiritAuxUsageRecorder 同模式）。生产实现为 twin 门面内按 twin- 会话
// 前缀过滤的 usage.Usecase 包装。
type GraphUsageRecorder interface {
	RecordGraphRunUsage(ctx context.Context, in GraphRunUsageInput) error
}

// GraphUsageRecorderFunc 函数适配器（测试/迟绑定用）。
type GraphUsageRecorderFunc func(ctx context.Context, in GraphRunUsageInput) error

// RecordGraphRunUsage implements GraphUsageRecorder.
func (f GraphUsageRecorderFunc) RecordGraphRunUsage(ctx context.Context, in GraphRunUsageInput) error {
	return f(ctx, in)
}

// graphUsageRecorderSetter 是 GraphRunnerFactory 实现的可选能力：把图运行
// 用量落账器注入此后构建的全部 runtime（ExecuteGraph 与 Resume 共用同一
// factory，注入一次即可覆盖两条路径）。与 GraphRunEventSinkOutput 同为
// 可选接口模式，不改动 GraphRunnerFactory 签名。
type graphUsageRecorderSetter interface {
	SetGraphUsageRecorder(rec GraphUsageRecorder)
}

// SetGraphUsageRecorder 注册图运行用量落账器。可选；nil 或未实现该能力
// 的 factory 静默跳过（行为与旧版一致，零行为变更）。
// 装配期（wire 组装、服务接受流量前）调用一次。
func (uc *GraphExecutionUsecase) SetGraphUsageRecorder(rec GraphUsageRecorder) {
	if uc == nil || rec == nil {
		return
	}
	if setter, ok := uc.factory.(graphUsageRecorderSetter); ok {
		setter.SetGraphUsageRecorder(rec)
	}
}
