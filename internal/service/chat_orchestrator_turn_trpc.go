package service

import (
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	"aranea-agents/internal/provider"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/sync/errgroup"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

// runSingleAgentViaTRPC runs a single agent turn via the trpc-agent-go framework.
func (o *ChatOrchestrator) runSingleAgentViaTRPC(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	dialogMode, prov, mod string,
) (biz.ChatMessage, biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	o.publishTurnProgress(ctx, sessionID, "routing", nil)

	admitStart := time.Now()
	admit, err := o.phases().admitTurn(ctx, sess, input, ag, dialogMode, prov, mod)
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

	if err := o.runStatus().SetRunStatus(ctx, sessionID, runID, "running", ""); err != nil {
		o.lg().Warn("set run status failed on early ack",
			loggateway.StepID("chat.turn.early_ack"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(err))
	}
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")

	turnStart := time.Now()
	biz.DefaultTurnCompletionBridge().RegisterTurnStart(sessionID, runID, turnStart)
	turnStatus := "ok"
	var turnErr error
	var resultPromptTok, resultCompletionTok, resultCachedTok int
	var resultCacheWriteTok, resultReasoningTok int
	var resultLastRoundPromptTok, resultLastRoundCompletionTok int
	var resultUsageSource string
	var resultModelCallCount int
	var resultFirstTokenMs int
	var turnErrMsg string
	ctx, traceBridge, _ := startTurnSpan(ctx, "chat.turn", sessionID, ag.AgentKey, runID)
	emitter := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx:       ctx,
		SessionID: sessionID, RunID: runID, AgentKey: ag.AgentKey, AgentID: ag.ID,
		Domain: event.TraceDomainChat, LG: o.lg(),
		Infra: event.NewInfraFromBus(o.core.TD.Pipeline.MonitorEventBus),
	})
	emitter.SetOtelRefs(traceBridge.TraceID(), traceBridge.RootSpanID())
	ctx = event.WithTraceEmitter(ctx, emitter)

	ctx, _ = chatagent.WithContextBudget(ctx)
	ctx = chatagent.WithConfirmWaitAcc(ctx)

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
			resultPromptTok, resultCompletionTok, resultCachedTok, resultCacheWriteTok, resultReasoningTok, resultUsageSource, resultModelCallCount, time.Since(turnStart), turnErrMsg,
			resultFirstTokenMs, chatagent.ConfirmWaitMS(ctx))
		if turnStatus != "ok" && resultPromptTok > 0 {

			o.patchSessionContextUsage(ctx, sessionID, sess, ag, prov, mod, resultLastRoundPromptTok, resultLastRoundCompletionTok)
		}
	}()

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

	content := strings.TrimSpace(input.Content)

	inputRisk := intent.ClassifyInputSafety(content)
	inputRiskFlags := inputRisk.Flags
	if inputRisk.Action == intent.SafetyDeny {
		ctx = biz.ContextWithRouteDecision(ctx, biz.RouteDecision{Lane: biz.RouteLaneRefuse, Reason: "input_safety_deny"})
		event.EmitGate(ctx, o.infraDeps.DecisionCollector, decision.GateDecision{
			TriggerRule: decision.TriggerInputRiskFlagged,
			Outcome:     "blocked",
			Scenario:    "用户输入命中 Deny 级破坏性操作，零 LLM 拒绝",
			Reasoning:   fmt.Sprintf("action=deny hits=%v", inputRisk.Hits),
			GuardName:   "input_safety_scan",
			SessionID:   sessionID,
			Extra:       map[string]any{"action": string(inputRisk.Action), "hits": strings.Join(inputRisk.Hits, ",")},
		})
		return o.failTurn(ctx, sessionID, runID, &turnStatus, &turnErr, &turnErrMsg,
			apierror.Forbidden(apierror.DomainChatNative, intent.SafetyDenyUserMessage))
	}
	if inputRisk.Action == intent.SafetyHITL {
		ctx = biz.ContextWithRouteDecision(ctx, biz.RouteDecision{Lane: biz.RouteLaneHITL, Reason: "input_safety_hitl"})
		event.EmitGate(ctx, o.infraDeps.DecisionCollector, decision.GateDecision{
			TriggerRule: decision.TriggerInputRiskFlagged,
			Outcome:     "tripped",
			Scenario:    "用户输入命中 HITL 级风险扫描（故障注入等，不零 LLM 拒绝）",
			Reasoning:   fmt.Sprintf("action=hitl hits=%v", inputRisk.Hits),
			GuardName:   "input_safety_scan",
			SessionID:   sessionID,
			Extra:       map[string]any{"action": string(inputRisk.Action), "flags": strings.Join(inputRiskFlags, ",")},
		})
	} else if inputRisk.Action == intent.SafetyInform {
		o.lg().Info("input risk shadow hit (not flagged)",
			loggateway.StepID("chat.input_risk.shadow"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("shadow_hits", strings.Join(inputRisk.Hits, ",")),
		)
	}

	prov = admit.provider
	mod = admit.model

	// ── PROACTIVE RECALL (P3-11) ──
	// Surface relevant memories based on conversation context before building
	// the runner. The hits are stored in ctx so the MemoryInject before-model
	// hook can merge them with RecallComposite results. Failures are non-fatal:
	// only a warning is logged and the turn continues without proactive hits.
	//
	// Voice Fast-Path（2026-08-09）：recall 从串行段移入 errgroup 与 BUILD/INTENT
	// 三方并行——真机实测串行 recall 0.3-3.3s 且语音轮次 hits 恒为 0，纯开销。
	// WithProactiveHits 在 eg.Wait() 后注入 ctx：MemoryInject before-model 钩子在
	// 请求时才读取，语义不变；goroutine→Wait 的 happens-before 由 errgroup 保证。
	var buildResult turnBuildResult
	var intentRunOpts []trpcagent.RunOption
	var intentArtifact *intent.Artifact
	var proactiveHits []biz.CompositeRecallHit
	eg, egCtx := errgroup.WithContext(ctx)

	skipIntent, assessLevel, skipReason := shouldSkipIntentPass(o, ctx, ag, input, content)
	if skipIntent {

		if provider.ModelSupportsThinking(ctx, o.td().ReadDeps.LLM, prov, mod, o.lg()) {
			ctx = chatagent.WithThinkingDisabled(ctx)
		}
	}
	if strings.TrimSpace(sess.ForkFromTurnID) != "" {
		ctx = biz.WithForkMemoryPrivate(ctx)
	}

	eg.Go(func() error {
		o.publishTurnProgress(ctx, sessionID, "preparing_tools", nil)
		var buildErr error
		buildResult, buildErr = o.buildTurnRunner(egCtx, sess, ag, admit, emitter, content)
		return buildErr
	})

	if shouldRunProactiveRecall(input) {
		eg.Go(func() error {

			recallCtx, cancel := context.WithTimeout(egCtx, 4*time.Second)
			defer cancel()
			proactiveStart := time.Now()
			o.publishTurnProgress(ctx, sessionID, "recalling", nil)
			proactiveHits = o.runProactiveRecall(recallCtx, sess, ag, content, emitter)
			o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: runProactiveRecall",
				loggateway.StepID("chat.proactive_recall"),
				loggateway.Any("elapsed_ms", time.Since(proactiveStart).Milliseconds()),
				loggateway.Any("hits", len(proactiveHits)))
			return nil
		})
	} else {
		emitter.LogSkip("chat.proactive_recall", "跳过主动召回", event.P("reason", proactiveRecallSkipReason(input)))
	}

	intentOutcome := "skipped"
	intentStart := time.Now()
	var intentCh chan *intent.Artifact
	var intentCtx context.Context
	var intentCancel context.CancelFunc
	if reusedArt := intent.ArtifactFromContext(ctx); reusedArt != nil {
		intentArtifact = reusedArt.CloneWithoutClarification()
		intentOutcome = "reused"
		emitter.LogDone("chat.intent.pass", "意图识别复用澄清前产物",
			event.P("outcome", "reused"),
			event.P("intent_kind", intentArtifact.IntentKind),
			event.P("refined_goal_len", len(intentArtifact.RefinedGoal)))
	} else if specArt := intent.SpeculativeArtifactFromContext(ctx); specArt != nil {

		intentArtifact = specArt
		intentOutcome = "reused_speculative"
		emitter.LogDone("chat.intent.pass", "意图识别复用投机产物",
			event.P("outcome", "reused_speculative"),
			event.P("intent_kind", intentArtifact.IntentKind),
			event.P("refined_goal_len", len(intentArtifact.RefinedGoal)))
	} else if skipIntent {
		emitter.LogSkip("chat.intent.pass", "闲聊/简单轮次跳过意图识别", event.P("reason", "direct_reply_or_simple"))
	} else if !biz.IsA2AProxyAgent(ag) && intent.ShouldRun(ag, content) {

		intentCh = make(chan *intent.Artifact, 1)
		intentCtx, intentCancel = context.WithTimeout(ctx, 2*time.Second+500*time.Millisecond)
		intentOutcome = "running"
		safego.Go(intentCtx, "chat.intent.pass", func() {
			o.publishTurnProgress(ctx, sessionID, "understanding", nil)
			intentCh <- o.runIntentPass(intentCtx, ag, sessionID, content, prov, mod, emitter)
		})

		defer func() { intentCancel() }()
	} else if biz.IsA2AProxyAgent(ag) {
		emitter.LogSkip("chat.intent.pass", "A2A Proxy Agent 跳过意图识别", event.P("agent_kind", ag.Kind))
	} else {
		emitter.LogSkip("chat.intent.pass", "Intent Pass 未启用", event.P("intent_pass_enabled", intent.IntentPassFromAgent(ag)))
	}

	buildIntentStart := time.Now()
	if err := eg.Wait(); err != nil {
		if intentCancel != nil {
			intentCancel()
		}
		return o.failTurn(ctx, sessionID, runID, &turnStatus, &turnErr, &turnErrMsg, err,
			withBeforePublish(func() { o.runs.Finish(sessionID, runID) }))
	}

	if intentCh != nil {
		fullWait := intent.LooksLikeUnderspecifiedTask(content) || assessLevel == biz.ComplexityComplex
		art, outcome := awaitIntentArtifact(intentCh, intentCtx, intentCancel, fullWait)
		if art != nil {
			intentArtifact = art
		}
		intentOutcome = outcome
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: BUILD+IntentPass parallel",
		loggateway.StepID("chat.build_intent_parallel"),
		loggateway.Any("elapsed_ms", time.Since(buildIntentStart).Milliseconds()),
		loggateway.Any("intent_outcome", intentOutcome),
		loggateway.Any("intent_elapsed_ms", time.Since(intentStart).Milliseconds()))

	ctx = chatagent.WithProactiveHits(ctx, proactiveHits)
	ctx = chatagent.WithTurnCuePrefetch(ctx, buildResult.prefetch)

	preGeneratedTaskID := chatagent.RootTaskActivityID(preAssignedMessageIDFromCtx(ctx))
	if preGeneratedTaskID == "" {
		preGeneratedTaskID = resolveRootTaskActivityID(input)
	}
	ctx = chatagent.ContextWithRootTaskActivityID(ctx, preGeneratedTaskID)

	// Session orchestration phase is resolved from persisted teams before the
	// Spirit LLM runs. Idle still uses ForcePlanning; Ready/Orchestrating/
	// Interrupted keep the current DAG unless the user asked for a new task.
	var orchTurn biz.SpiritTurnOrchestration
	if strings.TrimSpace(ag.AgentKey) == biz.SpiritAgentKey {
		orchTurn = o.resolveSpiritTurnOrchestration(ctx, sessionID)
		ctx = biz.WithSpiritTurnOrchestration(ctx, orchTurn)
	}

	gateStart := time.Now()
	if strings.TrimSpace(input.ParentTaskID) == "" {
		o.publishTurnProgress(ctx, sessionID, "assessing", nil)
	}
	gateDecision, gateErr := o.runPrePlanningGate(ctx, input, intentArtifact)
	if gateErr == nil && gateDecision.ForcePlanning && strings.TrimSpace(ag.AgentKey) == biz.SpiritAgentKey {
		looksNew := o.orchestrationLooksLikeNewTask(ctx, input, intentArtifact)
		if !biz.ShouldForcePlanning(orchTurn.Phase, true, looksNew) {
			gateDecision.ForcePlanning = false
			gateDecision.Reason = fmt.Sprintf("会话阶段 %s：沿用已有编排，不强制规划", orchTurn.Phase)
			o.lg().With(loggateway.SessionID(sessionID)).Info("预规划门控：阶段抑制强制规划",
				loggateway.StepID("chat.pre_planning_gate"),
				loggateway.Str("phase", string(orchTurn.Phase)),
			)
		}
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: runPrePlanningGate",
		loggateway.StepID("chat.pre_planning_gate"),
		loggateway.Any("elapsed_ms", time.Since(gateStart).Milliseconds()),
		loggateway.Any("force_planning", gateErr == nil && gateDecision.ForcePlanning),
		loggateway.Str("orch_phase", string(orchTurn.Phase)),
		loggateway.Any("gate_err", gateErr != nil))
	if gateErr == nil && gateDecision.ForcePlanning {
		emitter.LogDone("chat.pre_planning_gate", "强制规划路径", event.P("complexity_level", string(gateDecision.Level)), event.P("complexity_score", gateDecision.Score), event.P("reason", gateDecision.Reason))

		intentRunOpts = append(intentRunOpts, forcedPlanningRunOption(gateDecision))

		ctx = biz.ContextWithRouteDecision(ctx, gateDecision.Committed)
		ctx = chatagent.ContextWithForcePlanningRoute(ctx, chatagent.ForcePlanningRoute{
			TaskPrompt: forcePlanningTaskPrompt(gateDecision, intentArtifact, input.Content),
			Level:      string(gateDecision.Level),
			Score:      gateDecision.Score,
			Reason:     gateDecision.Reason,
			Mode:       gateDecision.Committed.Mode,
		})
	}

	if biz.IsOrgGovernanceAgent(ag) && biz.HasTaskActionSignal(content) {
		if _, ok := chatagent.ForcePlanningRouteFromCtx(ctx); !ok {
			ctx = chatagent.ContextWithForcePlanningRoute(ctx, chatagent.ForcePlanningRoute{
				TaskPrompt: strings.TrimSpace(content),
				Reason:     "governance_must_not_diy",
			})
		}
	}

	orchDecisionP := []event.Pair{
		event.P("agent_key", ag.AgentKey),
		event.P("query_len", len(content)),
		event.P("skip_intent", skipIntent),
		event.P("skip_reason", skipReason),
		event.P("assess_level", string(assessLevel)),
		event.P("intent_outcome", intentOutcome),
		event.P("intent_kind", intentKindOf(intentArtifact)),
		event.P("orch_phase", string(orchTurn.Phase)),
	}
	if gateErr != nil {
		orchDecisionP = append(orchDecisionP, event.P("gate_error", gateErr.Error()))
	} else {
		orchDecisionP = append(orchDecisionP,
			event.P("gate_level", string(gateDecision.Level)),
			event.P("gate_score", gateDecision.Score),
			event.P("force_planning", gateDecision.ForcePlanning),
			event.P("gate_reason", gateDecision.Reason),
		)
	}
	o.emitOrchDecisionRecord(ctx, sessionID, ag.AgentKey, skipIntent, skipReason, string(assessLevel), intentOutcome, gateErr, gateDecision, orchDecisionP)
	deps := buildResult.deps

	deps.AwaitHook = o.awaitCoord().MakeAwaitReplyFunc(ctx, sessionID, runID)
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

	o.publishTurnProgress(ctx, sessionID, "starting", nil)
	emitter.LogStart("chat.turn.execute", "开始执行对话轮次", event.P("run_id", runID))
	defer func() {
		if turnStatus != "ok" {
			rollbackRunnerSession()

			if !o.runWasCancelled(ctx, sessionID, turnErr) {

				failCtx := context.Background()
				o.setRunStatus(failCtx, sessionID, runID, "failed", safeErrMsgForWS(turnErr))
				o.transitionSessionStatus(failCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
				// E2E-B2: publish task.failed only for genuine failures. On
				// user-cancel the projector already emits the authoritative
				// task.cancelled; publishing a fabricated failed Task here
				// upserts a ghost failure card next to the real cancelled one
				// (two conflicting terminal states for the same turn).
				if turnErr != nil {
					o.publishTurnFailure(sessionID, runID, "chat-service", turnErr, "")
				}
			}
		}
		o.runs.Finish(sessionID, runID)
		runner.Close()

		if !inPendingLoop(ctx) {
			o.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod,
				string(chatagent.RootTaskActivityIDFromCtx(ctx)))
		}
	}()

	var suspended bool
	ctx, input, intentArtifact, intentRunOpts, suspended, _ = o.applyClarificationGatePhase(
		ctx, sessionID, runID, ag, input, intentArtifact, inputRiskFlags, intentRunOpts, emitter)
	if suspended {
		return biz.ChatMessage{}, biz.ChatMessage{}, nil
	}

	turnPhase.Store("executing")
	execResult, err := o.phases().executeTurn(ctx, sess, input, ag, admit, emitter, traceBridge, deps, runner, attachmentRefs, intentRunOpts, turnStart)
	resultModelCallCount = execResult.result.ModelCallCount
	resultFirstTokenMs = execResult.result.FirstTokenMs
	if err != nil {

		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return execResult.userMsg, biz.ChatMessage{}, err
	}
	defer func() {

		if execResult.cancelled && turnErr == nil {
			turnErr = context.Canceled
		}

		if execResult.cancelled && turnStatus == "ok" {
			turnStatus = "cancelled"
		}
		o.sessionRunLC().FinishSessionRunLifecycle(ctx, sessionID, execResult.sessionRunID, turnErr)
	}()

	turnPhase.Store("persisting")
	persistResult, err := o.phases().persistTurn(&ctx, sess, ag, admit, execResult, emitter, turnStart, &turnStatus, &turnErr, &turnErrMsg)
	if err != nil {

		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return execResult.userMsg, biz.ChatMessage{}, err
	}
	resultPromptTok = persistResult.promptTok
	resultCompletionTok = persistResult.completionTok
	resultCachedTok = persistResult.cachedTok
	resultCacheWriteTok = persistResult.cacheWriteTok
	resultReasoningTok = persistResult.reasoningTok
	resultLastRoundPromptTok = persistResult.lastRoundPromptTok
	resultLastRoundCompletionTok = persistResult.lastRoundCompletionTok
	resultUsageSource = persistResult.usageSource

	userMsgPersisted := execResult.userMsgPersisted
	userMsg := execResult.userMsg
	defer func() {
		if userMsgPersisted && turnStatus != "ok" {
			if err := o.td().Sessions.UpdateChatMessageStatus(ctx, sessionID, userMsg.ID, "failed", turnErrMsg); err != nil {
				o.lg().Warn("用户消息失败状态更新失败", loggateway.StepID("chat.user_msg_status_fail"), loggateway.Str("message_id", userMsg.ID), loggateway.Err(err))
			}
		}
	}()

	clarifySuspended := o.maybeSuspendTurnForClarification(ctx, ag, input, admit.provider, admit.model, persistResult.assistantMsg.ContentMarkdown, emitter)

	o.postProcessTurn(ctx, sess, ag, input, admit, execResult, persistResult, emitter, turnStart, turnStatus, clarifySuspended)

	return userMsg, persistResult.assistantMsg, nil
}
