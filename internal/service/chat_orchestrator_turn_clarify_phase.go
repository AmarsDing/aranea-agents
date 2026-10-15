package service

import (
	"context"
	"time"

	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// applyClarificationGatePhase runs the clarification gate and updates intent
// injection options. When the gate suspends for user input, suspended is true
// and the caller should return empty messages with a nil error.
func (o *ChatOrchestrator) applyClarificationGatePhase(
	ctx context.Context,
	sessionID, runID string,
	ag biz.Agent,
	input biz.TurnInput,
	intentArtifact *intent.Artifact,
	inputRiskFlags []string,
	intentRunOpts []trpcagent.RunOption,
	emitter *event.TraceEmitter,
) (context.Context, biz.TurnInput, *intent.Artifact, []trpcagent.RunOption, bool, error) {
	clarifyStart := time.Now()
	clarifyDecision, clarifyErr := o.runClarificationGate(ctx, sessionID, intentArtifact, ag, input)
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: runClarificationGate",
		loggateway.StepID("chat.clarification_gate"),
		loggateway.Any("elapsed_ms", time.Since(clarifyStart).Milliseconds()),
		loggateway.Any("triggered", clarifyDecision.Triggered),
		loggateway.Any("clarify_err", clarifyErr != nil))
	if clarifyErr != nil {
		o.lg().Warn("澄清门执行失败，按原流程继续",
			loggateway.SessionID(sessionID),
			loggateway.StepID("chat.clarification_gate"),
			loggateway.Err(clarifyErr))
	} else if clarifyDecision.Triggered {
		ctx = biz.ContextWithRouteDecision(ctx, biz.RouteDecision{
			Lane:   biz.RouteLaneClarify,
			Reason: "clarification_gate",
		})
		emitter.LogDone("chat.clarification_gate", "澄清门已触发，等待用户作答",
			event.P("step_id", clarifyDecision.StepID))
		o.setRunStatus(ctx, sessionID, runID, string(biz.RunStateAwaitingUser), "")
		return ctx, input, intentArtifact, intentRunOpts, true, nil
	} else if clarifyDecision.AutoResolved {
		input = clarifyDecision.ResolvedInput
		if clarifyDecision.Artifact != nil {
			intentArtifact = clarifyDecision.Artifact
		}
		emitter.LogDone("chat.clarification_gate", "澄清问题均含推荐默认，按推荐假设继续执行",
			event.P("step_id", clarifyDecision.StepID))
	}

	if intentArtifact != nil {
		intentRunOpts = append([]trpcagent.RunOption{intent.RunOptionInject(intentArtifact)}, intentRunOpts...)
	} else if len(inputRiskFlags) > 0 {
		intentRunOpts = append([]trpcagent.RunOption{intent.RunOptionInjectInputRisk(inputRiskFlags)}, intentRunOpts...)
	}
	return ctx, input, intentArtifact, intentRunOpts, false, nil
}
