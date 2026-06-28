package service

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

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
type PrePlanningGate struct {
	planner     biz.TaskPlannerPort
	activityBus biz.ActivityEventBus
	lg          loggateway.Logger
}

// NewPrePlanningGate constructs a PrePlanningGate.
func NewPrePlanningGate(planner biz.TaskPlannerPort, activityBus biz.ActivityEventBus, lg loggateway.Logger) *PrePlanningGate {
	return &PrePlanningGate{
		planner:     planner,
		activityBus: activityBus,
		lg:          lg.With(loggateway.Domain("pre-planning-gate")),
	}
}

// Evaluate runs the quick complexity assessment and returns a gate decision.
// The intent artifact from the intent pass (if available) is passed through
// to the planner for a more accurate assessment.
func (g *PrePlanningGate) Evaluate(ctx context.Context, input biz.PlanInput) (GateDecision, error) {
	start := time.Now()
	g.publishPlanningPhase(ctx, biz.ActivityEventCreated, biz.ActivityStatusRunning, "assess", "开始复杂度评估", input.SpiritSessionID, 0)

	level, score, err := g.planner.QuickAssess(ctx, input)
	if err != nil {
		g.lg.Warn("QuickAssess 失败",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("spirit_session_id", input.SpiritSessionID),
			loggateway.Err(err),
		)
		g.publishPlanningPhase(ctx, biz.ActivityEventFailed, biz.ActivityStatusFailed, "assess", "复杂度评估失败", input.SpiritSessionID, time.Since(start).Seconds()*1000)
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

	g.publishPlanningPhase(ctx, biz.ActivityEventCompleted, biz.ActivityStatusCompleted, "assess", reason, input.SpiritSessionID, time.Since(start).Seconds()*1000)

	return decision, nil
}

// publishPlanningPhase publishes a planning timeline event as an ActivityEvent
// (Kind=plan, Domain=chat). Replaces the legacy EnvelopeTypePlanningPhase* publish.
// In the Spirit direct-run scenario the caller passes input.SpiritSessionID,
// which equals the current SessionID (Spirit session is the root). Both
// SessionID and SpiritSessionID are set to the same value for cross-session
// aggregation. Bus layer normalizes empty SpiritSessionID by falling back
// to SessionID (design doc B.6.2).
func (g *PrePlanningGate) publishPlanningPhase(ctx context.Context, eventType biz.ActivityEventType, status biz.ActivityStatus, phase, message, spiritSessionID string, durationMs float64) {
	if g.activityBus == nil {
		return
	}
	g.activityBus.Publish(ctx, biz.ActivityEvent{
		Event: eventType,
		Activity: biz.Activity{
			ID:              uuid.NewString(),
			Kind:            biz.ActivityKindPlan,
			Status:          status,
			SessionID:       spiritSessionID,
			SpiritSessionID: spiritSessionID,
			Timestamp:       time.Now().UTC(),
			Meta: map[string]any{
				"phase":       phase,
				"message":     message,
				"duration_ms": durationMs,
				"session_id":  spiritSessionID,
				"source":      "pre-planning-gate",
			},
		},
		Domain: biz.ActivityDomainChat,
	})
}
