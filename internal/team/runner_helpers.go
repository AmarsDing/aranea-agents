package team

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

// publishTeamRunFailedActivity publishes a typed TeamStageFailedEvent.
func (r *Runner) publishTeamRunFailedActivity(ctx context.Context, run biz.TeamRunRecord, msg string) {
	if !r.hasPublisher() {
		return
	}
	now := time.Now().UTC()
	ts := biz.TeamStage{
		ID:        string(agent.NewTeamStageActivityID(run.TeamID, string(agent.RootTaskActivityIDFromCtx(ctx)))),
		TeamID:    run.TeamID,
		SessionID: run.SpiritSessionID,
		Status:    biz.TeamStageStatusFailed,
		Stage:     biz.TeamStageStageFailed,
		StartedAt: now,
		Version:   1,
	}
	r.publishEvent(ctx, biz.NewTeamStageFailedEvent(ts))
	r.publishEvent(ctx, biz.NewSystemNoticeEvent(run.SpiritSessionID, "team_run_failed", msg, map[string]any{
		"run_id":        run.ID,
		"team_id":       run.TeamID,
		"error_message": msg,
	}))
}

// publishTeamStepActivity publishes a MemberSession lifecycle event for a team member.
//
// 2026-07-28 成员终态单写者重设计：runner 只允许发布 created（V=1 生命周期
// 事实）。成员 completed 投影已删除——「成员产出最终文本」是消息生命周期
// 而非工作结果（12:33：成员返回文本即显示成功，实际安装失败）。成员终态
// （completed/failed/skipped）由 service 终态 outcome pass 唯一裁决发布
// （outcome 哨兵权威带，见 biz.MemberSessionVersion*）；完整证据（中断
// session / 失败 step / 交付物门 / 验证门）只在团队终态时刻齐备。
func (r *Runner) publishTeamStepActivity(ctx context.Context, run biz.TeamRunRecord, teamID, agentKey, agentName string, eventType biz.ActivityEventType, status biz.ActivityStatus, stage string, step any) {
	if !r.hasPublisher() {
		return
	}
	childSessionID := run.SessionID
	if r.cfg.SessionChildLookup != nil {
		if sid, err := r.cfg.SessionChildLookup.LookupChildSessionID(ctx, run.SessionID, agentKey); err == nil && sid != "" {
			childSessionID = sid
		} else if err != nil {
			r.lg.Warn("publishTeamStepActivity: failed to lookup child session, falling back to team session",
				loggateway.StepID("team.run.step_activity"),
				loggateway.Str("team_session_id", run.SessionID),
				loggateway.Str("agent_key", agentKey),
				loggateway.Err(err),
			)
		}
	}
	// 2026-07-29 F-2 钳制：runner 只发布 created（running）生命周期事实，
	// 终态映射（completed/failed/skipped）与 updated 事件分支已按单写者
	// 重设计删除——终态落入 evidence 带会被 outcome 哨兵守卫静默拒绝，
	// 且消息生命周期不得冒充工作结果。status/eventType 参数仅用于下方
	// notice meta 透传，不再映射为 MemberSession 状态。
	msStatus := biz.MemberSessionStatusRunning
	teamStageID := string(agent.NewTeamStageActivityID(teamID, string(agent.RootTaskActivityIDFromCtx(ctx))))
	// 统一使用 v2 确定性 ID（与 spirit_team service 同一公式），
	// 保证 runner 与 service 写入同一 member_sessions_v2 行（upsert-by-ID）。
	// run.ID 是 v1 随机 UUID，仅用于 meta，不可写入 v2 实体。
	teamRunV2ID := agent.NewTeamRunV2ID(teamStageID)
	// 版本带模型（biz.MemberSessionVersion*）：Version 是写者权威层级而非
	// 任意编号。runner 只写 created 带（V=1）；终态 outcome 哨兵带专属 service。
	ms := biz.MemberSession{
		ID:              string(agent.NewMemberSessionActivityID(teamRunV2ID, agentKey)),
		TeamRunID:       teamRunV2ID,
		TeamStageID:     teamStageID,
		TaskID:          string(agent.RootTaskActivityIDFromCtx(ctx)),
		SessionID:       childSessionID,
		SpiritSessionID: run.SpiritSessionID,
		AgentKey:        agentKey,
		AgentName:       agentName,
		Status:          msStatus,
		StartedAt:       time.Now().UTC(),
		Version:         biz.MemberSessionVersionCreated,
	}
	r.publishEvent(ctx, biz.NewMemberSessionCreatedEvent(ms))
	// Emit a notice carrying the member's live progress for notice consumers;
	// member terminal status arrives only with the service outcome pass
	// (outcome 哨兵权威带，见 biz.MemberSessionVersion*).
	meta := map[string]any{
		"run_id":           run.ID,
		"step":             step,
		"child_session_id": childSessionID,
		"activity_kind":    string(biz.ActivityKindSession),
		"activity_status":  string(status),
		"activity_event":   string(eventType),
		"agent_key":        agentKey,
		"team_id":          teamID,
	}
	r.publishEvent(ctx, biz.NewSystemNoticeEvent(run.SpiritSessionID, stage, "", meta))
}

