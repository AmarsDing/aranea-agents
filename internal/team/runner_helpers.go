package team

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	"github.com/google/uuid"
)

// publishTeamRunFailedActivity publishes a team_run_failed ActivityEvent to the
// v2 EventBus via the ActivityBridgeEvent wrapper. Replaces the legacy
// EnvelopeTypeTeamRunFailed publish.
func (r *Runner) publishTeamRunFailedActivity(ctx context.Context, run biz.TeamRunRecord, msg string) {
	if !r.hasPublisher() {
		return
	}
	// SessionID = spirit session ID (not run.SessionID which is the team
	// session ID) so the frontend WS filter and listActivities API return
	// this team_stage "failed" event. Matches publishSpiritTeamAssembled.
	ev := biz.ActivityEvent{
		Event: biz.ActivityEventFailed,
		Activity: biz.Activity{
			ID:               string(agent.NewTeamStageActivityID(run.TeamID)),
			Kind:             biz.ActivityKindTeamStage,
			Status:           biz.ActivityStatusFailed,
			SessionID:        run.SpiritSessionID,
			SpiritSessionID:  run.SpiritSessionID,
			TeamID:           run.TeamID,
			Timestamp:        time.Now().UTC(),
			Stage:            "failed",
			ParentActivityID: string(agent.NewGraphStageActivityID(run.SpiritSessionID)),
			Meta: map[string]any{
				"run_id":        run.ID,
				"error_message": msg,
			},
		},
		Domain: biz.ActivityDomainChat,
	}
	// Phase 3b-D: bridge to v2 EventBus. The v1 ActivityEvent is preserved
	// verbatim inside ActivityBridgeEvent so consumers (OrchestrationStatusStore,
	// frontend inboundActivityEventHandler) can reuse existing field-aware logic.
	r.publishEvent(ctx, biz.NewActivityBridgeEvent(ev))
}

// publishTeamStepActivity publishes a per-member session ActivityEvent
// (started/finished) to the v2 EventBus via ActivityBridgeEvent. status and
// stage identify the lifecycle phase.
//
// The session event is published with SessionID = SpiritSessionID so it appears
// in the spirit session's activity tree (as a child of team_stage via
// ParentActivityID). The child_session_id in meta points to the member's
// individual agent session (resolved via SessionChildLookup) so the frontend
// can lazy-load member execution processes (thinking/action/reply).
func (r *Runner) publishTeamStepActivity(ctx context.Context, run biz.TeamRunRecord, teamID, agentKey, agentName string, eventType biz.ActivityEventType, status biz.ActivityStatus, stage string, step any) {
	if !r.hasPublisher() {
		return
	}
	// Resolve the member's individual agent session ID for child_session_id.
	// The frontend uses this to lazy-load member execution processes.
	// Fall back to run.SessionID (team session) when lookup is unavailable.
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
	// Use deterministic session activity ID so the child session's
	// ActivityProjector can compute the same ID and use it as
	// ParentActivityID for member thinking/action/reply activities.
	sessionActivityID := string(agent.NewSessionActivityID(teamID, agentKey))
	ev := biz.ActivityEvent{
		Event: eventType,
		Activity: biz.Activity{
			ID:               sessionActivityID,
			Kind:             biz.ActivityKindSession,
			Status:           status,
			SessionID:        run.SpiritSessionID,
			SpiritSessionID:  run.SpiritSessionID,
			TeamID:           teamID,
			AgentKey:         agentKey,
			AgentName:        agentName,
			Timestamp:        time.Now().UTC(),
			Stage:            stage,
			ParentActivityID: string(agent.NewTeamStageActivityID(teamID)),
			Meta: map[string]any{
				"run_id":           run.ID,
				"step":             step,
				"child_session_id": childSessionID,
			},
		},
		Domain: biz.ActivityDomainChat,
	}
	// Phase 3b-D: bridge to v2 EventBus. ActivityBridgeEvent preserves all
	// v1 fields (Meta.child_session_id, ParentActivityID, AgentKey/Name) so
	// the frontend AgentCard renders correctly under the team_stage parent.
	r.publishEvent(ctx, biz.NewActivityBridgeEvent(ev))
}

func preview(s string, max int) string {
	return strings.TrimSpace(runesTruncate(strings.TrimSpace(s), max))
}

func runesTruncate(s string, max int) string {
	if max <= 0 || s == "" {
		return s
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
	updatedRun, transitionErr := r.runTransitioner.TransitionRunStatus(ctx, run.ID, biz.TeamRunStatusFailed)
	if transitionErr != nil {
		r.lg.Error("TransitionRunStatus failed in finishRunErr",
			loggateway.StepID("team.run.transition_fail"),
			loggateway.Str("team_run_id", run.ID), loggateway.Err(transitionErr))
	} else {
		// Preserve error/duration data from the original run before the transition.
		updatedRun.ErrorMessage = msg
		updatedRun.DurationMS = int(time.Since(t0).Milliseconds())
		// Preserve transient SpiritSessionID: team_runs table has no
		// spirit_session_id column, so TransitionRunStatus's entTeamRunToBiz
		// returns it empty. publishTeamRunFailedActivity below relies on it
		// to attribute the failed event to the spirit session.
		updatedRun.SpiritSessionID = run.SpiritSessionID
		if err := r.runWriter.UpdateTeamRun(ctx, updatedRun); err != nil {
			r.lg.Warn("UpdateTeamRun failed in finishRunErr", loggateway.StepID("team.run.err_update_fail"), loggateway.Str("team_run_id", run.ID), loggateway.Err(err))
		}
		*run = updatedRun
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
	r.lg.With(loggateway.SessionID(strings.TrimSpace(run.SessionID))).Warn(msg, loggateway.StepID("team.run.finish"), loggateway.Str("team_id", run.TeamID), loggateway.Str("run_id", run.ID))
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
	// Phase 3b-D: bridge the v1 TeamSummaryActivityEvent to v2 EventBus. The
	// summary event carries a team_summary meta blob with no direct v2 typed
	// equivalent; ActivityBridgeEvent preserves the payload so the frontend
	// summary renderer continues to work without changes.
	r.publishEvent(ctx, biz.NewActivityBridgeEvent(TeamSummaryActivityEvent(run, steps)))
}

func (r *Runner) persistStep(ctx context.Context, run biz.TeamRunRecord, teamID string, sortIdx int, m MemberDef, ag biz.Agent, userContent string, asst biz.ChatMessage, prov, mod, dialogMode string, toolCallCount int) {
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
	saved, err := r.runWriter.CreateTeamRunStep(ctx, step)
	if err != nil {
		return
	}
	r.recordMemberUsage(ctx, run, teamID, ag, asst, prov, mod, dialogMode, saved.ID)
	if r.hasPublisher() {
		r.publishTeamStepActivity(ctx, run, teamID, ag.AgentKey, saved.AgentName, biz.ActivityEventCompleted, biz.ActivityStatusCompleted, "completed", saved)
	}
}
