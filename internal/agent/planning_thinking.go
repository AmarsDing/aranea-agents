package agent

import (
	"context"
	"time"

	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	"github.com/google/uuid"
)

// planningThinkingPublisher 将 decompose LLM 调用的推理段（reasoning_content）
// 增量实时发布为 v2 thinking step（created → streaming × N → completed），
// 使用户在任务分解期间看到实时思考流，而不是静止等待（P1b）。
//
// step 挂接到当前 spirit turn（TurnID/TaskID 从 ctx 解析），与该 turn 的
// 其他 step 共用同一 SeqAssigner（Seq=0 由 Sequencer 分配），保证前端按
// Seq 排序时思考块出现在正确位置。AuthorAgentKey 固定为 biz.SpiritAgentKey
// ——规划是 spirit 自身的思考，不属于任何 team member。
//
// 持久化与展示同步：StepCreated/StepCompleted 由 Sequencer 持久化并推 WS，
// StepStreaming 走 16ms 批合并通道（尽力而为持久化 + 同步推送），与 agent
// 运行时思考块共享同一管线。
type planningThinkingPublisher struct {
	seq  v2.SequencerPublisher
	step biz.Step
	ctx  context.Context
}

// newPlanningThinkingPublisher 创建并立即发布 thinking step 的 created 事件。
// seq 为 nil 或 spiritSessionID 为空时返回 nil（调用方需 nil 检查降级）。
func newPlanningThinkingPublisher(seq v2.SequencerPublisher, ctx context.Context, spiritSessionID string) *planningThinkingPublisher {
	if seq == nil || spiritSessionID == "" {
		return nil
	}
	step := biz.Step{
		ID:              "plan-think-" + uuid.NewString(),
		TurnID:          event.TurnIDFromContext(ctx),
		TaskID:          string(RootTaskActivityIDFromCtx(ctx)),
		SessionID:       spiritSessionID,
		SpiritSessionID: spiritSessionID,
		Kind:            biz.StepKindThinking,
		AuthorAgentKey:  biz.SpiritAgentKey,
		Status:          biz.StepStatusRunning,
		StartedAt:       time.Now(),
		Version:         1,
	}
	p := &planningThinkingPublisher{seq: seq, step: step, ctx: ctx}
	p.seq.Publish(ctx, biz.NewStepCreatedEvent(step))
	return p
}

// OnReasoning 匹配 StreamCallbacks.OnReasoning 签名：每个推理段增量发布一条
// StepStreamingEvent（DeltaField=reasoning）。
func (p *planningThinkingPublisher) OnReasoning(piece string) error {
	if p == nil || piece == "" {
		return nil
	}
	p.seq.Publish(p.ctx, biz.NewStepStreamingEvent(p.step.SpiritSessionID, p.step.TaskID, p.step.ID, "reasoning", piece))
	return nil
}

// Complete 以 completed 终态闭合思考 step，携带全量推理文本。
func (p *planningThinkingPublisher) Complete(finalReasoning string) {
	p.finish(biz.StepStatusCompleted, finalReasoning)
}

// Fail 以 failed 终态闭合思考 step（分解失败时防止前端思考块永远转圈）。
func (p *planningThinkingPublisher) Fail() {
	p.finish(biz.StepStatusFailed, "")
}

func (p *planningThinkingPublisher) finish(status biz.StepStatus, finalReasoning string) {
	if p == nil {
		return
	}
	now := time.Now()
	p.step.Status = status
	p.step.Reasoning = finalReasoning
	p.step.CompletedAt = &now
	p.step.Version++
	p.seq.Publish(p.ctx, biz.NewStepCompletedEvent(p.step))
}
