package team

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

// TeamGraphRunFinisher persists graph run steps and finalizes deferred team runs.
type TeamGraphRunFinisher interface {
	PersistGraphRunStep(ctx context.Context, stepCtx *GraphRunStepContext, nodeID, outputPreview, errMsg string, skipped bool, toolCallCount int)
	FinalizeGraphTeamRun(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string)
}

// SetFinisher wires step persistence and team summary on graph resume completion.
func (c *TeamGraphRunCoordinator) SetFinisher(f TeamGraphRunFinisher) {
	if c == nil {
		return
	}
	c.finisher = f
}

// PersistGraphRunStep writes a TeamRunStep for a graph member node (initial run or resume).
func (r *Runner) PersistGraphRunStep(ctx context.Context, stepCtx *GraphRunStepContext, nodeID, outputPreview, errMsg string, skipped bool, toolCallCount int) {
	if r == nil || stepCtx == nil || strings.TrimSpace(nodeID) == "" {
		return
	}
	if stepCtx.AlreadyPersisted(nodeID) {
		return
	}
	m, ok := stepCtx.MemberDefForNode(nodeID)
	if !ok {
		return
	}
	ag, err := r.catalogAgent(ctx, m.AgentID)
	if err != nil {
		event.CtxFlowLogWarn(ctx, "team.graph.step.persist", "catalog agent lookup failed",
			event.P("run_id", stepCtx.TeamRunID), event.P("node_id", nodeID), event.P("agent_id", m.AgentID), event.P("error", err.Error()))
		return
	}
	run, err := r.teams.GetTeamRunByID(ctx, stepCtx.TeamRunID)
	if err != nil {
		event.CtxFlowLogWarn(ctx, "team.graph.step.persist", "team run lookup failed",
			event.P("run_id", stepCtx.TeamRunID), event.P("node_id", nodeID), event.P("error", err.Error()))
		return
	}
	status := biz.TeamMemberStepStatusOK
	if skipped {
		status = biz.TeamMemberStepStatusSkipped
	}
	if strings.TrimSpace(errMsg) != "" {
		status = biz.TeamMemberStepStatusError
	}
	asst := biz.ChatMessage{
		ID:              uuid.NewString(),
		SessionID:       stepCtx.SessionID,
		Role:            "assistant",
		ContentMarkdown: outputPreview,
		Status:          status,
		ErrorMessage:    errMsg,
		CreatedAt:       agent.RFC3339Now(),
	}
	r.persistStep(ctx, run, stepCtx.TeamID, stepCtx.SortIndex(nodeID), m, ag, stepCtx.InputPreview, asst, "", "", "default", toolCallCount)
	stepCtx.MarkPersisted(nodeID)
}

// FinalizeGraphTeamRun closes a deferred team run and publishes team_summary.
func (r *Runner) FinalizeGraphTeamRun(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string) {
	if r == nil || stepCtx == nil || r.teams == nil {
		return
	}
	run, err := r.teams.GetTeamRunByID(ctx, stepCtx.TeamRunID)
	if err != nil {
		return
	}
	if run.Status != biz.TeamRunStatusWaitingHuman && run.Status != biz.TeamRunStatusRunning {
		return
	}
	t0 := time.Now()
	if run.StartedAt != "" {
		if parsed, perr := time.Parse(time.RFC3339, run.StartedAt); perr == nil {
			t0 = parsed
		}
	}
	if failed {
		r.finishRunErr(ctx, &run, t0, errMsg)
		return
	}
	steps, _ := r.teams.ListTeamRunSteps(ctx, run.ID)
	enrichTeamRunMetricsFromSteps(&run, steps)
	now := agent.RFC3339Now()
	run.Status = biz.TeamRunStatusSuccess
	run.FinishedAt = now
	run.UpdatedAt = now
	run.DurationMS = int(time.Since(t0).Milliseconds())
	if err := r.teams.UpdateTeamRun(ctx, run); err != nil {
		event.CtxFlowLogWarn(ctx, "team.graph.finisher_update_fail", "UpdateTeamRun failed in FinalizeGraphTeamRun",
			event.P("team_run_id", run.ID), event.P("update_error", err.Error()))
	}
	if r.td.Pipeline.Bus != nil {
		cp := run
		env := event.NewEnvelope(event.EnvelopeTypeTeamRunFinished, "team-graph-coordinator", stepCtx.SessionID)
		env.TeamID = stepCtx.TeamID
		env.Metadata = map[string]any{"run_id": run.ID, "run": cp}
		r.td.Pipeline.Bus.Publish(ctx, env)
		r.publishTeamRunSummary(ctx, run)
	}
}

func buildResumeSessionContext(defJSON, inputPreview string, agentKeyFn func(agentID string) string, lg loggateway.Logger) (
	reg biz.OrchestrationRegistry,
	memberByNode map[string]MemberDef,
	stepSortIndex map[string]int,
) {
	def, err := ParseDefinition(defJSON)
	if err != nil {
		lg.Warn("buildResumeSessionContext: ParseDefinition failed", loggateway.StepID("team.intent.merge_fail"), loggateway.Err(err))
		return biz.OrchestrationRegistry{}, nil, nil
	}
	if agentKeyFn == nil {
		agentKeyFn = func(agentID string) string { return strings.TrimSpace(agentID) }
	}
	reg = BuildOrchestrationRegistry(def,
		agentKeyFn,
		func(agentID string) string { return strings.TrimSpace(agentID) },
	)
	memberByNode = MemberByCompileNodeID(def)
	members := EnabledMembers(def)
	stepSortIndex = make(map[string]int, len(members))
	for i, m := range members {
		stepSortIndex[memberNodeID(m, i)] = i
	}
	_ = inputPreview
	return reg, memberByNode, stepSortIndex
}

func enrichTeamRunMetricsFromSteps(run *biz.TeamRun, steps []biz.TeamRunStep) {
	if run == nil {
		return
	}
	var tokenIn, tokenOut int
	lastOutput := ""
	for _, s := range steps {
		tokenIn += s.TokenIn
		tokenOut += s.TokenOut
		if out := strings.TrimSpace(s.OutputPreview); out != "" {
			lastOutput = out
		}
	}
	if tokenIn > 0 {
		run.TokenIn = tokenIn
	}
	if tokenOut > 0 {
		run.TokenOut = tokenOut
	}
	if strings.TrimSpace(run.OutputPreview) == "" && lastOutput != "" {
		run.OutputPreview = preview(lastOutput, 512)
	}
}

// ensureGraphRunStepsFallback persists a single anchor step when graph events produced no team_run_steps.
func (r *Runner) ensureGraphRunStepsFallback(
	ctx context.Context,
	run biz.TeamRun,
	teamID string,
	anchor MemberDef,
	anchorAg biz.Agent,
	userContent string,
	assistantMsg biz.ChatMessage,
	promptTok, completionTok int,
) {
	if r == nil || r.teams == nil {
		return
	}
	steps, err := r.teams.ListTeamRunSteps(ctx, run.ID)
	if err != nil || len(steps) > 0 {
		return
	}
	stepMsg := assistantMsg
	stepMsg.TokenIn, stepMsg.TokenOut = promptTok, completionTok
	r.persistStep(ctx, run, teamID, 0, anchor, anchorAg, userContent, stepMsg, "", "", "default", 0)
}
