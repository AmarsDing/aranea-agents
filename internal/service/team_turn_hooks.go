package service

import (
	"context"
	"strings"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"

	"github.com/google/uuid"
	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// executeTeamTurnViaHooks runs a team session turn through the TurnExecutor lifecycle hooks (DECO-15).
// Build → Run → Persist → Project events are delegated to TeamsNative while admission/registry
// remain in ChatOrchestrator.
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
	o.setRunStatus(sessionID, runID, "running", "")
	if unlock != nil {
		unlock()
	}
	defer func() {
		o.runs.Finish(sessionID)
		o.processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "")
	}()

	// Hook: Build + Run (TeamsNative encapsulates team runtime build/project/persist).
	userMsg, assistantMsg, err = o.team.TeamsNative.RunTurnFromInput(teamCtx, sess, input)
	if err != nil {
		o.setRunStatus(sessionID, runID, "failed", err.Error())
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return userMsg, assistantMsg, err
	}

	// Hook: Persist + project completion (status + session turn record).
	o.setRunStatus(sessionID, runID, "completed", "")
	o.recordTeamSessionTurn(ctx, sessionID, strings.TrimSpace(sess.TeamID),
		userMsg.ID, assistantMsg.ID, "", "",
		assistantMsg.TokenIn, assistantMsg.TokenOut, assistantMsg.ContentMarkdown)
	return userMsg, assistantMsg, nil
}
