package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	a2apkg "aranea-agents/internal/a2a"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/internal/runtime/lifecycle"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
)

// pendingQueueLoopKey is a context key used to suppress the recursive
// processPendingQueue call inside runSingleAgentViaTRPC / team turn defers
// when the turn is being executed from within the iterative pending-queue
// loop. This converts the previous goroutine-chain "recursion" into a single
// goroutine while-loop, bounded by maxPendingQueueDepth.
type pendingQueueLoopKey struct{}

// contextWithPendingLoop marks ctx as executing inside the pending-queue loop.
// Turns started with this context skip the processPendingQueue call in their
// defer, letting the loop own the queue draining.
func contextWithPendingLoop(ctx context.Context) context.Context {
	return context.WithValue(ctx, pendingQueueLoopKey{}, true)
}

// inPendingLoop reports whether ctx is marked as executing inside the
// pending-queue loop.
func inPendingLoop(ctx context.Context) bool {
	v, _ := ctx.Value(pendingQueueLoopKey{}).(bool)
	return v
}

// maxPendingQueueDepth bounds the number of pending messages processed in a
// single loop iteration. 32 matches MaxPendingPerSession; reaching this limit
// means the user enqueued 32+ messages faster than the agent could process
// them, which is already the queue cap. A notification is emitted so the user
// knows remaining messages stay queued.
const maxPendingQueueDepth = 32

// RunAgentTurn implements a2a.AgentTurnRunner for call_agent and HTTP Invoke dispatch.
func (o *ChatOrchestrator) RunAgentTurn(ctx context.Context, agentID, input string, timeoutSec int) (string, error) {
	if o == nil || o.td().Sessions == nil {
		return "", apierror.Internal(apierror.DomainA2A, "chat service not configured")
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
		o.lg().Warn("a2a create session failed",
			loggateway.StepID("chat_orchestrator.a2a_create_session"),
			loggateway.Err(err))
		return "", apierror.Internal(apierror.DomainA2A, "create session failed")
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
		return "", apierror.Internal(apierror.DomainA2A, "a2a turn did not complete: "+string(tr.Outcome))
	}
	return tr.AssistantMsg.ContentMarkdown, nil
}

