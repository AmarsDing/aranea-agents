package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	a2apkg "aranea-agents/internal/a2a"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// RunAgentTurn implements a2a.AgentTurnRunner for call_agent and HTTP Invoke dispatch.
func (o *ChatOrchestrator) RunAgentTurn(ctx context.Context, agentID, input string, timeoutSec int) (string, error) {
	if o == nil || o.td().Sessions == nil {
		return "", apierror.Internal("A2A", "chat service not configured")
	}
	if timeoutSec <= 0 {
		timeoutSec = 30
	}
	runCtx, cancel := context.WithTimeout(ctx, time.Duration(timeoutSec)*time.Second)
	defer cancel()

	uid := chatagent.UserIDFromCtx(runCtx)
	if uid == "" {
		uid = "system"
	}
	sess, err := o.td().Sessions.Create(runCtx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   strings.TrimSpace(agentID),
		OwnerType: "agent",
		Title:     fmt.Sprintf("a2a-%s", agentID),
		UserID:    uid,
	})
	if err != nil {
		return "", apierror.Internal("A2A", "create session: "+err.Error())
	}
	tr, err := o.Execute(runCtx, biz.TurnInput{
		SessionID: sess.ID,
		Content:   strings.TrimSpace(input),
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint: biz.EntryPointA2A,
			AllowQueue: false,
		},
	})
	if err != nil {
		return "", err
	}
	if tr.Outcome != biz.TurnOutcomeCompleted {
		return "", apierror.Internal("CHAT", "a2a turn outcome: "+string(tr.Outcome))
	}
	return tr.AssistantMsg.ContentMarkdown, nil
}

// RunEvalAgentTurn runs an evaluation agent turn.
func (o *ChatOrchestrator) RunEvalAgentTurn(ctx context.Context, agentID, input string) (string, error) {
	if o == nil || o.td().Sessions == nil {
		return "", apierror.Internal("CHAT", "eval: chat service not configured")
	}
	agentID = strings.TrimSpace(agentID)
	input = strings.TrimSpace(input)
	if agentID == "" || input == "" {
		return "", apierror.BadRequest("CHAT", "eval: agent_id and input are required")
	}
	sess, err := o.td().Sessions.Create(ctx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   agentID,
		OwnerType: "agent",
		Title:     fmt.Sprintf("eval-%s", agentID),
		UserID:    "1",
	})
	if err != nil {
		return "", apierror.Internal("CHAT", "eval: create session: %v", err)
	}
	_, asst, err := o.RunNativeAgentTurnFromInput(ctx, biz.TurnInput{
		SessionID: sess.ID,
		Content:   input,
	})
	if err != nil {
		return "", err
	}
	return asst.ContentMarkdown, nil
}

// RunCronTurn dispatches a cron-triggered turn via the unified TurnExecutor.
func (o *ChatOrchestrator) RunCronTurn(ctx context.Context, sessionID, content, teamID string) (userMsgID, agentMsgID string, err error) {
	input := biz.TurnInput{
		SessionID: sessionID,
		Content:   content,
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint: biz.EntryPointCron,
			AllowQueue: false,
		},
	}
	if strings.TrimSpace(teamID) != "" {
		input.TeamID = teamID
	}
	tr, err := o.Execute(ctx, input)
	if err != nil {
		return "", "", err
	}
	if tr.Outcome != biz.TurnOutcomeCompleted {
		return "", "", nil
	}
	return tr.UserMsg.ID, tr.AssistantMsg.ID, nil
}

