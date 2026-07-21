package service

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
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
func (g *PrePlanningGate) Evaluate(ctx context.Context, input biz.PlanInput) (GateDecision, error) {
	g.publishPlanningPhase(ctx, biz.ActivityStatusRunning, "开始复杂度评估", input.SpiritSessionID)

	level, score, err := g.planner.QuickAssess(ctx, input)
	if err != nil {
		g.lg.Warn("QuickAssess 失败",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("spirit_session_id", input.SpiritSessionID),
			loggateway.Err(err),
		)
		g.publishPlanningPhase(ctx, biz.ActivityStatusFailed, "复杂度评估失败", input.SpiritSessionID)
		return GateDecision{}, err
	}

	forcePlanning := level == biz.ComplexityModerate || level == biz.ComplexityComplex
	reason := "简单任务，直接回答"
	if forcePlanning {
		reason = fmt.Sprintf("%s任务，强制走规划路径", level)
	}

	decision := GateDecision{
		Level:          level,
		Score:          score,
		ForcePlanning:  forcePlanning,
		Reason:         reason,
		IntentArtifact: input.IntentArtifact,
	}

	g.lg.Info("预规划门控决策",
		loggateway.StepID(biz.SpiritStepPlannerAssess),
		loggateway.Str("spirit_session_id", input.SpiritSessionID),
		loggateway.Str("complexity_level", string(level)),
		loggateway.Float64("complexity_score", score),
		loggateway.Bool("force_planning", forcePlanning),
	)

	g.publishPlanningPhase(ctx, biz.ActivityStatusCompleted, reason, input.SpiritSessionID)

	return decision, nil
}

// publishPlanningPhase publishes a complexity-assessment timeline event as a
// v2 StepCreatedEvent (Kind=StepKindNotice).
//
// Design rationale (B.4.3): the pre-planning gate's "assess" phase is a
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
func (g *PrePlanningGate) publishPlanningPhase(ctx context.Context, status biz.ActivityStatus, message, spiritSessionID string) {
	if g.seq == nil && g.eventBus == nil {
		return
	}
	// Map assess status → notice type for NoticeBlock rendering.
	// 2026-07-21 P1-5 F3：直发 notice 无后续更新事件，必须自终态；
	// 此前 running 的起始事件在 DB 中永久残留为僵尸步骤。
	noticeType := "info"
	stepStatus := biz.StepStatusCompleted
	switch status {
	case biz.ActivityStatusFailed:
		noticeType = "warning"
		stepStatus = biz.StepStatusFailed
	case biz.ActivityStatusCompleted:
		noticeType = "success"
	}
	now := time.Now()
	step := biz.Step{
		ID:              uuid.NewString(),
		SessionID:       spiritSessionID,
		SpiritSessionID: spiritSessionID,
		Kind:            biz.StepKindNotice,
		NoticeType:      noticeType,
		Content:         message,
		Status:          stepStatus,
		StartedAt:       now,
		CompletedAt:     &now,
		Version:         1,
		AuthorAgentKey:  "pre-planning-gate",
	}
	ev := biz.NewStepCreatedEvent(step)
	if g.seq != nil {
		g.seq.Publish(ctx, ev)
		return
	}
	g.eventBus.Publish(ctx, ev)
}
