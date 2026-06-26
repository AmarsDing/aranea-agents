// TECH-DEBT(COG): phase methods extracted to chat_orchestrator_turn_phases.go;
// dispatch methods to chat_orchestrator_turn_dispatch.go;
// API methods to chat_orchestrator_turn_api.go;
// metrics methods to chat_orchestrator_turn_metrics.go
package service

import (
	"context"
	"strings"
	"sync/atomic"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/turn"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"
)

// RunNativeAgentTurnFromInput executes a full agent/team turn from a biz-level TurnInput.
func (o *ChatOrchestrator) RunNativeAgentTurnFromInput(ctx context.Context, input biz.TurnInput) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	result, err := o.RunNativeAgentTurnWithOutcome(ctx, input)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	return result.UserMsg, result.AssistantMsg, nil
}

// RunNativeAgentTurnWithOutcome classifies completed vs queued turns explicitly (P1).
func (o *ChatOrchestrator) RunNativeAgentTurnWithOutcome(ctx context.Context, input biz.TurnInput) (biz.TurnResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	if sessionID == "" || content == "" {
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed}, apierror.BadRequest(apierror.DomainChatNative, "session_id and content are required")
	}

	// Ensure spirit_trace_id is present in context for all Spirit orchestration paths.
	// If not already set (e.g., by TaskPlanner), generate one at the turn entry point.
	if _, ok := biz.SpiritTraceIDFromContext(ctx); !ok {
		ctx = biz.ContextWithSpiritTraceID(ctx, biz.NewSpiritTraceID())
	}

	if ep := strings.TrimSpace(string(input.EntryConfig.EntryPoint)); ep != "" {
		ctx = event.WithEnvelopeSource(ctx, ep)
	}

	flow := event.NewFlowLogger(o.td().Pipeline.Bus, sessionID, "", o.lg())
	flow.LogStart("chat.receive", "收到用户消息", event.P("content_len", len(content)))

	hasActive := o.runs.HasActive(sessionID)
	flow.Log("chat.active_check", event.FlowPhaseDone, "检查活跃运行", event.P("has_active", hasActive))
	contextPressure := o.sessionContextPressure(ctx, input)
	if verdict, handled := o.checkTurnAdmission(input, hasActive, contextPressure); handled {
		return turnResultFromAdmissionVerdict(verdict)
	}

	userMsg, assistantMsg, err := o.runNativeAgentTurnBody(ctx, input, flow)
	if err != nil {
		if isTurnMessageQueued(err) {
			return biz.TurnResult{
				Outcome:   biz.TurnOutcomeQueued,
				PendingID: o.LastPendingMessageID(sessionID),
			}, err
		}
		return biz.TurnResult{Outcome: biz.TurnOutcomeFailed, UserMsg: userMsg}, err
	}
	return biz.TurnResult{
		Outcome:      biz.TurnOutcomeCompleted,
		UserMsg:      userMsg,
		AssistantMsg: assistantMsg,
		Reply:        assistantMsg.ContentMarkdown,
	}, nil
}

// TECH-DEBT(BA4): checkTurnAdmission still encodes the reject_busy_queue
// business rule (IngressQueue + intent check) at the service layer. Moving
// this into TurnAdmissionUsecase requires expanding the admission verdict
// model and touching multiple callers (chat_native.go, chat_orchestrator_turn.go).
// Deferred to avoid cross-module churn in this P1 pass.
func (o *ChatOrchestrator) checkTurnAdmission(input biz.TurnInput, hasActive, contextPressure bool) (turn.AdmissionVerdict, bool) {
	if o == nil || o.admitGate() == nil || !hasActive {
		return turn.AdmissionVerdict{}, false
	}
	hasRunner := o.HasActiveRunner(input.SessionID)
	policy := ingressPolicyFromTurnInput(input, true, hasRunner, contextPressure)
	recordIngressIntentMetric(policy.Intent)
	if policy.Decision == IngressQueue && policy.Intent == "reject_busy_queue" {
		return turn.AdmissionVerdict{Action: turn.AdmissionRejectBusy}, true
	}
	verdict := o.admitGate().Check(input)
	switch verdict.Action {
	case turn.AdmissionProceed:
		return verdict, false
	default:
		return verdict, true
	}
}

