package team

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
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

func extractOpts(req *chatv1.SendChatMessageRequest) (dialogMode, prov, mod string, attN int) {
	if req == nil {
		return "", "", "", 0
	}
	o := req.GetOptions()
	if o == nil {
		return "", "", "", 0
	}
	return strings.TrimSpace(o.GetDialogMode()), strings.TrimSpace(o.GetProvider()), strings.TrimSpace(o.GetModel()), len(o.Attachments)
}

func mergeTeamUserTurnMetaJSON(userOpts string, displayContent, sendText string) (string, error) {
	displayContent = strings.TrimSpace(displayContent)
	sendText = strings.TrimSpace(sendText)
	var opts map[string]any
	if strings.TrimSpace(userOpts) == "" {
		opts = map[string]any{}
	} else if err := json.Unmarshal([]byte(userOpts), &opts); err != nil {
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

func publishTeamMonitor(ctx context.Context, bus event.Bus, level, msg, sessionID string) {
	if bus == nil || strings.TrimSpace(msg) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeLog, "team-runner", sessionID)
	env.Metadata = map[string]any{"level": level, "source": "team-runner"}
	env.Content = &event.EnvelopeContent{Text: msg}
	bus.Publish(ctx, env)
}

func (r *Runner) finishRunErr(ctx context.Context, run *biz.TeamRun, t0 time.Time, msg string) {
	if run == nil {
		return
	}
	run.Status = "failed"
	run.ErrorMessage = msg
	run.FinishedAt = agent.RFC3339Now()
	run.DurationMS = int(time.Since(t0).Milliseconds())
	_ = r.teams.UpdateTeamRun(ctx, *run)
	if r.td.EventBus != nil {
		cp := *run
		env := event.NewEnvelope(event.EnvelopeTypeTeamRunFinished, "team-runner", strings.TrimSpace(run.SessionID))
		env.TeamID = run.TeamID
		env.Metadata = map[string]any{"run_id": run.ID, "run": cp}
		r.td.EventBus.Publish(ctx, env)

		failEnv := event.NewEnvelope(event.EnvelopeTypeTeamRunFailed, "team-runner", strings.TrimSpace(run.SessionID))
		failEnv.TeamID = run.TeamID
		failEnv.Metadata = map[string]any{"run_id": run.ID, "error_message": msg}
		r.td.EventBus.Publish(ctx, failEnv)
	}
	publishTeamMonitor(ctx, r.td.EventBus, "WARN", fmt.Sprintf("team_run failed team_id=%s run_id=%s session_id=%s: %s", run.TeamID, run.ID, strings.TrimSpace(run.SessionID), msg), strings.TrimSpace(run.SessionID))
}

func (r *Runner) persistStep(ctx context.Context, run biz.TeamRun, teamID string, sortIdx int, m MemberDef, ag biz.Agent, userContent string, asst biz.ChatMessage) {
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
	}
	saved, err := r.teams.CreateTeamRunStep(ctx, step)
	if err != nil {
		return
	}
	if r.td.EventBus != nil {
		env := event.NewEnvelope(event.EnvelopeTypeTeamStepFinished, ag.AgentKey, run.SessionID)
		env.TeamID = teamID
		env.Metadata = map[string]any{"run_id": run.ID, "step": saved}
		r.td.EventBus.Publish(ctx, env)
	}
}
