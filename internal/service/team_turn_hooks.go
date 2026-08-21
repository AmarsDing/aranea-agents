package service

import (
	"context"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

func (o *ChatOrchestrator) executeTeamTurnViaHooks(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	flow *event.TraceEmitter,
	unlock func(),
) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	if o == nil || o.team().TeamsNative == nil {
		if unlock != nil {
			unlock()
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Internal(apierror.DomainChatTeamNative, "team runner not wired")
	}
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)

	if qerr := o.admission().EnforceChatTurnQuotas(ctx, "", chatagent.UserIDFromCtx(ctx)); qerr != nil {
		if unlock != nil {
			unlock()
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, qerr
	}
	if qerr := o.checkTeamMemberQuotas(ctx, strings.TrimSpace(sess.TeamID)); qerr != nil {
		if unlock != nil {
			unlock()
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, qerr
	}
	if flow != nil {
		flow.LogStart("chat.team.invoke", "委派团队会话",
			event.P("team_id", strings.TrimSpace(sess.TeamID)), event.P("content_len", len(content)))
	}

	runID := uuid.NewString()
	teamCtx, teamCancel := context.WithCancel(ctx)
	// S-5（2026-08-05）：团队 chat 入口注入 RootTaskActivityID 并幂等建根
	// Task v2——与单 agent 路径（chat_orchestrator_turn.go:443）同一姿态。
	// 此前团队路径在注入点前提前 return，runner/service 派生 v2 ID 时 run
	// 维度为空，同团队每轮 turn 碰撞同一 team_stages_v2 行（FSM 转换失败、
	// 状态冻结），且 turn 树挂到 tasks_v2 不存在的幽灵 ID。
	rootTaskID := resolveRootTaskActivityID(input)
	teamCtx = chatagent.ContextWithRootTaskActivityID(teamCtx, rootTaskID)
	isContinuation := strings.TrimSpace(input.ParentTaskID) != ""
	if !isContinuation {
		o.ensureTeamTurnRootTask(ctx, sess, input, string(rootTaskID))
	}
	// No-Timeout principle (T1.1): no hard turn timeout — tasks run until
	// completion or user cancel. Mirrors the single-agent path in
	// runSingleAgentViaTRPC which uses WithCancel only (no WithTimeout).
	// User cancel is wired via o.runs.StoreCancelable below.
	o.runs.StoreCancelable(sessionID, runID, teamCancel)
	if err := o.runStatus().SetRunStatus(ctx, sessionID, runID, biz.TeamRunStatusRunning, ""); err != nil {
		o.lg().Warn("set run status failed on team turn start",
			loggateway.StepID("chat.team_turn.start"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")
	if unlock != nil {
		unlock()
	}
	defer func() {
		o.runs.Finish(sessionID, runID)
		// Skip processPendingQueue when this turn was started from inside
		// the iterative pending-queue loop (see processPendingQueue). The
		// loop owns draining the queue; re-entering would spawn a new
		// goroutine per message, re-introducing the chain we eliminated.
		if !inPendingLoop(ctx) {
			o.processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "",
				string(chatagent.RootTaskActivityIDFromCtx(ctx)))
		}
	}()

	userMsg, assistantMsg, err = o.team().TeamsNative.RunTurnFromInput(teamCtx, sess, input)
	if err != nil {
		if !isContinuation {
			o.terminalizeTeamTurnRootTask(ctx, string(rootTaskID), biz.TaskStatusFailed)
		}
		if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, biz.TeamRunStatusFailed, err.Error()); serr != nil {
			o.lg().Warn("set run status failed on team turn error",
				loggateway.StepID("chat.team_turn.fail"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(serr))
		}
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		// 2026-07-29 F-1/F-3：standalone（Mode A）团队 ParentSessionID 为空，
		// 回退 team session ID 作为聚合根（与 runner deriveSpiritSessionID
		// 回退 sess.ID 一致）；不再以 ParentSessionID 为空跳过终态 pass。
		if teamID := strings.TrimSpace(sess.TeamID); teamID != "" {
			spiritID := strings.TrimSpace(sess.ParentSessionID)
			if spiritID == "" {
				spiritID = sessionID
			}
			o.teamStarter().HandleTeamTurnResult(teamCtx, spiritID, teamID, "failed", err.Error(), "")
		}
		return userMsg, assistantMsg, err
	}

	if err := o.runStatus().SetRunStatus(ctx, sessionID, runID, "completed", ""); err != nil {
		o.lg().Warn("set run status failed on team turn complete",
			loggateway.StepID("chat.team_turn.complete"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}
	if !isContinuation {
		o.terminalizeTeamTurnRootTask(ctx, string(rootTaskID), biz.TaskStatusCompleted)
	}
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, "")
	if teamID := strings.TrimSpace(sess.TeamID); teamID != "" {
		spiritID := strings.TrimSpace(sess.ParentSessionID)
		if spiritID == "" {
			spiritID = sessionID
		}
		o.teamStarter().HandleTeamTurnResult(teamCtx, spiritID, teamID, "completed", "", "")
	}
	o.recordTeamSessionTurn(ctx, sessionID, strings.TrimSpace(sess.TeamID),
		userMsg.ID, assistantMsg.ID, "", "",
		assistantMsg.TokenIn, assistantMsg.TokenOut, assistantMsg.TokenCached, assistantMsg.ContentMarkdown)
	return userMsg, assistantMsg, nil
}

// ensureTeamTurnRootTask idempotently creates the root Task v2 row for a
// team chat turn (S-5). Task.SessionID is the spirit root (per spec §3.2.2),
// resolved with the same fallback chain as the runner's deriveSpiritSessionID.
// Upsert-by-ID + pre-generated UUID makes this safe on retry.
func (o *ChatOrchestrator) ensureTeamTurnRootTask(ctx context.Context, sess biz.Session, input biz.TurnInput, rootTaskID string) {
	writer := o.taskV2Writer()
	if writer == nil || strings.TrimSpace(rootTaskID) == "" {
		return
	}
	spiritID := strings.TrimSpace(sess.RootSessionID)
	if spiritID == "" {
		spiritID = strings.TrimSpace(sess.ParentSessionID)
	}
	if spiritID == "" {
		spiritID = strings.TrimSpace(sess.ID)
	}
	now := time.Now().UTC()
	if _, err := writer.UpsertTask(ctx, biz.Task{
		ID:          rootTaskID,
		SessionID:   spiritID,
		UserMessage: strings.TrimSpace(input.Content),
		Status:      biz.TaskStatusRunning,
		Version:     1,
		CreatedAt:   now,
		UpdatedAt:   now,
	}); err != nil {
		o.lg().Warn("团队入口根 Task 创建失败",
			loggateway.StepID("chat.team_turn.root_task"),
			loggateway.Str("session_id", sess.ID),
			loggateway.Err(err))
	}
}

// terminalizeTeamTurnRootTask marks the team turn's root Task v2
// completed/failed (S-5). Without this the task stays "running" forever and
// startup recovery (FailOrphanedInFlight) would flip every finished team
// task to interrupted on the next process restart. CompleteTaskTerminal is
// idempotent and derives version from DB, so a racing outcome pass cannot
// corrupt it.
func (o *ChatOrchestrator) terminalizeTeamTurnRootTask(ctx context.Context, rootTaskID string, status biz.TaskStatus) {
	writer := o.taskV2Writer()
	if writer == nil || strings.TrimSpace(rootTaskID) == "" {
		return
	}
	if _, err := writer.CompleteTaskTerminal(ctx, biz.Task{ID: rootTaskID, Status: status}); err != nil {
		o.lg().Warn("团队入口根 Task 终态化失败",
			loggateway.StepID("chat.team_turn.root_task_terminal"),
			loggateway.Str("task_id", rootTaskID),
			loggateway.Str("status", string(status)),
			loggateway.Err(err))
	}
}