func preview(s string, max int) string {
	return strings.TrimSpace(runesTruncate(strings.TrimSpace(s), max))
}

func runesTruncate(s string, max int) string {
	if max <= 0 || s == "" {
		return ""
	}
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}

func topologyJSON(def Definition) string {
	ids := make([]string, 0, len(def.Members))
	for _, m := range EnabledMembers(def) {
		ids = append(ids, m.AgentID)
	}
	b, _ := json.Marshal(map[string]any{"member_order": ids, "mode": def.Mode})
	return string(b)
}

// extractOptsFromInput extracts turn options from a biz.TurnInput.
func extractOptsFromInput(input biz.TurnInput) (dialogMode, prov, mod string, attN int) {
	return strings.TrimSpace(input.Options.DialogMode),
		strings.TrimSpace(input.Options.Provider),
		strings.TrimSpace(input.Options.Model),
		len(artifactbiz.NormalizeAttachmentIDs(input.Options.AttachmentIDs))
}

func mergeTeamUserTurnMetaJSON(userOpts string, displayContent, sendText string, lg loggateway.Logger) (string, error) {
	displayContent = strings.TrimSpace(displayContent)
	sendText = strings.TrimSpace(sendText)
	var opts map[string]any
	if strings.TrimSpace(userOpts) == "" {
		opts = map[string]any{}
	} else if err := json.Unmarshal([]byte(userOpts), &opts); err != nil {
		lg.Warn("解析 team user turn meta 失败", loggateway.StepID("team.runner_helpers"), loggateway.Err(err))
		return userOpts, err
	}
	sendLen := len([]rune(sendText))
	opts["team_user_display_len"] = len([]rune(displayContent))
	opts["team_user_send_len"] = sendLen
	opts["team_user_send_differs_from_display"] = sendText != displayContent
	opts["user_turn_length"] = sendLen
	if sendText != "" {
		pr := runesTruncate(sendText, 240)
		opts["team_user_send_preview"] = pr
		opts["user_text_preview"] = pr
	}
	out, err := json.Marshal(opts)
	if err != nil {
		return userOpts, err
	}
	return string(out), nil
}

