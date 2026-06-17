package service

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
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
	planner biz.TaskPlannerPort
	bus     contract.Bus
	lg      loggateway.Logger
}

// NewPrePlanningGate constructs a PrePlanningGate.
func NewPrePlanningGate(planner biz.TaskPlannerPort, bus contract.Bus, lg loggateway.Logger) *PrePlanningGate {
	return &PrePlanningGate{
		planner: planner,
		bus:     bus,
		lg:      lg.With(loggateway.Domain("pre-planning-gate")),
	}
}

// Evaluate runs the quick complexity assessment and returns a gate decision.
// The intent artifact from the intent pass (if available) is passed through
// to the planner for a more accurate assessment.
func (g *PrePlanningGate) Evaluate(ctx context.Context, input biz.PlanInput) (GateDecision, error) {
	start := time.Now()
	g.publishPlanningPhase(ctx, contract.EnvelopeTypePlanningPhaseStart, "assess", "开始复杂度评估", input.SpiritSessionID, 0)

	level, score, err := g.planner.QuickAssess(ctx, input)
	if err != nil {
		g.lg.Warn("QuickAssess 失败",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.Str("spirit_session_id", input.SpiritSessionID),
			loggateway.Err(err),
		)
		g.publishPlanningPhase(ctx, contract.EnvelopeTypePlanningPhaseDone, "assess", "复杂度评估失败", input.SpiritSessionID, time.Since(start).Seconds()*1000)
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

	g.publishPlanningPhase(ctx, contract.EnvelopeTypePlanningPhaseDone, "assess", reason, input.SpiritSessionID, time.Since(start).Seconds()*1000)

	return decision, nil
}

// publishPlanningPhase publishes a planning timeline event.
func (g *PrePlanningGate) publishPlanningPhase(ctx context.Context, eventType contract.EnvelopeType, phase, message, sessionID string, durationMs float64) {
	if g.bus == nil {
		return
	}
	env := contract.NewEnvelope(eventType, "pre-planning-gate", sessionID)
	env.Metadata = map[string]any{
		"phase":        phase,
		"message":      message,
		"duration_ms":  durationMs,
		"session_id":   sessionID,
	}
	g.bus.Publish(ctx, env)
}
