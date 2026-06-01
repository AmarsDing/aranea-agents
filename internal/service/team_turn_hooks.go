package service

import (
	"context"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

func (o *ChatOrchestrator) executeTeamTurnViaHooks(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	flow *event.TraceEmitter,
	unlock func(),
) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	if o == nil || o.team.TeamsNative == nil {
		if unlock != nil {
			unlock()
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.InternalServer("CHAT_TEAM_NATIVE", "team runner not wired")
	}
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)

	if qerr := enforceChatTurnQuotas(ctx, o.usage, "", chatagent.UserIDFromCtx(ctx)); qerr != nil {
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
	o.runs.StoreCancelable(sessionID, runID, teamCancel)
	o.setRunStatus(ctx, sessionID, runID, biz.TeamRunStatusRunning, "")
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")
	if unlock != nil {
		unlock()
	}
	defer func() {
		o.runs.Finish(sessionID)
		o.processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "")
	}()

	userMsg, assistantMsg, err = o.team.TeamsNative.RunTurnFromInput(teamCtx, sess, input)
	if err != nil {
		o.setRunStatus(ctx, sessionID, runID, biz.TeamRunStatusFailed, err.Error())
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		if sess.ParentSessionID != "" && strings.TrimSpace(sess.TeamID) != "" {
			o.teamStarter.HandleTeamTurnResult(ctx, sess.ParentSessionID, strings.TrimSpace(sess.TeamID), "failed", err.Error())
		}
		return userMsg, assistantMsg, err
	}

	o.setRunStatus(ctx, sessionID, runID, "completed", "")
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, "")
	if sess.ParentSessionID != "" && strings.TrimSpace(sess.TeamID) != "" {
		o.teamStarter.HandleTeamTurnResult(ctx, sess.ParentSessionID, strings.TrimSpace(sess.TeamID), "completed", "")
	}
	o.recordTeamSessionTurn(ctx, sessionID, strings.TrimSpace(sess.TeamID),
		userMsg.ID, assistantMsg.ID, "", "",
		assistantMsg.TokenIn, assistantMsg.TokenOut, assistantMsg.ContentMarkdown)
	return userMsg, assistantMsg, nil
}

func containsString(slice []string, s string) bool {
	for _, v := range slice {
		if v == s {
			return true
		}
	}
	return false
}
