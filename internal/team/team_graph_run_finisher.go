package team

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

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
	ag, err := r.lookupAgent(ctx, m.AgentID)
	if err != nil {
		r.lg.Warn("catalog agent lookup failed", loggateway.StepID("team.graph.step.persist"), loggateway.Str("run_id", stepCtx.TeamRunID), loggateway.Str("node_id", nodeID), loggateway.Str("agent_id", m.AgentID), loggateway.Err(err))
		return
	}
	run, err := r.runReader.GetTeamRunByID(ctx, stepCtx.TeamRunID)
	if err != nil {
		r.lg.Warn("team run lookup failed", loggateway.StepID("team.graph.step.persist"), loggateway.Str("run_id", stepCtx.TeamRunID), loggateway.Str("node_id", nodeID), loggateway.Err(err))
		return
	}
	run.SpiritSessionID = stepCtx.SpiritSessionID
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

// PublishTeamStepStarted publishes a session ActivityEvent (kind=session,
// status=running, stage=executing) when a graph node starts executing. This
// ensures the frontend AgentCard appears in "running" state before the member's
// thinking/action/reply activities arrive, solving the children chain breakage
// caused by the previous behavior where the session activity was only published
// after the member finished (in PersistGraphRunStep).
func (r *Runner) PublishTeamStepStarted(ctx context.Context, stepCtx *GraphRunStepContext, nodeID string) {
	if r == nil || stepCtx == nil || strings.TrimSpace(nodeID) == "" {
		return
	}
	m, ok := stepCtx.MemberDefForNode(nodeID)
	if !ok {
		return
	}
	ag, err := r.lookupAgent(ctx, m.AgentID)
	if err != nil {
		r.lg.Warn("catalog agent lookup failed in PublishTeamStepStarted",
			loggateway.StepID("team.graph.step.started"),
			loggateway.Str("run_id", stepCtx.TeamRunID),
			loggateway.Str("node_id", nodeID),
			loggateway.Str("agent_id", m.AgentID),
			loggateway.Err(err))
		return
	}
	run, err := r.runReader.GetTeamRunByID(ctx, stepCtx.TeamRunID)
	if err != nil {
		r.lg.Warn("team run lookup failed in PublishTeamStepStarted",
			loggateway.StepID("team.graph.step.started"),
			loggateway.Str("run_id", stepCtx.TeamRunID),
			loggateway.Str("node_id", nodeID),
			loggateway.Err(err))
		return
	}
	run.SpiritSessionID = stepCtx.SpiritSessionID
	agentName := strutil.FirstNonEmpty(ag.DisplayName, ag.AgentKey)
	// Publish the "created" session event so the frontend renders the AgentCard
	// before member thinking/action/reply activities arrive.
	r.publishTeamStepActivity(ctx, run, stepCtx.TeamID, ag.AgentKey, agentName,
		biz.ActivityEventCreated, biz.ActivityStatusRunning, "executing", nil)
}

// FinalizeGraphTeamRun closes a deferred team run and publishes team_summary.
func (r *Runner) FinalizeGraphTeamRun(ctx context.Context, stepCtx *GraphRunStepContext, failed bool, errMsg string) {
	if r == nil || stepCtx == nil || r.runReader == nil {
		return
	}
	run, err := r.runReader.GetTeamRunByID(ctx, stepCtx.TeamRunID)
	if err != nil {
		return
	}
	run.SpiritSessionID = stepCtx.SpiritSessionID
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
	steps, _ := r.runReader.ListTeamRunSteps(ctx, run.ID)
	enrichTeamRunMetricsFromSteps(&run, steps)
	updatedRun, transitionErr := r.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusSuccess)
	if transitionErr != nil {
		r.lg.Error("TransitionRunStatus failed in FinalizeGraphTeamRun",
			loggateway.StepID("team.run.transition_fail"),
			loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
		return
	}
	// Preserve token/duration data from the original run before the transition.
	updatedRun.DurationMS = int(time.Since(t0).Milliseconds())
	if run.TokenIn > 0 {
		updatedRun.TokenIn = run.TokenIn
	}
	if run.TokenOut > 0 {
		updatedRun.TokenOut = run.TokenOut
	}
	if strings.TrimSpace(run.OutputPreview) != "" {
		updatedRun.OutputPreview = run.OutputPreview
	}
	// Preserve transient SpiritSessionID (team_runs has no such column;
	// publishTeamRunSummary → TeamSummaryActivityEvent relies on it).
	updatedRun.SpiritSessionID = run.SpiritSessionID
	if err := r.runWriter.UpdateTeamRun(ctx, updatedRun); err != nil {
		r.lg.Warn("UpdateTeamRun failed in FinalizeGraphTeamRun", loggateway.StepID("team.graph.finisher_update_fail"), loggateway.Str("team_run_id", updatedRun.ID), loggateway.Err(err))
	}
	run = updatedRun
	if r.td.Pipeline.EventBus != nil {
		cp := run
		ev := biz.ActivityEvent{
			Event: biz.ActivityEventCompleted,
			Activity: biz.Activity{
				ID:              agent.TeamStageActivityID(stepCtx.TeamID),
				Kind:            biz.ActivityKindTeamStage,
				Status:          biz.ActivityStatusCompleted,
				SessionID:       stepCtx.SessionID,
				SpiritSessionID: stepCtx.SpiritSessionID,
				TeamID:          stepCtx.TeamID,
				Timestamp:       time.Now().UTC(),
				Stage:           "completed",
				Meta:            map[string]any{"run_id": run.ID, "run": cp},
			},
			Domain: biz.ActivityDomainChat,
		}
		// Phase 3b-D: bridge to v2 EventBus. ActivityBridgeEvent preserves the
		// v1 TeamStage Completed activity (Meta.run_id, Meta.run snapshot) so
		// the frontend team_stage terminal state renders correctly.
		r.td.Pipeline.EventBus.Publish(ctx, biz.NewActivityBridgeEvent(ev))
		// Finalize session activities for members that were started (session
		// "created" event published at team start) but never executed in the
		// graph. Without this, their AgentCards stay in "running" forever even
		// after the team is marked completed. Only members whose step was
		// persisted (via PersistGraphRunStep) get a "completed" event during
		// normal flow; this loop covers the rest.
		r.finalizePendingSessionActivities(ctx, run, stepCtx.TeamID, steps)
		r.publishTeamRunSummary(ctx, run)
	}
}