// RunEvalAgentTurn runs an evaluation agent turn.
func (o *ChatOrchestrator) RunEvalAgentTurn(ctx context.Context, agentID, input string) (string, error) {
	if o == nil || o.td().Sessions == nil {
		return "", apierror.Internal(apierror.DomainChat, "eval: chat service not configured")
	}
	agentID = strings.TrimSpace(agentID)
	input = strings.TrimSpace(input)
	if agentID == "" || input == "" {
		return "", apierror.BadRequest(apierror.DomainChat, "eval: agent_id and input are required")
	}
	sess, err := o.td().Sessions.Create(ctx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   agentID,
		OwnerType: "agent",
		Title:     fmt.Sprintf("eval-%s", agentID),
		UserID:    "1",
	})
	if err != nil {
		o.lg().Warn("eval create session failed",
			loggateway.StepID("chat_orchestrator.eval_create_session"),
			loggateway.Err(err))
		return "", apierror.Internal(apierror.DomainChat, "eval: create session failed")
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
		// No-Timeout principle (T1.1): no hard turn timeout — the resumed
		// turn runs until completion or user cancel. User cancel is wired
		// via RunNativeAgentTurnFromInput → runSingleAgentViaTRPC →
		// o.runs.StoreCancelable.
		bgCtx, cancel := context.WithCancel(context.Background())
		defer cancel()
		_, _, turnErr := o.RunNativeAgentTurnFromInput(bgCtx, biz.TurnInput{
			SessionID: sessionID,
			Content:   reply,
		})
		if turnErr != nil && !isTurnMessageQueued(turnErr) {
			if serr := o.runStatus().SetRunStatus(bgCtx, sessionID, runID, "failed", turnErr.Error()); serr != nil {
				o.lg().Warn("set run status failed on turn error",
					loggateway.StepID("chat.turn.dispatch_fail"),
					loggateway.Str("session_id", sessionID),
					loggateway.Str("run_id", runID),
					loggateway.Err(serr))
			}
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

// processPendingQueue drains pending messages for a session iteratively in a
// single goroutine. Previously this method spawned a new goroutine per pending
// message (via runSingleAgentViaTRPC's defer → processPendingQueue recursion),
// creating an unbounded goroutine chain. The iterative form uses a while-loop
// bounded by maxPendingQueueDepth, and suppresses the recursive
// processPendingQueue call in runSingleAgentViaTRPC / team turn defers via
// contextWithPendingLoop so the loop owns queue draining.
//
// The session lock is released between iterations so concurrent enqueue
// operations (user sending more messages) aren't blocked while a turn runs.
// After each turn, the loop re-acquires the lock, checks for an active run
// (e.g. user interrupted with "send now"), and dequeues the next message.
//
// 2026-07-04 问题 C2 修复：新增 rootTaskID 参数，将 RootTaskActivityID 注入
// loopCtx，让 pending queue 路径的 HandleTeamTurnResult → publishV2TeamRunCompletion
// 能拿到正确的 rootTaskID（之前 appctx.Ctx() 不携带 RootTaskActivityID，导致
// MemberSession updated 事件的 TaskID 为空）。
func (o *ChatOrchestrator) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string, rootTaskID string) {
	safego.GoBackground("pending-queue", func() {
		// Mark the context so turns started from this loop skip the
		// processPendingQueue call in their defer (otherwise each turn would
		// spawn a new goroutine, re-introducing the chain we're eliminating).
		loopCtx := contextWithPendingLoop(appctx.Ctx())
		// 2026-07-04 问题 C2 修复：注入 RootTaskActivityID 让下游事件能关联到根 Task。
		if rootTaskID != "" {
			loopCtx = chatagent.ContextWithRootTaskActivityID(
				loopCtx, chatagent.RootTaskActivityID(rootTaskID))
		}
		isTeam := strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team")
		spiritSessionID := strings.TrimSpace(sess.ParentSessionID)
		teamID := strings.TrimSpace(sess.TeamID)

		for depth := 0; depth < maxPendingQueueDepth; depth++ {
			// Acquire session lock and atomically peek + check + dequeue.
			// This eliminates the TOCTOU window between the original Dequeue
			// (outside lock) and HasActive check (inside lock), and avoids the
			// dequeue-requeue pattern that lost the message's original position.
			unlock := o.lockSession(sessionID)
			entry, ok := o.chatUC.PeekPendingMessage(sessionID)
			if !ok {
				unlock()
				return // queue empty
			}
			if o.runs.HasActive(sessionID) {
				// Another turn is active (e.g. user used "send now" to
				// interrupt). Leave the message at the head of the queue
				// (preserving its original position/priority) and stop the
				// loop; the active turn's defer will re-enter processPendingQueue.
				unlock()
				return
			}
			// Dequeue is guaranteed to return the same head we just peeked:
			// we hold the session lock, and the only dequeue path for this
			// session is this loop (recursive processPendingQueue calls are
			// suppressed by loopCtx via contextWithPendingLoop).
			entry, ok = o.chatUC.DequeuePendingMessage(sessionID)
			if !ok {
				unlock()
				o.lg().Warn("dequeue pending message failed after peek",
					loggateway.StepID("chat.pending_queue.dequeue_fail"),
					loggateway.Str("session_id", sessionID),
					loggateway.Int("depth", depth))
				return
			}
			bgCtx, cancel := context.WithCancel(loopCtx)
			o.runs.SetPendingCancel(sessionID, cancel)
			unlock()

			pendingContent := entry.Content
			pendingEntryID := entry.ID
			pendingEmitter := event.NewFlowLogger(sessionID, ag.AgentKey, o.lg())
			pendingEmitter.LogStart("chat.pending_dequeue", "排队消息开始处理",
				event.P("entry_id", pendingEntryID),
				event.P("content_len", len(pendingContent)),
				event.P("depth", depth))

			pendingInput := biz.TurnInput{
				SessionID: sessionID,
				Content:   pendingContent,
			}
			var err error
			if isTeam {
				o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusRunning, "")
				_, _, err = o.team().TeamsNative.RunTurnFromInput(bgCtx, sess, pendingInput)
				if err != nil {
					o.publishTurnFailure(sessionID, "", "pending-queue", err, pendingEntryID)
					o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
					if spiritSessionID != "" && teamID != "" {
						o.teamStarter().HandleTeamTurnResult(bgCtx, spiritSessionID, teamID, "failed", err.Error(), sessionID)
					}
				} else {
					o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusCompleted, "")
					if spiritSessionID != "" && teamID != "" {
						o.teamStarter().HandleTeamTurnResult(bgCtx, spiritSessionID, teamID, "completed", "", sessionID)
					}
				}
			} else {
				_, _, err = o.runSingleAgentViaTRPC(bgCtx, sess, pendingInput, ag, dialogMode, prov, mod)
				if err != nil {
					o.publishTurnFailure(sessionID, "", "pending-queue", err, pendingEntryID)
				}
			}
			cancel()
			o.runs.ClearPendingCancel(sessionID)

			if err != nil {
				pendingEmitter.LogError("chat.pending_dequeue", "排队消息处理失败",
					event.P("entry_id", pendingEntryID),
					event.P("error", err.Error()))
				// A4: Enqueue failed pending message to dead-letter queue so
				// operators can inspect/retry/discard via admin API instead of
				// silent loss. When DLQ is nil (not configured), degrade to
				// legacy behavior (log only).
				if dlq := o.deadLetterQueue(); dlq != nil {
					dlq.Enqueue(lifecycle.DeadLetterMessage{
						ID:         uuid.NewString(),
						Source:     "pending-queue",
						Original:   pendingContent,
						Error:      err.Error(),
						MaxRetries: 3,
					})
				}
			} else {
				pendingEmitter.LogDone("chat.pending_dequeue", "排队消息处理完成",
					event.P("entry_id", pendingEntryID))
			}
		}
		// Loop exited due to depth cap. Remaining messages (if any) stay
		// queued and will be picked up by the next processPendingQueue
		// trigger (e.g. when the user sends another message or the active
		// turn completes). Log so operators can spot sustained overload.
		if remaining := len(o.chatUC.GetPendingMessages(sessionID)); remaining > 0 {
			o.lg().With(loggateway.SessionID(sessionID)).Warn("pending queue reached depth cap; remaining messages stay queued",
				loggateway.StepID("chat.pending_queue.depth_cap"),
				loggateway.Int("cap", maxPendingQueueDepth),
				loggateway.Int("remaining", remaining))
		}
	})
}