// runNativeAgentTurnBody executes agent/team turn after admission checks.
func (o *ChatOrchestrator) runNativeAgentTurnBody(ctx context.Context, input biz.TurnInput, flow *event.TraceEmitter) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	sessionID := strings.TrimSpace(input.SessionID)

	lockStart := time.Now()
	unlock := o.lockSession(sessionID)
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: lockSession",
		loggateway.StepID("chat.lock_session"),
		loggateway.Any("elapsed_ms", time.Since(lockStart).Milliseconds()))
	sessGetStart := time.Now()
	sess, err := o.td().Sessions.Get(ctx, sessionID)
	if err != nil {
		unlock()
		flow.LogError("chat.session_fetch", "获取会话失败", event.P("error", err.Error()))
		o.lg().With(loggateway.SessionID(sessionID)).Info("runNativeAgentTurnBody: Sessions.Get 失败",
			loggateway.StepID("chat.session_get_fail"), loggateway.Err(err))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.ChatMessage{}, biz.ChatMessage{}, apierror.NotFound(apierror.DomainSession, "session not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: Sessions.Get",
		loggateway.StepID("chat.session_get"),
		loggateway.Any("elapsed_ms", time.Since(sessGetStart).Milliseconds()))
	flow.LogDone("chat.session_fetch", "会话已获取", event.P("owner_type", sess.OwnerType), event.P("agent_id", sess.AgentID), event.P("team_id", sess.TeamID))

	releaseLane := rt.AcquireTurnLane(ctx, input, sess.OwnerType)
	defer releaseLane()

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return o.executeTeamTurnViaHooks(ctx, sess, input, flow, unlock)
	}

	if rtid := strings.TrimSpace(input.TeamID); rtid != "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Forbidden(apierror.DomainChatTeamNative, "team_id is only valid for team sessions")
	}

	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.BadRequest(apierror.DomainChatNative, "session has no agent_id")
	}
	hydrateStart := time.Now()
	ag, err := o.hydratedAgent(ctx, agentID)
	if err != nil {
		unlock()
		flow.LogError("chat.agent_hydrate", "加载Agent配置失败", event.P("agent_id", agentID), event.P("error", err.Error()))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.ChatMessage{}, biz.ChatMessage{}, apierror.NotFound(apierror.DomainAgent, "agent not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: hydratedAgent",
		loggateway.StepID("chat.agent_hydrate"),
		loggateway.Any("elapsed_ms", time.Since(hydrateStart).Milliseconds()),
		loggateway.Any("agent_key", ag.AgentKey))
	flow.LogDone("chat.agent_hydrate", "Agent配置已加载", event.P("agent_key", ag.AgentKey), event.P("provider", ag.Provider), event.P("model", ag.Model))
	if err := o.admission().EnforceChatTurnQuotas(ctx, agentID, chatagent.UserIDFromCtx(ctx)); err != nil {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	dialogMode := strings.TrimSpace(input.Options.DialogMode)
	prov := strings.TrimSpace(input.Options.Provider)
	mod := strings.TrimSpace(input.Options.Model)
	dialogMode = strutil.FirstNonEmpty(dialogMode, sess.DialogMode, "default")
	prov = strutil.FirstNonEmpty(prov, sess.DefaultProvider, ag.Provider)
	mod = strutil.FirstNonEmpty(mod, sess.DefaultModel, ag.Model)
	prov, mod = o.resolveProviderModelFallback(ctx, prov, mod)
	o.syncSessionProviderModel(ctx, sessionID, sess, prov, mod)
	flow.LogDone("chat.provider_resolve", "Provider/Model已解析", event.P("provider", prov), event.P("model", mod), event.P("dialog_mode", dialogMode))

	attN := len(artifactbiz.NormalizeAttachmentIDs(input.Options.AttachmentIDs))

	flow.LogStart("chat.turn.enter", "进入Agent Turn执行", event.P("dialog_mode", dialogMode), event.P("provider", prov), event.P("model", mod), event.P("attachments", attN))

	agentRunID := uuid.NewString()
	turnCtx, turnCancel := context.WithCancel(ctx)
	o.runs.StoreCancelable(sessionID, agentRunID, turnCancel)
	unlock()
	return o.runSingleAgentViaTRPC(turnCtx, sess, input, ag, dialogMode, prov, mod)
}

func (o *ChatOrchestrator) resolveProviderModelFallback(ctx context.Context, prov, mod string) (string, string) {
	// BA4: business rule (RefineLLM → LLM catalog fallback) lives in biz layer.
	resolvedProv, resolvedMod, _ := o.chatUC.ResolveProviderModel(ctx, prov, mod)
	return resolvedProv, resolvedMod
}

func (o *ChatOrchestrator) syncSessionProviderModel(ctx context.Context, sessionID string, sess biz.Session, prov, mod string) {
	// BA4: business rule (sync session defaults when resolved values differ)
	// lives in biz layer. Error is logged inside the biz method.
	if err := o.chatUC.SyncSessionProviderModel(ctx, sessionID, sess, prov, mod); err != nil {
		o.lg().Debug("SyncSessionProviderModel failed",
			loggateway.StepID("chat_orchestrator.sync_provider_model"),
			loggateway.SessionID(sessionID),
			loggateway.Err(err))
	}
}

// hydratedAgent loads and returns an Agent by ID.
func (o *ChatOrchestrator) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return biz.Agent{}, apierror.BadRequest(apierror.DomainChatNative, "agent id is required")
	}
	if o.td().ReadDeps.AgentsUC != nil {
		return o.td().ReadDeps.AgentsUC.Get(ctx, agentID)
	}
	if o.td().ReadDeps.Agents == nil {
		return biz.Agent{}, apierror.Internal(apierror.DomainChatNative, "agent repository not configured")
	}
	return o.td().ReadDeps.Agents.GetAgentByID(ctx, agentID)
}

