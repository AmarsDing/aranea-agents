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

func (r *Runner) finishRunErr(ctx context.Context, run *biz.TeamRun, t0 time.Time, msg string) {
	if run == nil {
		return
	}
	if biz.IsTeamRunTerminalStatus(run.Status) {
		return
	}
	run.Status = biz.TeamRunStatusFailed
	run.ErrorMessage = msg
	run.FinishedAt = agent.RFC3339Now()
	run.DurationMS = int(time.Since(t0).Milliseconds())
	if err := r.runWriter.UpdateTeamRun(ctx, *run); err != nil {
		r.lg.Warn("UpdateTeamRun failed in finishRunErr", loggateway.StepID("team.run.err_update_fail"), loggateway.Str("team_run_id", run.ID), loggateway.Err(err))
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
	if r.td.Pipeline.Bus != nil {
		failEnv := event.NewEnvelope(event.EnvelopeTypeTeamRunFailed, "team-runner", strings.TrimSpace(run.SessionID))
		failEnv.TeamID = run.TeamID
		failEnv.Metadata = map[string]any{"run_id": run.ID, "error_message": msg}
		r.td.Pipeline.Bus.Publish(ctx, failEnv)
	}
	r.publishTeamRunSummary(ctx, *run)
	r.lg.With(loggateway.SessionID(strings.TrimSpace(run.SessionID))).Warn(msg, loggateway.StepID("team.run.finish"), loggateway.Str("team_id", run.TeamID), loggateway.Str("run_id", run.ID))
}

func (r *Runner) publishTeamRunSummary(ctx context.Context, run biz.TeamRun) {
	if r == nil || r.td.Pipeline.Bus == nil || r.runReader == nil {
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
	r.td.Pipeline.Bus.Publish(ctx, TeamSummaryEnvelope(run, steps))
}

func (r *Runner) persistStep(ctx context.Context, run biz.TeamRun, teamID string, sortIdx int, m MemberDef, ag biz.Agent, userContent string, asst biz.ChatMessage, prov, mod, dialogMode string, toolCallCount int) {
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
	if r.td.Pipeline.Bus != nil {
		started := step
		started.Status = biz.TeamRunStatusRunning
		envStart := event.NewEnvelope(event.EnvelopeTypeTeamStepStarted, ag.AgentKey, run.SessionID)
		envStart.TeamID = teamID
		envStart.Metadata = map[string]any{"run_id": run.ID, "step": started}
		r.td.Pipeline.Bus.Publish(ctx, envStart)
	}
	saved, err := r.runWriter.CreateTeamRunStep(ctx, step)
	if err != nil {
		return
	}
	r.recordMemberUsage(ctx, run, teamID, ag, asst, prov, mod, dialogMode, saved.ID)
	if r.td.Pipeline.Bus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeTeamStepFinished, ag.AgentKey, run.SessionID)
		env.TeamID = teamID
		env.Metadata = map[string]any{"run_id": run.ID, "step": saved}
		r.td.Pipeline.Bus.Publish(ctx, env)
	}
}
