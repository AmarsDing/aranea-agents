package service

import (
	"context"
	"fmt"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"
)

// NOTE(Phase3b-D Task 10): PrePlanningGate migrated from v1 ActivityEventBus
// to v2 EventBus. publishPlanningPhase now emits biz.NewStepCreatedEvent
// (Kind=StepKindNotice). DATA LOSS: v2 Step has no Meta field, so phase/
// duration_ms/session_id/source carried in v1 Activity.Meta are dropped.
// The message content is preserved as Step.Content.

// GateDecision is the result of the pre-planning gate evaluation (P1-2).
// When ForcePlanning is true, the caller must route the turn through the
// plan_and_execute path instead of letting the Spirit LLM decide.
type GateDecision struct {
	Level          biz.ComplexityLevel
	Score          float64
	ForcePlanning  bool
	Reason         string
	IntentArtifact *biz.IntentArtifact
}

// PrePlanningGate evaluates task complexity before the Spirit LLM runs,
// forcing the planning path for Moderate/Complex tasks (P1-2).
//
// The gate runs a pure-computation QuickAssess (no LLM, no DB) so it adds
// negligible latency (<1ms) to the turn. Planning timeline events are
// published so the frontend can render the assessment phase.
//
// Phase 3b-D Task 10: migrated from v1 ActivityEventBus to v2 EventBus.
type PrePlanningGate struct {
	planner  biz.TaskPlannerPort
	eventBus biz.EventBus
	seq      rt.EventPublisher
	lg       loggateway.Logger
}

// NewPrePlanningGate constructs a PrePlanningGate.
func NewPrePlanningGate(planner biz.TaskPlannerPort, eventBus biz.EventBus, seq rt.EventPublisher, lg loggateway.Logger) *PrePlanningGate {
	return &PrePlanningGate{
		planner:  planner,
		eventBus: eventBus,
		seq:      seq,
		lg:       lg.With(loggateway.Domain("pre-planning-gate")),
	}
}

// Evaluate runs the quick complexity assessment and returns a gate decision.
// The intent artifact from the intent pass (if available) is passed through
// to the planner for a more accurate assessment.
//
// 2026-07-28: 不再发布评估阶段通知到前端。这些是内部状态变化，对用户无意义，
// 且会污染会话时间线。保留日志记录以便调试。
func (g *PrePlanningGate) Evaluate(ctx context.Context, input biz.PlanInput) (GateDecision, error) {
	// 内部日志记录，不发布到前端
	g.lg.Debug("开始复杂度评估",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("spirit_session_id", input.SpiritSessionID),
	)

	level, score, err := g.planner.QuickAssess(ctx, input)
	if err != nil {
		g.lg.Warn("QuickAssess 失败",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("spirit_session_id", input.SpiritSessionID),
			loggateway.Err(err),
		)
		return GateDecision{}, err
	}

	forcePlanning := level == biz.ComplexityModerate || level == biz.ComplexityComplex
	// 2026-07-28：simple 决策改中性表述——门控只陈述评估结论，不承诺"直接回答"
	// （LLM 对 simple 任务仍可能自主调用 plan_and_execute，旧文案与实际行为矛盾）。
	reason := fmt.Sprintf("评估完成：%s任务", complexityLevelZh(level))
	if forcePlanning {
		reason = fmt.Sprintf("评估完成：%s任务，强制走规划路径", complexityLevelZh(level))
	}
	// 词法事实查询（LooksLikeFactQuery）是轻档路由信号，不是 LLM/QuickAssess
	// 分类豁免：该函数已对 HasTaskActionSignal 一票否决（「核对天气并生成报告」
	// 仍强制规划）。ADR-79-V V2 约束的是评分器/意图分类器不得免除义务；此处
	// 与 ClassifyTaskGear.FactQuery 同源，避免天气/时点问询空跑一轮
	// plan_and_execute。工具边界 shouldRejectFactQueryPlan 仍作纵深拦截。
	if forcePlanning && biz.LooksLikeFactQuery(input.UserMessage) {
		forcePlanning = false
		reason = fmt.Sprintf("评估完成：%s任务（事实查询，不强制规划）", complexityLevelZh(level))
	}
	// 会话阶段抑制（ShouldForcePlanning，基于已持久化团队的事实证据）在
	// chat_orchestrator_turn.go 消费侧叠加。

	decision := GateDecision{
		Level:          level,
		Score:          score,
		ForcePlanning:  forcePlanning,
		Reason:         reason,
		IntentArtifact: input.IntentArtifact,
	}

	// 内部日志记录，不发布到前端
	g.lg.Info("预规划门控决策",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("spirit_session_id", input.SpiritSessionID),
		loggateway.Str("complexity_level", string(level)),
		loggateway.Float64("complexity_score", score),
		loggateway.Bool("force_planning", forcePlanning),
		loggateway.Str("reason", reason),
	)

	return decision, nil
}

// complexityLevelZh returns the Chinese label for a complexity level.
// UI-facing notice text must not leak English enum values (moderate/complex).
func complexityLevelZh(level biz.ComplexityLevel) string {
	switch level {
	case biz.ComplexityModerate:
		return "中等"
	case biz.ComplexityComplex:
		return "复杂"
	default:
		return "简单"
	}
}

// publishPlanningPhase 已不再使用。2026-07-28 起，预规划门控不再发布评估阶段通知到前端，
// 这些是内部状态变化，对用户无意义，且会污染会话时间线。保留此方法仅为兼容性考虑，
// 实际已不再调用。
//
// 原设计 rationale (B.4.3): the pre-planning gate's "assess" phase is a
// complexity assessment, NOT task decomposition. The PlanBlock component
// is reserved for actual task plans (Kind=plan with steps). Emitting an
// assess event as Kind=plan created a spurious PlanBlock that appeared
// before the real plan ("UI 提前占位" issue) and competed with the real
// plan for visual space. Routing to Kind=notice renders it as a NoticeBlock
// banner instead, leaving the plan slot exclusively for actual plans.
//
// Phase 3b-D Task 10: migrated from v1 ActivityEvent to v2 StepCreatedEvent.
// The original v1 Meta (phase/duration_ms/session_id/source) is dropped
// because v2 Step has no Meta field. The message is preserved as Step.Content,
// and the status is mapped to StepStatus. noticeType is preserved as
// Step.NoticeType for NoticeBlock rendering.
//
// 2026-07-27: taskID 挂接本 turn 所属根 Task（根 turn 预生成 ID），notice 经
// TaskCard orphanNoticeSteps 渲染为任务 footer；此前无 TaskID 的 notice 是
// session 级孤儿步骤，前端永不渲染且污染 DB。
func (g *PrePlanningGate) publishPlanningPhase(ctx context.Context, status biz.ActivityStatus, message, spiritSessionID, taskID string) {
	// 方法保留但不再使用
}