// runSingleAgentViaTRPC runs a single agent turn via the trpc-agent-go framework.
func (o *ChatOrchestrator) runSingleAgentViaTRPC(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	dialogMode, prov, mod string,
) (biz.ChatMessage, biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(input.SessionID)

	// ── ADMISSION ──
	admitStart := time.Now()
	admit, err := o.admitTurn(ctx, sess, input, ag, dialogMode, prov, mod)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: admitTurn",
		loggateway.StepID("chat.admit"),
		loggateway.Any("elapsed_ms", time.Since(admitStart).Milliseconds()),
		loggateway.Any("run_id", admit.runID))
	runID := admit.runID
	dialogMode = admit.dialogMode
	prov = admit.provider
	mod = admit.model

	// ── EARLY ACK ──
	// Signal "running" to the frontend immediately after admission so the
	// client-side 30s Turn ACK timeout is cleared before the BUILD phase.
	// Previously this was sent after BUILD, which could take 2-15s on cache
	// miss, causing the frontend to time out.
	if err := o.runStatus().SetRunStatus(ctx, sessionID, runID, "running", ""); err != nil {
		o.lg().Warn("set run status failed on early ack",
			loggateway.StepID("chat.turn.early_ack"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")

	// ── BUILD (inline — defer interactions prevent extraction) ──
	turnStart := time.Now()
	biz.DefaultTurnCompletionBridge().RegisterTurnStart(sessionID, runID, turnStart)
	turnStatus := "ok"
	var turnErr error
	var resultPromptTok, resultCompletionTok int
	var turnErrMsg string
	ctx, traceBridge, _ := startTurnSpan(ctx, "chat.turn", sessionID, ag.AgentKey, runID)
	emitter := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx: ctx, Bus: o.td().Pipeline.Bus,
		SessionID: sessionID, RunID: runID, AgentKey: ag.AgentKey, AgentID: ag.ID,
		Domain: event.TraceDomainChat, LG: o.lg(),
	})
	emitter.SetOtelRefs(traceBridge.TraceID(), traceBridge.RootSpanID())
	ctx = event.WithTraceEmitter(ctx, emitter)

	// P1-7: Start the Wire-injected run heartbeat emitter so the frontend
	// can detect stale runs within 30s. The stop function is invoked in the
	// defer below to avoid goroutine leaks. When the emitter is not wired
	// (nil), heartbeats are skipped and stale detection degrades gracefully.
	//
	// The progress closure reports the current turn phase (building/executing/
	// persisting) via an atomic.Value so the heartbeat goroutine can read it
	// concurrently without locks.
	var turnPhase atomic.Value
	turnPhase.Store("building")
	progress := func() RunProgress {
		step, _ := turnPhase.Load().(string)
		return RunProgress{CurrentStep: step}
	}

	var stopHeartbeat func()
	if hb := o.heartbeatEmitter(); hb != nil {
		stopHeartbeat = hb.Start(ctx, runID, sessionID, progress)
	} else {
		stopHeartbeat = func() {}
	}

	defer func() {
		stopHeartbeat()
		emitter.FinishRoot(turnStatus)
		endTurnSpan(traceBridge, turnErr)
		o.recordTurnUsage(ctx, emitter, sessionID, runID, ag.AgentKey, ag.ID, prov, mod, turnStatus,
			resultPromptTok, resultCompletionTok, time.Since(turnStart), turnErrMsg)
		if turnStatus != "ok" && resultPromptTok > 0 {
			o.patchSessionContextUsage(ctx, sessionID, sess, ag, prov, mod, resultPromptTok, resultCompletionTok)
		}
	}()

	// No-Timeout principle (2026-06-18): long tasks run until completion or
	// user cancel. The previous 24h hard deadline (longTaskHardDeadline)
	// blocked tasks longer than 24h, contradicting the "no time limit"
	// requirement. LLM/DB transient failures are now recovered via
	// automatic retry (see RetryTransport and ExecInTxWithRetry); process
	// crashes are recovered via CheckpointSaver + RecoveryWorker. The
	// heartbeat emitter (10s) keeps the frontend informed of progress
	// without imposing a timeout. User can cancel at any time via WS
	// `cancel` command.

	attachmentRefs, err := o.resolveUserAttachmentRefs(ctx, sessionID, input.Options.AttachmentIDs)
	if err != nil {
		return o.failTurn(ctx, sessionID, runID, &turnStatus, &turnErr, &turnErrMsg, err)
	}
	if err := o.validateTurnAttachmentCapabilities(ctx, prov, mod, attachmentRefs); err != nil {
		return o.failTurn(ctx, sessionID, runID, &turnStatus, &turnErr, &turnErrMsg, err,
			withBeforePublish(func() {
				emitter.LogWarn("chat.attachment.preflight", "模型不支持当前附件类型", "", event.P("provider", prov), event.P("model", mod), event.P("error", err.Error()))
			}))
	}

	// ── BUILD + INTENT PASS (parallel) ──
	// Run BUILD and Intent Pass concurrently since they have no data
	// dependency on each other. This saves 0.5-3s when Intent Pass is enabled.
	content := strings.TrimSpace(input.Content)
	prov = admit.provider
	mod = admit.model

	// ── PROACTIVE RECALL (P3-11) ──
	// Surface relevant memories based on conversation context before building
	// the runner. The hits are stored in ctx so the MemoryInject before-model
	// hook can merge them with RecallComposite results. Failures are non-fatal:
	// only a warning is logged and the turn continues without proactive hits.
	proactiveStart := time.Now()
	proactiveHits := o.runProactiveRecall(ctx, sess, ag, content, emitter)
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: runProactiveRecall",
		loggateway.StepID("chat.proactive_recall"),
		loggateway.Any("elapsed_ms", time.Since(proactiveStart).Milliseconds()),
		loggateway.Any("hits", len(proactiveHits)))
	ctx = chatagent.WithProactiveHits(ctx, proactiveHits)

	var buildResult turnBuildResult
	var intentRunOpts []trpcagent.RunOption
	var intentArtifact *intent.Artifact
	eg, egCtx := errgroup.WithContext(ctx)

	// Goroutine 1: BUILD
	// P1-8: CheckpointSaver is force-enabled for all graph-based Runs at the
	// graph builder factory (internal/graph/adapter.runtime_adapter.createAgent),
	// so every Run persists checkpoints and can be recovered by RecoveryWorker
	// after a process restart.
	eg.Go(func() error {
		var buildErr error
		buildResult, buildErr = o.buildTurnRunner(egCtx, sess, ag, admit, emitter)
		return buildErr
	})

	// Goroutine 2: Intent Pass (only for non-A2A agents with intent enabled)
	if !biz.IsA2AProxyAgent(ag) && intent.ShouldRun(ag, content) {
		eg.Go(func() error {
			intentRunOpts, intentArtifact = o.runIntentPass(egCtx, ag, sessionID, content, prov, mod, emitter)
			return nil // Intent Pass failure is non-fatal; it returns empty opts on error
		})
	} else if biz.IsA2AProxyAgent(ag) {
		emitter.LogSkip("chat.intent.pass", "A2A Proxy Agent 跳过意图识别", event.P("agent_kind", ag.Kind))
	} else {
		emitter.LogSkip("chat.intent.pass", "Intent Pass 未启用或消息过短", event.P("intent_pass_enabled", intent.IntentPassFromAgent(ag)))
	}

	buildIntentStart := time.Now()
	if err := eg.Wait(); err != nil {
		return o.failTurn(ctx, sessionID, runID, &turnStatus, &turnErr, &turnErrMsg, err,
			withBeforePublish(func() { o.runs.Finish(sessionID) }))
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: BUILD+IntentPass parallel",
		loggateway.StepID("chat.build_intent_parallel"),
		loggateway.Any("elapsed_ms", time.Since(buildIntentStart).Milliseconds()))

	// ── PRE-PLANNING GATE (P1-2) ──
	// After intent pass, run a quick complexity assessment. If Moderate/Complex,
	// force the planning path by injecting a system instruction.
	//
	// P1 fix (2026-06-18): Upgraded from soft gate to hard gate. When
	// ForcePlanning=true, the Service layer directly calls TaskPlanner.Plan()
	// to create and persist a plan, rather than relying on the LLM to
	// voluntarily invoke plan_and_execute. This ensures complex tasks always
	// go through planning. The forcedPlanningRunOption is still injected as a
	// hint to the LLM. If Plan() fails, we fall back to the soft gate.
	gateStart := time.Now()
	gateDecision, gateErr := o.runPrePlanningGate(ctx, sessionID, content, intentArtifact)
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: runPrePlanningGate",
		loggateway.StepID("chat.pre_planning_gate"),
		loggateway.Any("elapsed_ms", time.Since(gateStart).Milliseconds()),
		loggateway.Any("force_planning", gateErr == nil && gateDecision.ForcePlanning),
		loggateway.Any("gate_err", gateErr != nil))
	if gateErr == nil && gateDecision.ForcePlanning {
		emitter.LogDone("chat.pre_planning_gate", "强制规划路径", event.P("complexity_level", string(gateDecision.Level)), event.P("complexity_score", gateDecision.Score), event.P("reason", gateDecision.Reason))

		// Hard gate: directly invoke TaskPlanner.Plan() to create and persist
		// the plan. This guarantees a plan exists even if the LLM later
		// ignores the forcedPlanningRunOption hint.
		if planner := o.team().TaskPlanner; planner != nil {
			planInput := biz.PlanInput{
				UserMessage:     content,
				SpiritSessionID: sessionID,
				IntentArtifact:  gateDecision.IntentArtifact,
			}
			if traceID, ok := biz.SpiritTraceIDFromContext(ctx); ok {
				planInput.TraceID = traceID
			}
			// Emit chat-visible progress so the frontend can show
			// "正在创建任务规划" during the potentially multi-second Plan()
			// call. Without this, the user sees a black screen between
			// Intent Pass and the LLM invoke.
			emitter.EmitProgress(ctx, event.StepIDChatPrePlanningGate, "start", "正在创建任务规划", "orchestration",
				event.P("complexity_level", string(gateDecision.Level)),
				event.P("complexity_score", gateDecision.Score))
			if plan, planErr := planner.Plan(ctx, planInput); planErr != nil {
				emitter.LogWarn("chat.pre_planning_gate.hard", "硬门控规划失败，回退到软门控", "",
					event.P("error", planErr.Error()))
				// Emit "done" (not "error") because the hard-gate failure is
				// non-fatal: the turn falls back to the soft gate. Using
				// "error" would surface a failure card for a recoverable
				// degradation.
				emitter.EmitProgress(ctx, event.StepIDChatPrePlanningGate, "done", "任务规划已跳过", "orchestration",
					event.P("skipped_reason", "plan_error"))
			} else if plan != nil {
				emitter.LogDone("chat.pre_planning_gate.hard", "硬门控规划已创建",
					event.P("plan_id", plan.ID),
					event.P("strategy", string(plan.Strategy)),
					event.P("subtask_count", len(plan.SubTasks)))
				emitter.EmitProgress(ctx, event.StepIDChatPrePlanningGate, "done", "任务规划已创建", "orchestration",
					event.P("plan_id", plan.ID),
					event.P("strategy", string(plan.Strategy)),
					event.P("subtask_count", len(plan.SubTasks)))
			} else {
				// plan == nil with no error: planner returned empty. Treat
				// as a completed-but-no-op step so the frontend card closes.
				emitter.EmitProgress(ctx, event.StepIDChatPrePlanningGate, "done", "任务规划已完成", "orchestration")
			}
		}

		intentRunOpts = append(intentRunOpts, forcedPlanningRunOption(gateDecision))
	}
	deps := buildResult.deps
	runner := buildResult.runner

	rollbackDone := false
	rollbackRunnerSession := func() {
		if rollbackDone {
			return
		}
		rollbackDone = true
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer rollbackCancel()
		runnerMgr := o.tdPtr().CoalesceRunnerManager()
		if err := runnerMgr.RollbackToBoundary(rollbackCtx, buildResult.rollbackBoundary); err != nil {
			emitter.LogWarn("chat.runner.rollback", "Runner 会话回滚失败", "", event.P("error", err.Error()))
		}
	}
	// SetRunStatus("running") was already sent in the EARLY ACK section
	// above (right after ADMISSION) to clear the frontend 30s timeout early.
	emitter.LogStart("chat.turn.execute", "开始执行对话轮次", event.P("run_id", runID))
	defer func() {
		if turnStatus != "ok" {
			rollbackRunnerSession()
			// Ensure frontend runStatus is reset to "failed" on EXECUTE/PERSIST
			// errors (e.g. turn timeout). BUILD-phase errors already publish
			// run_status=failed above (line ~386). Without this, the frontend
			// runStatus stays "running" → isActiveRun() returns true → new
			// messages go through enqueueDuringRun instead of direct send →
			// input box never clears (root cause of "no response after send").
			// Use context.Background() because ctx may be cancelled (timeout).
			failCtx := context.Background()
			o.publishRunStatus(sessionID, runID, "failed", safeErrMsgForWS(turnErr))
			o.transitionSessionStatus(failCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
			if turnErr != nil {
				o.publishTurnFailure(sessionID, runID, "chat-service", turnErr, "")
			}
		}
		o.runs.Finish(sessionID)
		runner.Close()
		// Skip processPendingQueue when this turn was started from inside
		// the iterative pending-queue loop (see processPendingQueue). The
		// loop owns draining the queue; re-entering would spawn a new
		// goroutine per message, re-introducing the chain we eliminated.
		if !inPendingLoop(ctx) {
			o.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod)
		}
	}()

	// ── EXECUTE ──
	turnPhase.Store("executing")
	execResult, err := o.executeTurn(ctx, sess, input, ag, admit, emitter, traceBridge, deps, runner, attachmentRefs, intentRunOpts, turnStart)
	if err != nil {
		// EXECUTE phase error: only markTurnError; the defer block above
		// (runSingleAgentViaTRPC L477-503) handles publishRunStatus +
		// transitionSessionStatus + publishTurnFailure.
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return execResult.userMsg, biz.ChatMessage{}, err
	}
	defer func() {
		o.sessionRunLC().FinishSessionRunLifecycle(ctx, sessionID, execResult.sessionRunID, turnErr)
	}()

	// ── PERSIST ──
	turnPhase.Store("persisting")
	persistResult, err := o.persistTurn(&ctx, sess, ag, admit, execResult, emitter, turnStart, &turnStatus, &turnErr, &turnErrMsg)
	if err != nil {
		// PERSIST phase error: only markTurnError; the defer block above
		// (runSingleAgentViaTRPC L477-503) handles publishRunStatus +
		// transitionSessionStatus + publishTurnFailure.
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return execResult.userMsg, biz.ChatMessage{}, err
	}
	resultPromptTok = persistResult.promptTok
	resultCompletionTok = persistResult.completionTok

	// userMsg status rollback on failure
	userMsgPersisted := execResult.userMsgPersisted
	userMsg := execResult.userMsg
	defer func() {
		if userMsgPersisted && turnStatus != "ok" {
			if err := o.td().Sessions.UpdateChatMessageStatus(ctx, sessionID, userMsg.ID, "failed", turnErrMsg); err != nil {
				o.lg().Warn("用户消息失败状态更新失败", loggateway.StepID("chat.user_msg_status_fail"), loggateway.Str("message_id", userMsg.ID), loggateway.Err(err))
			}
		}
	}()

	// ── POST-PROCESS ──
	o.postProcessTurn(ctx, sess, ag, input, admit, execResult, persistResult, emitter, turnStart, turnStatus)

	return userMsg, persistResult.assistantMsg, nil
}