// finalizePendingSessionActivities publishes "completed" session events for
// team members that were started but never had their step persisted.
//
// At team start (runner_team_trpc.go), session "created" events are published
// for ALL enabled members, putting their AgentCards in "running" status. During
// graph execution, only members whose graph node reaches a terminal state get
// a "completed" event (via persistStep → publishTeamStepActivity). Members
// whose nodes are never reached (e.g., synthesizer finished early, or member
// has no graph node) stay "running" indefinitely.
//
// This function enumerates all enabled members from the team definition
// snapshot, compares against persisted steps, and publishes a "completed"
// event for the gap. Uses AgentKey for dedup since multiple member entries
// can share the same agent (e.g., programmer as both synthesizer and worker).
func (r *Runner) finalizePendingSessionActivities(ctx context.Context, run biz.TeamRunRecord, teamID string, persistedSteps []biz.TeamRunStep) {
	if r == nil || r.td.Pipeline.EventBus == nil {
		return
	}
	persistedAgentKeys := make(map[string]struct{}, len(persistedSteps))
	for _, s := range persistedSteps {
		if s.AgentKey == "" {
			continue
		}
		persistedAgentKeys[s.AgentKey] = struct{}{}
	}
	def, derr := ParseDefinition(run.DefinitionSnapshotJSON)
	if derr != nil {
		r.lg.Warn("finalizePendingSessionActivities: parse definition failed",
			loggateway.StepID("team.graph.finalize_sessions"),
			loggateway.Str("team_run_id", run.ID),
			loggateway.Err(derr))
		return
	}
	for _, m := range EnabledMembers(def) {
		ag, lookupErr := r.lookupAgent(ctx, m.AgentID)
		if lookupErr != nil {
			continue
		}
		if _, ok := persistedAgentKeys[ag.AgentKey]; ok {
			continue
		}
		agentName := strutil.FirstNonEmpty(ag.DisplayName, ag.AgentKey, m.Name)
		r.publishTeamStepActivity(ctx, run, teamID, ag.AgentKey, agentName,
			biz.ActivityEventCompleted, biz.ActivityStatusCompleted, "completed", nil)
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

func enrichTeamRunMetricsFromSteps(run *biz.TeamRunRecord, steps []biz.TeamRunStep) {
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
	run biz.TeamRunRecord,
	teamID string,
	anchor MemberDef,
	anchorAg biz.Agent,
	userContent string,
	assistantMsg biz.ChatMessage,
	promptTok, completionTok int,
) {
	if r == nil || r.runReader == nil {
		return
	}
	steps, err := r.runReader.ListTeamRunSteps(ctx, run.ID)
	if err != nil || len(steps) > 0 {
		return
	}
	// Publish the "created" session event for the fallback case since no
	// node_start graph event was published (PublishTeamStepStarted was not called).
	if r.td.Pipeline.EventBus != nil {
		agentName := strutil.FirstNonEmpty(anchorAg.DisplayName, anchorAg.AgentKey)
		r.publishTeamStepActivity(ctx, run, teamID, anchorAg.AgentKey, agentName,
			biz.ActivityEventCreated, biz.ActivityStatusRunning, "executing", nil)
	}
	stepMsg := assistantMsg
	stepMsg.TokenIn, stepMsg.TokenOut = promptTok, completionTok
	r.persistStep(ctx, run, teamID, 0, anchor, anchorAg, userContent, stepMsg, "", "", "default", 0)
}
