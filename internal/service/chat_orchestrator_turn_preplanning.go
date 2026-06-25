package service

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// forcedPlanningSystemPrompt is the system message injected when the pre-planning
// gate forces the planning path. It instructs the Spirit LLM to invoke the
// plan_and_execute tool instead of answering directly.
//
// The prompt is intentionally minimal: it conveys the gate's decision and the
// complexity signal without overriding the agent's domain instructions.
const forcedPlanningSystemPrompt = `Pre-Planning Gate decision: this task has been assessed as %s complexity (score %.2f).
Reason: %s.

You MUST invoke the plan_and_execute tool to decompose the task before producing the final answer.
Do not answer directly even if you believe you can; route through plan_and_execute so the
orchestration layer can persist a plan, allocate agents, and emit planning timeline events.`

// runPrePlanningGate evaluates task complexity after the intent pass and returns
// a gate decision. When the planner is unavailable (e.g., single-agent sessions
// without team orchestration deps), it returns a zero-value decision with no
// error, leaving the turn on the default direct-answer path.
//
// The gate is non-fatal: any error from QuickAssess is logged and propagated,
// but the caller treats it as "no force planning" (see chat_orchestrator_turn.go).
func (o *ChatOrchestrator) runPrePlanningGate(
	ctx context.Context,
	sessionID, content string,
	intentArt *intent.Artifact,
) (GateDecision, error) {
	planner := o.team().TaskPlanner
	if planner == nil {
		// No planner wired (e.g., single-agent deployment without Spirit orchestration).
		// Skip the gate silently — direct-answer path remains the default.
		return GateDecision{}, nil
	}

	bus := o.td().Pipeline.ActivityBus
	gate := NewPrePlanningGate(planner, bus, o.lg())

	planInput := biz.PlanInput{
		UserMessage:     content,
		SpiritSessionID: sessionID,
		IntentArtifact:  intentArtifactToBiz(intentArt),
	}
	if traceID, ok := biz.SpiritTraceIDFromContext(ctx); ok {
		planInput.TraceID = traceID
	}

	decision, err := gate.Evaluate(ctx, planInput)
	if err != nil {
		o.lg().Warn("预规划门控评估失败，回退到直接回答路径",
			loggateway.StepID(biz.SpiritStepPlannerAssess),
			loggateway.SessionID(sessionID),
			loggateway.Err(err),
		)
		return GateDecision{}, err
	}
	return decision, nil
}

// intentArtifactToBiz converts the intent pass artifact (internal/agent/intent)
// to the biz-layer mirror (biz.IntentArtifact). Returns nil when the source is nil
// to preserve the "no intent pass ran" signal downstream.
func intentArtifactToBiz(art *intent.Artifact) *biz.IntentArtifact {
	if art == nil {
		return nil
	}
	return &biz.IntentArtifact{
		RefinedGoal:     art.RefinedGoal,
		IntentKind:      art.IntentKind,
		SuccessCriteria: art.SuccessCriteria,
		Ambiguities:     art.Ambiguities,
		SearchHints:     art.SearchHints,
		RiskFlags:       art.RiskFlags,
	}
}

// forcedPlanningRunOption returns a trpc-agent RunOption that injects a system
// message instructing the Spirit LLM to invoke plan_and_execute. The injected
// message is appended to the run's context messages (same mechanism as the
// intent pass injection), so it does not replace the agent's system prompt.
func forcedPlanningRunOption(decision GateDecision) trpcagent.RunOption {
	msg := trpcmodel.NewSystemMessage(fmt.Sprintf(forcedPlanningSystemPrompt,
		string(decision.Level),
		decision.Score,
		strings.TrimSpace(decision.Reason),
	))
	return trpcagent.WithInjectedContextMessages([]trpcmodel.Message{msg})
}