// runProactiveRecall triggers proactive memory recall at turn start to surface
// relevant memories based on the conversation context (P3-11). The returned
// hits are later merged with RecallComposite results by the MemoryInject
// before-model hook.
//
// Failures are non-fatal: only a warning is logged and nil is returned so the
// turn continues without proactive hits. The method is a no-op when memory is
// disabled, the composite recaller is not wired, or the recaller does not
// implement biz.ProactiveRecaller.
func (o *ChatOrchestrator) runProactiveRecall(ctx context.Context, sess biz.Session, ag biz.Agent, content string, emitter *event.TraceEmitter) []biz.CompositeRecallHit {
	policy := biz.ResolveMemoryRuntimePolicy(ag.Settings)
	if !policy.MasterEnabled || !policy.RecallL2 || !policy.InjectL3 {
		return nil
	}
	compositeRecaller := o.td().Persist.Memory.CompositeRecall
	if compositeRecaller == nil {
		return nil
	}
	proactiveRecaller, ok := compositeRecaller.(biz.ProactiveRecaller)
	if !ok || proactiveRecaller == nil {
		return nil
	}
	agentID := strings.TrimSpace(ag.ID)
	if agentID == "" {
		return nil
	}
	userID := strings.TrimSpace(chatagent.UserIDFromCtx(ctx))
	convCtx := biz.ProactiveRecallContext{
		UserStatement:     strutil.TruncateRunes(content, 200),
		MentionedEntities: extractMentionedEntities(content),
	}
	// Emit chat-visible progress so the frontend can show "正在检索相关记忆"
	// during this phase. Only emitted when the recaller is actually wired
	// (the early returns above for disabled policy / nil recaller stay silent
	// to avoid showing a card for a no-op).
	emitter.EmitProgress(ctx, event.StepIDChatProactiveRecall, "start", "正在检索相关记忆", "orchestration",
		event.P("agent_id", agentID))
	hits, err := proactiveRecaller.ProactiveRecall(ctx, agentID, userID, convCtx)
	if err != nil {
		o.lg().Warn("proactive recall failed, continuing without proactive hits",
			loggateway.StepID("chat.proactive_recall"),
			loggateway.SessionID(sess.ID),
			loggateway.Err(err))
		// Emit "done" (not "error") because proactive recall is non-fatal:
		// the turn continues without proactive hits. Using "error" would
		// surface a failure card for a recoverable degradation.
		emitter.EmitProgress(ctx, event.StepIDChatProactiveRecall, "done", "记忆检索已跳过", "orchestration",
			event.P("hit_count", 0),
			event.P("skipped_reason", "recall_error"))
		return nil
	}
	emitter.EmitProgress(ctx, event.StepIDChatProactiveRecall, "done", "记忆检索完成", "orchestration",
		event.P("hit_count", len(hits)))
	if len(hits) > 0 {
		emitter.LogDone("chat.proactive_recall", "主动召回完成",
			event.P("hit_count", len(hits)),
			event.P("agent_id", agentID))
	}
	return hits
}

// extractMentionedEntities extracts candidate entity keywords from the user
// message for proactive recall. This is a simple tokenisation (YAGNI: no NLP,
// no stemming) that splits on whitespace and common punctuation (ASCII + CJK),
// filters out very short tokens, and caps the result to avoid excessive queries.
func extractMentionedEntities(content string) []string {
	content = strings.TrimSpace(content)
	if content == "" {
		return nil
	}
	const (
		minTokenLen = 3
		maxEntities = 8
	)
	fields := strings.FieldsFunc(strings.ToLower(content), func(r rune) bool {
		switch r {
		case ' ', '\t', '\n', ',', '.', '!', '?', ';', ':', '\'', '"':
			return true
		case '，', '。', '！', '？', '；', '：', '、':
			return true
		}
		return false
	})
	out := make([]string, 0, maxEntities)
	seen := make(map[string]bool, len(fields))
	for _, f := range fields {
		if len(f) < minTokenLen || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
		if len(out) >= maxEntities {
			break
		}
	}
	return out
}