func (r *Runner) finishRunErr(ctx context.Context, run *biz.TeamRunRecord, t0 time.Time, msg string) {
	if run == nil {
		return
	}
	if biz.IsTeamRunTerminalStatus(run.Status) {
		return
	}
	// F-B：提前取出 graph exec ID（状态转换可能整体替换 run 记录）。
	graphExecID := strings.TrimSpace(run.GraphExecutionID)
	updatedRun, transitionErr := r.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusFailed)
	if transitionErr != nil {
		r.lg.Error("TransitionRunStatus failed in finishRunErr",
			loggateway.StepID("team.run.transition_fail"),
			loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
	} else {
		updatedRun.ErrorMessage = msg
		updatedRun.DurationMS = int(time.Since(t0).Milliseconds())
		updatedRun.SpiritSessionID = run.SpiritSessionID
		if err := r.runWriter.UpdateTeamRun(ctx, updatedRun); err != nil {
			r.lg.Warn("UpdateTeamRun failed in finishRunErr", loggateway.StepID("team.run.err_update_fail"), loggateway.Str("team_run_id", run.ID), loggateway.Err(err))
		}
		*run = updatedRun
	}
	// F-B：失败终态同步收敛 graph_executions（team graph 路径无事件消费者）。
	if graphExecID != "" && r.mediator != nil {
		_ = r.mediator.FinalizeTeamGraphExecution(ctx, graphExecID, true, msg)
	}
	if biz.ShouldRecordTaskDeadLetter(run.DefinitionSnapshotJSON) {
		if dlerr := r.deadLetter.CreateTaskDeadLetter(ctx, biz.TaskDeadLetter{
			ID:               uuid.NewString(),
			SourceType:       biz.TaskDeadLetterSourceTeamRun,
			SourceID:         run.ID,
			TeamID:           run.TeamID,
			TeamRunID:        run.ID,
			SessionID:        strings.TrimSpace(run.SessionID),
			GraphExecutionID: strings.TrimSpace(run.GraphExecutionID),
			ErrorMessage:     msg,
			Status:           biz.TaskDeadLetterStatusPending,
			CreatedAt:        agent.RFC3339Now(),
		}); dlerr != nil {
			r.lg.Warn("CreateTaskDeadLetter failed", loggateway.StepID("team.run.dead_letter_fail"), loggateway.Str("team_run_id", run.ID), loggateway.Err(dlerr))
		}
	}
	if r.hasPublisher() {
		r.publishTeamRunFailedActivity(ctx, *run, msg)
	}
	r.publishTeamRunSummary(ctx, *run)
	r.recordRunCompletion(ctx, *run, msg, t0)
	// S1（K2 流程日志覆盖）：失败终态必须发射 LogError 流程日志，让业务用户
	// 在 Monitor Logs「流程日志」Tab 看到团队任务的失败原因。
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogError("team.run.finish", msg)
	}
	r.lg.With(loggateway.SessionID(strings.TrimSpace(run.SessionID))).Warn(msg, loggateway.StepID("team.run.finish"), loggateway.Str("team_id", run.TeamID), loggateway.Str("run_id", run.ID))
}

