package team

import (
	"context"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

// teamMemberFlowStepID returns the per-node flow-log step ID for member
// execution monitoring. The node suffix keeps FlowContext start/done timing
// isolated when members execute in parallel; the "team.member" prefix resolves
// the display title via stepTitleRegistry's prefix fallback.
func teamMemberFlowStepID(nodeID string) string {
	return "team.member." + strings.TrimSpace(nodeID)
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
	// 2026-08-08 问题4b：取 node_start 追踪的真实开始时刻落真实执行窗口；
	// watch 未观察到 node_start 时零值回退（StartedAt=FinishedAt=落库时刻）。
	startedAt, _ := stepCtx.NodeStartedAt(nodeID)
	// usageSource "": this path carries no token counts (asst TokenIn/Out are
	// zero → recordMemberUsage skips), so there is no provenance to label.
	// runLevelAttribution=false: zero-token rows never reach session metrics anyway.
	r.persistStep(ctx, run, stepCtx.TeamID, stepCtx.SortIndex(nodeID), m, ag, stepCtx.InputPreview, asst, strings.TrimSpace(ag.Provider), strings.TrimSpace(ag.Model), "default", toolCallCount, 0, "", startedAt, false)
	stepCtx.MarkPersisted(nodeID)
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		agentName := strutil.FirstNonEmpty(ag.DisplayName, ag.AgentKey)
		stepID := teamMemberFlowStepID(nodeID)
		switch {
		case skipped:
			em.LogSkip(stepID, fmt.Sprintf("成员 %s 已跳过", agentName),
				event.P("node_id", nodeID), event.P("agent_key", ag.AgentKey))
		case strings.TrimSpace(errMsg) != "":
			em.LogError(stepID, fmt.Sprintf("成员 %s 执行失败：%s", agentName, errMsg),
				event.P("node_id", nodeID), event.P("agent_key", ag.AgentKey))
		default:
			em.LogDone(stepID, fmt.Sprintf("成员 %s 执行完成", agentName),
				event.P("node_id", nodeID), event.P("agent_key", ag.AgentKey),
				event.P("output_len", len(outputPreview)))
		}
	}
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
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogStart(teamMemberFlowStepID(nodeID), fmt.Sprintf("成员 %s 开始执行", agentName),
			event.P("node_id", nodeID),
			event.P("agent_key", ag.AgentKey),
			event.P("team_id", stepCtx.TeamID),
			event.P("run_id", stepCtx.TeamRunID))
	}
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
	if r.hasPublisher() {
		cp := run
		now := time.Now().UTC()
		ts := biz.TeamStage{
			ID:        string(agent.NewTeamStageActivityID(stepCtx.TeamID, stepCtx.RootTaskID)),
			TeamID:    stepCtx.TeamID,
			SessionID: stepCtx.SpiritSessionID,
			Status:    biz.TeamStageStatusCompleted,
			Stage:     biz.TeamStageStageCompleted,
			StartedAt: now,
			Version:   1,
		}
		if ts.SessionID == "" {
			ts.SessionID = stepCtx.SessionID
		}
		r.publishEvent(ctx, biz.NewTeamStageCompletedEvent(ts))
		r.publishEvent(ctx, biz.NewSystemNoticeEvent(ts.SessionID, "team_stage_completed", "", map[string]any{
			"run_id":  run.ID,
			"run":     cp,
			"team_id": stepCtx.TeamID,
		}))
		// 2026-07-28 单写者重设计：原 finalizePendingSessionActivities（为无
		// step 的缺口成员补发 completed）已删除——消息生命周期不得升格为成员
		// 终态。缺口成员（无 step / 无 agent session）统一由 service 终态
		// outcome pass（哨兵权威带，含 F4 兜底）裁决发布终态事件。
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
	promptTok, completionTok, cachedTok int,
	prov, mod string,
	usageSource string,
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
	if r.hasPublisher() {
		agentName := strutil.FirstNonEmpty(anchorAg.DisplayName, anchorAg.AgentKey)
		r.publishTeamStepActivity(ctx, run, teamID, anchorAg.AgentKey, agentName,
			biz.ActivityEventCreated, biz.ActivityStatusRunning, "executing", nil)
	}
	stepMsg := assistantMsg
	stepMsg.TokenIn, stepMsg.TokenOut = promptTok, completionTok
	prov = strutil.FirstNonEmpty(strings.TrimSpace(prov), strings.TrimSpace(anchorAg.Provider))
	mod = strutil.FirstNonEmpty(strings.TrimSpace(mod), strings.TrimSpace(anchorAg.Model))
	// runLevelAttribution=true: this anchor fallback step carries RUN-LEVEL totals
	// (same totals the team_turn row records), so its usage row must not also
	// accumulate session metrics (P2-1 双计根治).
	r.persistStep(ctx, run, teamID, 0, anchor, anchorAg, userContent, stepMsg, prov, mod, "default", 0, cachedTok, usageSource, time.Time{}, true)
}
