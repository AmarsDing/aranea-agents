package service

import (
	"context"
	"errors"
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
	"aranea-agents/internal/telemetry/turntrace"
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
	turnIn := biz.TurnInput{
		SessionID: sess.ID,
		Content:   input,
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint: biz.EntryPointEvaluation,
			AllowQueue: false,
		},
	}
	if ov, ok := biz.EvalRunOverrideFrom(ctx); ok {
		if ov.Model != "" {
			turnIn.Options.Model = ov.Model
		}
		if ov.Prompt != "" {
			turnIn.Content = biz.OverlayEvalPrompt(ov.Prompt, turnIn.Content)
		}
	}
	_, asst, err := o.RunNativeAgentTurnFromInput(ctx, turnIn)
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

// awaitReplyOutcome classifies how a user's await reply was delivered.
type awaitReplyOutcome int

const (
	// awaitReplyRejected means the reply could not be delivered (channel full,
	// no await route, or a resume already in flight).
	awaitReplyRejected awaitReplyOutcome = iota
	// awaitReplyDelivered means the reply was sent through the in-memory
	// await channel to the blocked run.
	awaitReplyDelivered
	// awaitReplyResumed means the in-memory channel was gone (process
	// restart) and the awaiting run was resumed via the recovery path.
	awaitReplyResumed
)

// awaitReply carries a user reply destined for an awaiting run.
type awaitReply struct {
	runID string
	// token is delivered through the in-memory await channel; it is
	// machine-parsed by the confirmation gate / await_user_reply tool.
	token string
	// resumeContent is used as the turn content when the in-memory channel
	// is gone (process restart) and the run must be resumed. It must be a
	// self-contained natural-language statement of the user's decision so
	// the LLM receives it as meaningful context. Falls back to token.
	resumeContent string
	// toolCallID routes the reply to one specific parallel tool confirmation
	// (BUG-02, chat-e2e-20260823). Empty = session-level slot (free-text).
	toolCallID string
}

