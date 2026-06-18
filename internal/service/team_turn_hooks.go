package service

import (
	"context"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/apierror"

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
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Internal("CHAT_TEAM_NATIVE", "team runner not wired")
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
	// No-Timeout principle (T1.1): no hard turn timeout — tasks run until
	// completion or user cancel. Mirrors the single-agent path in
	// runSingleAgentViaTRPC which uses WithCancel only (no WithTimeout).
	// User cancel is wired via o.runs.StoreCancelable below.
	o.runs.StoreCancelable(sessionID, runID, teamCancel)
	o.runStatus().SetRunStatus(ctx, sessionID, runID, biz.TeamRunStatusRunning, "")
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")
	if unlock != nil {
		unlock()
	}
	defer func() {
		o.runs.Finish(sessionID)
		// Skip processPendingQueue when this turn was started from inside
		// the iterative pending-queue loop (see processPendingQueue). The
		// loop owns draining the queue; re-entering would spawn a new
		// goroutine per message, re-introducing the chain we eliminated.
		if !inPendingLoop(ctx) {
			o.processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "")
		}
	}()

	userMsg, assistantMsg, err = o.team().TeamsNative.RunTurnFromInput(teamCtx, sess, input)
	if err != nil {
		o.runStatus().SetRunStatus(ctx, sessionID, runID, biz.TeamRunStatusFailed, err.Error())
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		if sess.ParentSessionID != "" && strings.TrimSpace(sess.TeamID) != "" {
			o.teamStarter().HandleTeamTurnResult(ctx, sess.ParentSessionID, strings.TrimSpace(sess.TeamID), "failed", err.Error())
		}
		return userMsg, assistantMsg, err
	}

	o.runStatus().SetRunStatus(ctx, sessionID, runID, "completed", "")
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, "")
	if sess.ParentSessionID != "" && strings.TrimSpace(sess.TeamID) != "" {
		o.teamStarter().HandleTeamTurnResult(ctx, sess.ParentSessionID, strings.TrimSpace(sess.TeamID), "completed", "")
	}
	o.recordTeamSessionTurn(ctx, sessionID, strings.TrimSpace(sess.TeamID),
		userMsg.ID, assistantMsg.ID, "", "",
		assistantMsg.TokenIn, assistantMsg.TokenOut, assistantMsg.ContentMarkdown)
	return userMsg, assistantMsg, nil
}