// resumeAwaitAfterRestart resumes an await after process restart.
func (o *ChatOrchestrator) resumeAwaitAfterRestart(ctx context.Context, sessionID, reply, runID string) error {
	if !o.awaitCoord().TryBeginResume(sessionID) {
		return errResumeInFlight
	}
	if err := o.runStatus().ClearAwaitingRunStateSync(ctx, sessionID); err != nil {
		o.awaitCoord().EndResume(sessionID)
		return err
	}
	o.awaitCoord().PublishAwaitResumed(sessionID, runID)
	safego.Go(ctx, "chat.resume_await_turn", func() {
		defer o.awaitCoord().EndResume(sessionID)
		bgCtx, cancel := context.WithTimeout(context.Background(), o.turnTimeout())
		defer cancel()
		_, _, turnErr := o.RunNativeAgentTurnFromInput(bgCtx, biz.TurnInput{
			SessionID: sessionID,
			Content:   reply,
		})
		if turnErr != nil && !isTurnMessageQueued(turnErr) {
			o.runStatus().SetRunStatus(bgCtx, sessionID, runID, "failed", turnErr.Error())
			o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
			o.publishTurnFailure(sessionID, runID, "chat-service", turnErr, "")
		}
	})
	return nil
}

// injectA2AContext injects A2A invoker context.
func (o *ChatOrchestrator) injectA2AContext(ctx context.Context, callerAgentID string) context.Context {
	if o == nil || o.a2aUC() == nil {
		return ctx
	}
	inv := a2apkg.NewInvoker(o, o.a2aUC(), o.td().ReadDeps.Agents, o.lg(), a2abiz.DefaultRetryPolicy())
	return a2apkg.InjectRunContext(ctx, o.a2aUC(), callerAgentID, inv, o.lg())
}

// processPendingQueue handles the next pending message after a turn completes.
func (o *ChatOrchestrator) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string) {
	entry, ok := o.chatUC.DequeuePendingMessage(sessionID)
	if !ok {
		return
	}
	pendingContent := entry.Content
	pendingEntryID := entry.ID
	pendingEmitter := event.NewFlowLogger(o.td().Pipeline.Bus, o.td().Pipeline.Buffer, sessionID, ag.AgentKey, o.lg())
	pendingEmitter.LogStart("chat.pending_dequeue", "排队消息开始处理", event.P("entry_id", pendingEntryID), event.P("content_len", len(pendingContent)))
	safego.Go(appctx.Ctx(), "pending-queue", func() {
		unlock := o.lockSession(sessionID)
		defer unlock()
		if o.runs.HasActive(sessionID) {
			o.chatUC.EnqueuePendingMessage(sessionID, pendingContent)
			pendingEmitter.Log("chat.pending_dequeue", event.FlowPhaseDone, "会话仍活跃，消息已重新入队", event.P("entry_id", pendingEntryID))
			return
		}
		bgCtx, cancel := context.WithTimeout(appctx.Ctx(), o.turnTimeout())
		o.runs.SetPendingCancel(sessionID, cancel)
		defer func() {
			cancel()
			o.runs.ClearPendingCancel(sessionID)
		}()
		pendingInput := biz.TurnInput{
			SessionID: sessionID,
			Content:   pendingContent,
		}
		var err error
		if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
			o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusRunning, "")
			_, _, err = o.team().TeamsNative.RunTurnFromInput(bgCtx, sess, pendingInput)
			spiritSessionID := strings.TrimSpace(sess.ParentSessionID)
			teamID := strings.TrimSpace(sess.TeamID)
			if err != nil {
				o.publishTurnFailure(sessionID, "", "pending-queue", err, pendingEntryID)
				o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
				if spiritSessionID != "" && teamID != "" {
					o.teamStarter().HandleTeamTurnResult(bgCtx, spiritSessionID, teamID, "failed", err.Error())
				}
			} else {
				o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusCompleted, "")
				if spiritSessionID != "" && teamID != "" {
					o.teamStarter().HandleTeamTurnResult(bgCtx, spiritSessionID, teamID, "completed", "")
				}
			}
		} else {
			_, _, err = o.runSingleAgentViaTRPC(bgCtx, sess, pendingInput, ag, dialogMode, prov, mod)
			if err != nil {
				o.publishTurnFailure(sessionID, "", "pending-queue", err, pendingEntryID)
			}
		}
		if err != nil {
			pendingEmitter.LogError("chat.pending_dequeue", "排队消息处理失败", event.P("entry_id", pendingEntryID), event.P("error", err.Error()))
		} else {
			pendingEmitter.LogDone("chat.pending_dequeue", "排队消息处理完成", event.P("entry_id", pendingEntryID))
		}
	})
}