// submitAwaitReply is the single delivery path for user replies to an
// awaiting run (tool confirmation, await_user_reply, clarification answers).
//
// Delivery contract, in order:
//  1. Fast path — send the machine token through the in-memory await channel.
//  2. GC race guard — if the channel entry still exists, it is merely full:
//     reject without falling through to resume (would double-deliver).
//  3. Restart recovery — if the entry is gone but the session still persists
//     an awaiting_user route, resume the run with resumeContent as input.
//
// An explicit runID overrides the persisted one. errResumeInFlight maps to
// awaitReplyRejected without error.
func (o *ChatOrchestrator) submitAwaitReply(ctx context.Context, sessionID string, msg awaitReply) (awaitReplyOutcome, error) {
	if o.TrySendAwaitChannelForTool(sessionID, msg.toolCallID, biz.AwaitReplyMsg{RunID: msg.runID, Reply: msg.token, ToolCallID: msg.toolCallID}) {
		return awaitReplyDelivered, nil
	}
	if _, stillExists := o.LoadAwaitChannelForTool(sessionID, msg.toolCallID); stillExists {
		return awaitReplyRejected, nil
	}
	persistedRunID, canResume := o.canResumeAwait(ctx, sessionID)
	if !canResume {
		return awaitReplyRejected, nil
	}
	if trimmed := strings.TrimSpace(msg.runID); trimmed != "" {
		persistedRunID = trimmed
	}
	resumeContent := strings.TrimSpace(msg.resumeContent)
	if resumeContent == "" {
		resumeContent = msg.token
	}
	resumeFn := o.resumeAwaitFn
	if resumeFn == nil {
		resumeFn = o.resumeAwaitAfterRestart
	}
	if err := resumeFn(ctx, sessionID, resumeContent, persistedRunID); err != nil {
		if errors.Is(err, errResumeInFlight) {
			return awaitReplyRejected, nil
		}
		return awaitReplyRejected, err
	}
	return awaitReplyResumed, nil
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
// P0 FR-1 双保险：
//   - single-flight：入口同步 LoadOrStore 合并重复触发（turn defer、
//     drainPendingAfterDispatchTeamTurn、重启惰性排空），杜绝并发排空循环；
//     goroutine 退出时 Delete 并按需接力（仅非深度上限退出）。
//   - 注册先于放锁：每轮 dequeue 后在临界区内 StoreCancelable(regID)，
//     恢复 registration-before-unlock 不变量（对齐正常路径
//     chat_orchestrator_turn.go StoreCancelable→unlock 序），执行窗口内
//     HasActive=true，并发 Execute 走准入（queue/busy）而非双跑。
func (o *ChatOrchestrator) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string, rootTaskID string) {
	if _, loaded := o.pendingLoops.LoadOrStore(sessionID, struct{}{}); loaded {
		return
	}
	safego.GoBackground("pending-queue", func() {
		capExited := false
		// P0 FR-3：迭代执行句柄提到 goroutine 级，panic 时外层 defer 统一
		// 收尾（cancel/ClearPendingCancel/Finish(regID)），防注册项泄漏。
		var iterCancel context.CancelFunc
		var iterRegID string
		defer func() {
			o.pendingLoops.Delete(sessionID)
			if r := recover(); r != nil {
				if iterCancel != nil {
					iterCancel()
					o.runs.ClearPendingCancel(sessionID)
				}
				if iterRegID != "" {
					o.runs.Finish(sessionID, iterRegID)
				}
				o.lg().With(loggateway.SessionID(sessionID)).Error("pending queue loop panicked",
					loggateway.StepID("chat.pending_queue.panic"),
					loggateway.Any("panic", r))
			}
			if capExited {
				return // 深度上限退出：等自然触发，防单会话霸占（原 Warn 语义）
			}
			// 合并吞触发补偿：循环存活期内的再次触发被 single-flight 合并，
			// 退出瞬间新到的消息（含 panic 中断循环剩余的）可能无人驱动——
			// 有可派发内容且无活跃 run 时接力。
			if !o.runs.HasActive(sessionID) {
				if _, _, _, ok := biz.SplitLeadingInjects(o.chatUC.GetPendingMessages(sessionID)); ok {
					o.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod, rootTaskID)
				}
			}
		}()
		// Mark the context so turns started from this loop skip the
		// processPendingQueue call in their defer (otherwise each turn would
		// spawn a new goroutine, re-introducing the chain we're eliminating).
		loopCtx := contextWithPendingLoop(appctx.Ctx())
		if rootTaskID != "" {
			loopCtx = chatagent.ContextWithRootTaskActivityID(
				loopCtx, chatagent.RootTaskActivityID(rootTaskID))
		}
		isTeam := strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team")
		// 2026-07-29 F-1/F-3：standalone（Mode A）团队 ParentSessionID 为空，
		// 回退 team session ID 作为聚合根（与 runner deriveSpiritSessionID
		// 回退 sess.ID 一致）；不再以 ParentSessionID 为空跳过终态 pass。
		spiritSessionID := strings.TrimSpace(sess.ParentSessionID)
		if spiritSessionID == "" {
			spiritSessionID = sessionID
		}
		teamID := strings.TrimSpace(sess.TeamID)

		for depth := 0; depth < maxPendingQueueDepth; depth++ {
			// Acquire session lock and atomically peek + check + dequeue.
			// This eliminates the TOCTOU window between the original Dequeue
			// (outside lock) and HasActive check (inside lock), and avoids the
			// dequeue-requeue pattern that lost the message's original position.
			unlock := o.lockSession(sessionID)
			// P2-3 inject 级：头部连续 inject 条目不单独唤醒 turn，仅作为上下文
			// 前缀合入其后的第一条 followup。仅剩 inject 时保持排队直接返回。
			_, _, leadInjects, ok := biz.SplitLeadingInjects(o.chatUC.GetPendingMessages(sessionID))
			if !ok {
				unlock()
				return // queue empty or only silent inject entries
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
			//
			// P2-3：依次出队 leadInjects 条 inject + 第一条 followup。inject
			// 内容以实际出队条目为准（List 快照与出队之间无锁 Remove/Update
			// 可能并发改队列；逐条校验 kind，意外遇到非 inject 即按 followup
			// 派发，已收集的 inject 仍随其合入——与"上下文跟下一条走"一致）。
			var injects []string
			var entry biz.PendingQueueEntry
			dispatched := false
			for i := 0; i <= leadInjects; i++ {
				e, deqOK := o.chatUC.DequeuePendingMessage(sessionID)
				if !deqOK {
					break
				}
				if i < leadInjects && e.Kind == biz.ChatEnqueueKindInject {
					injects = append(injects, e.Content)
					continue
				}
				entry = e
				dispatched = true
				break
			}
			if !dispatched {
				unlock()
				o.lg().Warn("dequeue pending message failed after peek",
					loggateway.StepID("chat.pending_queue.dequeue_fail"),
					loggateway.Str("session_id", sessionID),
					loggateway.Int("depth", depth))
				return
			}
			bgCtx, cancel := context.WithCancel(loopCtx)
			// Unify trace id for the dequeued turn: the dequeue event and the
			// subsequent team/agent run share one trace id (bgCtx flows into
			// RunTurnFromInput / the single-agent path).
			bgCtx, _ = turntrace.EnsureTraceID(bgCtx)
			// P0 FR-1 注册先于放锁：unlock 前将出队 turn 登记进 RunRegistry
			//（此前仅 SetPendingCancel —— pendingCancels 对 HasActive 不可见，
			// 执行窗口内准入判定 HasActive=false 放行并发 turn 双跑，且双方
			// StoreRunner/Finish CAS 交叉污染）。BUILD 阶段 StoreRunner 经
			// UpdateOrStore 原地升级此槽位并保留 cancel（run_registry.go）。
			regID := uuid.NewString()
			o.runs.StoreCancelable(sessionID, regID, cancel)
			o.runs.SetPendingCancel(sessionID, cancel)
			iterCancel = cancel
			iterRegID = regID
			unlock()

			pendingContent := biz.MergeInjectContext(injects, entry.Content)
			pendingEntryID := entry.ID
			pendingEmitter := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
				Ctx:       bgCtx,
				SessionID: sessionID,
				AgentKey:  ag.AgentKey,
				Domain:    event.TraceDomainChat,
				LG:        o.lg(),
				Infra:     event.NewInfraFromBus(o.core.TD.Pipeline.MonitorEventBus),
			})
			pendingEmitter.LogStart("chat.pending_dequeue", "排队消息开始处理",
				event.P("entry_id", pendingEntryID),
				event.P("content_len", len(pendingContent)),
				event.P("inject_count", len(injects)),
				event.P("depth", depth))

			pendingInput := biz.TurnInput{
				SessionID: sessionID,
				Content:   pendingContent,
			}
			var err error
			// D1-F1（87-orchestration-chain-hardening）：pending 团队分支取消
			// 分流——用户停止不是失败：根 Task cancelled + 会话 interrupted
			// (user_cancelled) + 终态回调 cancelled（WithoutCancel 落账），跳过
			// publishTurnFailure 与 DLQ Enqueue（取消不是失败邮件）。
			pendingCancelled := false
			if isTeam {
				// S-5b（2026-08-05）：pending-queue 团队 turn 与入口路径
				// （executeTeamTurnViaHooks, S-5）同一姿态——每条出队消息是新的
				// run，注入全新 RootTaskActivityID 并幂等建根/终态化。loopCtx
				// 携带的是上一轮 rootTaskID，直接复用会让同团队所有 pending
				// turn 碰撞同一 team_stages_v2 行（S-3 要消除的 bug）。
				pendingRootTaskID := resolveRootTaskActivityID(pendingInput)
				bgCtx = chatagent.ContextWithRootTaskActivityID(bgCtx, pendingRootTaskID)
				o.ensureTeamTurnRootTask(bgCtx, sess, pendingInput, string(pendingRootTaskID))
				o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusRunning, "")
				_, _, err = o.team().TeamsNative.RunTurnFromInput(bgCtx, sess, pendingInput)
				if err != nil && o.runWasCancelled(bgCtx, sessionID, err) {
					pendingCancelled = true
					bookCtx := context.WithoutCancel(bgCtx)
					o.terminalizeTeamTurnRootTask(bookCtx, string(pendingRootTaskID), biz.TaskStatusCancelled)
					o.transitionSessionStatus(bookCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonUserCancelled)
					if spiritSessionID != "" && teamID != "" {
						o.teamStarter().HandleTeamTurnResult(bookCtx, spiritSessionID, teamID, biz.TeamStatusCancelled, err.Error(), sessionID)
					}
				} else if err != nil {
					o.terminalizeTeamTurnRootTask(bgCtx, string(pendingRootTaskID), biz.TaskStatusFailed)
					o.publishTurnFailure(sessionID, "", "pending-queue", err, pendingEntryID)
					o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
					if spiritSessionID != "" && teamID != "" {
						o.teamStarter().HandleTeamTurnResult(bgCtx, spiritSessionID, teamID, "failed", err.Error(), sessionID)
					}
				} else {
					o.terminalizeTeamTurnRootTask(bgCtx, string(pendingRootTaskID), biz.TaskStatusCompleted)
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
			// P0 FR-1/FR-3 迭代收尾：turn 正常完成时 EXECUTE defer 已
			// Finish(真 runID)，此处 CAS 幂等失配跳过；admit 前失败路径
			//（无自身注册）兜底清除 regID。
			o.runs.Finish(sessionID, regID)
			iterCancel = nil
			iterRegID = ""

			if err != nil {
				pendingEmitter.LogError("chat.pending_dequeue", "排队消息处理失败",
					event.P("entry_id", pendingEntryID),
					event.P("error", err.Error()))
				// A4: Enqueue failed pending message to dead-letter queue so
				// operators can inspect/retry/discard via admin API instead of
				// silent loss. When DLQ is nil (not configured), degrade to
				// legacy behavior (log only).
				// D1-F1: 用户取消不是失败邮件——跳过 DLQ（取消 turn 的终态已由
				// cancel 路径落账，重投会把用户明确停止的任务再次执行）。
				if !pendingCancelled {
					if dlq := o.deadLetterQueue(); dlq != nil {
						dlq.Enqueue(lifecycle.DeadLetterMessage{
							ID:         uuid.NewString(),
							Source:     "pending-queue",
							Original:   pendingContent,
							Error:      err.Error(),
							MaxRetries: 3,
						})
					}
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
		capExited = true
		if remaining := len(o.chatUC.GetPendingMessages(sessionID)); remaining > 0 {
			o.lg().With(loggateway.SessionID(sessionID)).Warn("pending queue reached depth cap; remaining messages stay queued",
				loggateway.StepID("chat.pending_queue.depth_cap"),
				loggateway.Int("cap", maxPendingQueueDepth),
				loggateway.Int("remaining", remaining))
		}
	})
}