// recordRunCompletion writes the runner.completion monitor event for a
// terminal team run (success or error). Classic teams and spirit-orchestrated
// teams both execute through this Runner, making it the single funnel that
// feeds the Runner metrics panel and the runner.error_rate alert rule
// (restored after the Activity-First migration dropped the legacy writer).
func (r *Runner) recordRunCompletion(ctx context.Context, run biz.TeamRunRecord, errMsg string, t0 time.Time) {
	if r == nil || r.monitor == nil {
		return
	}
	de := biz.DomainEvent{
		Type:       biz.DomainEventRunnerCompletion,
		SessionID:  strings.TrimSpace(run.SessionID),
		RunID:      strings.TrimSpace(run.ID),
		TeamID:     strings.TrimSpace(run.TeamID),
		DurationMS: time.Since(t0).Milliseconds(),
		Timestamp:  time.Now().UTC(),
		RunKind:    "team",
	}
	if msg := strings.TrimSpace(errMsg); msg != "" {
		de.Error = &biz.DomainError{Message: msg}
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := biz.RecordRunnerCompletion(recCtx, r.monitor, de); err != nil {
		r.lg.Warn("runner.completion 监控事件落库失败",
			loggateway.StepID("team.runner_completion_fail"),
			loggateway.Str("team_id", run.TeamID),
			loggateway.Str("run_id", run.ID),
			loggateway.Err(err))
	}
}

func (r *Runner) publishTeamRunSummary(ctx context.Context, run biz.TeamRunRecord) {
	if r == nil || !r.hasPublisher() || r.runReader == nil {
		return
	}
	steps, err := r.runReader.ListTeamRunSteps(ctx, run.ID)
	if err != nil {
		steps = nil
	}
	data := biz.BuildTeamRunSummaryData(run, steps)
	summary := SummaryMapFromData(data)
	if b, merr := json.Marshal(summary); merr == nil {
		if uerr := r.runWriter.UpdateTeamRunSummaryJSON(ctx, run.ID, string(b)); uerr != nil {
			r.lg.Warn("UpdateTeamRunSummaryJSON failed", loggateway.StepID("team.run.summary_update_fail"), loggateway.Str("team_run_id", run.ID), loggateway.Err(uerr))
		}
	}
	r.publishEvent(ctx, biz.NewSystemNoticeEvent(run.SpiritSessionID, "team_summary", "", map[string]any{
		"run_id":       run.ID,
		"team_id":      run.TeamID,
		"run":          run,
		"team_summary": summary,
	}))
}

func (r *Runner) persistStep(ctx context.Context, run biz.TeamRunRecord, teamID string, sortIdx int, m MemberDef, ag biz.Agent, userContent string, asst biz.ChatMessage, prov, mod, dialogMode string, toolCallCount, cachedTok int, usageSource string, startedAt time.Time, attribution string) {
	step := biz.TeamRunStep{
		ID:            uuid.NewString(),
		RunID:         run.ID,
		TeamID:        teamID,
		AgentID:       ag.ID,
		AgentKey:      ag.AgentKey,
		AgentName:     strutil.FirstNonEmpty(ag.DisplayName, ag.AgentKey),
		Role:          m.Role,
		SortOrder:     sortIdx,
		Status:        asst.Status,
		InputPreview:  preview(userContent, 400),
		OutputPreview: preview(asst.ContentMarkdown, 400),
		TokenIn:       asst.TokenIn,
		TokenOut:      asst.TokenOut,
		CostMicroUSD:  0,
		DurationMS:    asst.LatencyMS,
		ErrorMessage:  asst.ErrorMessage,
		StartedAt:     asst.CreatedAt,
		FinishedAt:    asst.CreatedAt,
		CreatedAt:     agent.RFC3339Now(),
		ToolCallCount: toolCallCount,
	}
	// P2-1 (2026-08-19): backfill the step's cost from the active pricing
	// snapshot (same pricing path as persisted usage events). 0 when unpriced
	// or tokenless — matches the usage-row behavior for missing pricing.
	if r.usage != nil {
		step.CostMicroUSD = r.usage.QuoteTokenUsageCostMicroUSD(ctx, prov, mod, asst.TokenIn, asst.TokenOut, cachedTok)
	}
	// 2026-08-08 问题4b：graph watch 路径传入 node_start 追踪的真实开始时刻
	// 时，落真实执行窗口——StartedAt 取追踪值、DurationMS 取墙钟耗时；否则
	// StartedAt≈FinishedAt≈落库时刻、DurationMS=0（graph 成员消息无 LatencyMS）。
	// 零值回退保持既有行为（standalone / watch 迟到 / anchor fallback）。
	if !startedAt.IsZero() {
		step.StartedAt = startedAt.UTC().Format(time.RFC3339)
		if d := time.Since(startedAt); d >= 0 {
			step.DurationMS = int(d.Milliseconds())
		}
	}
	saved, err := r.runWriter.CreateTeamRunStep(ctx, step)
	if err != nil {
		return
	}
	r.recordMemberUsage(ctx, run, teamID, ag, asst, prov, mod, dialogMode, saved.ID, cachedTok, usageSource, attribution)
	// 2026-07-28 单写者重设计：此处不再发布成员 completed 事件。step 落库
	// 即消息生命周期的事实记录；成员终态由 service 终态 outcome pass（哨兵
	// 权威带）依据完整证据链唯一裁决——成员产出最终文本不代表工作成功（12:33）。
}
