// TECH-DEBT(COG): phase methods extracted to chat_orchestrator_turn_phases.go;
// dispatch methods to chat_orchestrator_turn_dispatch.go;
// API methods to chat_orchestrator_turn_api.go;
// metrics methods to chat_orchestrator_turn_metrics.go
package service

import (
	"context"
	"strings"
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
func (o *ChatOrchestrator) RunNativeAgentTurnWithOutcome(ctx context.Context, input biz.TurnInput) (biz.NativeTurnResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	if sessionID == "" || content == "" {
		return biz.NativeTurnResult{}, apierror.BadRequest("CHAT_NATIVE", "session_id and content are required")
	}

	// Ensure spirit_trace_id is present in context for all Spirit orchestration paths.
	// If not already set (e.g., by TaskPlanner), generate one at the turn entry point.
	if _, ok := biz.SpiritTraceIDFromContext(ctx); !ok {
		ctx = biz.ContextWithSpiritTraceID(ctx, biz.NewSpiritTraceID())
	}

	if ep := strings.TrimSpace(string(input.EntryConfig.EntryPoint)); ep != "" {
		ctx = event.WithEnvelopeSource(ctx, ep)
	}

	flow := event.NewFlowLogger(o.td().Pipeline.Bus, o.td().Pipeline.Buffer, sessionID, "", o.lg())
	flow.LogStart("chat.receive", "收到用户消息", event.P("content_len", len(content)))

	hasActive := o.runs.HasActive(sessionID)
	flow.Log("chat.active_check", event.FlowPhaseDone, "检查活跃运行", event.P("has_active", hasActive))
	contextPressure := o.sessionContextPressure(ctx, input)
	if verdict, handled := o.checkTurnAdmission(input, hasActive, contextPressure); handled {
		return nativeResultFromAdmissionVerdict(verdict)
	}

	userMsg, assistantMsg, err := o.runNativeAgentTurnBody(ctx, input, flow)
	if err != nil {
		if isTurnMessageQueued(err) {
			return biz.NativeTurnResult{
				Outcome:   biz.NativeTurnOutcomeQueued,
				PendingID: o.LastPendingMessageID(sessionID),
			}, err
		}
		return biz.NativeTurnResult{Outcome: biz.NativeTurnOutcomeFailed, UserMsg: userMsg}, err
	}
	return biz.NativeTurnResult{
		Outcome:      biz.NativeTurnOutcomeCompleted,
		UserMsg:      userMsg,
		AssistantMsg: assistantMsg,
	}, nil
}

func (o *ChatOrchestrator) checkTurnAdmission(input biz.TurnInput, hasActive, contextPressure bool) (turn.AdmissionVerdict, bool) {
	if o == nil || o.admitGate() == nil || !hasActive {
		return turn.AdmissionVerdict{}, false
	}
	hasRunner := o.HasActiveRunner(input.SessionID)
	policy := ingressPolicyFromTurnInput(input, true, hasRunner, contextPressure)
	recordIngressIntentMetric(policy.Intent)
	if policy.Decision == IngressRejectBusy {
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

	unlock := o.lockSession(sessionID)
	sess, err := o.td().Sessions.Get(ctx, sessionID)
	if err != nil {
		unlock()
		flow.LogError("chat.session_fetch", "获取会话失败", event.P("error", err.Error()))
		o.lg().With(loggateway.SessionID(sessionID)).Info("runNativeAgentTurnBody: Sessions.Get 失败",
			loggateway.StepID("chat.session_get_fail"), loggateway.Err(err))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.ChatMessage{}, biz.ChatMessage{}, apierror.NotFound("SESSION", "session not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.session_fetch", "会话已获取", event.P("owner_type", sess.OwnerType), event.P("agent_id", sess.AgentID), event.P("team_id", sess.TeamID))

	releaseLane := rt.AcquireTurnLane(ctx, input, sess.OwnerType)
	defer releaseLane()

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return o.executeTeamTurnViaHooks(ctx, sess, input, flow, unlock)
	}

	if rtid := strings.TrimSpace(input.TeamID); rtid != "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.Forbidden("CHAT_TEAM_NATIVE", "team_id is only valid for team sessions")
	}

	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, apierror.BadRequest("CHAT_NATIVE", "session has no agent_id")
	}
	ag, err := o.hydratedAgent(ctx, agentID)
	if err != nil {
		unlock()
		flow.LogError("chat.agent_hydrate", "加载Agent配置失败", event.P("agent_id", agentID), event.P("error", err.Error()))
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return biz.ChatMessage{}, biz.ChatMessage{}, apierror.NotFound("AGENT", "agent not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
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
	if prov != "" && mod != "" {
		return prov, mod
	}
	if o.td().ReadDeps.Settings != nil {
		if refine, err := o.td().ReadDeps.Settings.GetRefineLLM(ctx); err == nil {
			prov = strutil.FirstNonEmpty(prov, refine.Provider)
			mod = strutil.FirstNonEmpty(mod, refine.Model)
		}
	}
	if prov != "" && mod != "" {
		return prov, mod
	}
	if o.td().ReadDeps.LLM != nil {
		if models, err := o.td().ReadDeps.LLM.List(ctx); err == nil {
			for _, m := range models {
				if m.Enabled && m.Provider != "" && m.Model != "" {
					prov = strutil.FirstNonEmpty(prov, m.Provider)
					mod = strutil.FirstNonEmpty(mod, m.Model)
					break
				}
			}
		}
	}
	return prov, mod
}

func (o *ChatOrchestrator) syncSessionProviderModel(ctx context.Context, sessionID string, sess biz.Session, prov, mod string) {
	if prov == "" || mod == "" {
		return
	}
	if sess.DefaultProvider == prov && sess.DefaultModel == mod {
		return
	}
	if o.td().Sessions == nil {
		return
	}
	p, m := prov, mod
	if _, err := o.td().Sessions.Update(ctx, sessionID, biz.SessionUpdateFields{
		DefaultProvider: &p,
		DefaultModel:    &m,
	}); err != nil {
		o.lg().Warn("sync session provider model failed", loggateway.Err(err), loggateway.Str("session_id", sessionID))
	}
}

// hydratedAgent loads and returns an Agent by ID.
func (o *ChatOrchestrator) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return biz.Agent{}, apierror.BadRequest("CHAT_NATIVE", "agent id is required")
	}
	if o.td().ReadDeps.AgentsUC != nil {
		return o.td().ReadDeps.AgentsUC.Get(ctx, agentID)
	}
	if o.td().ReadDeps.Agents == nil {
		return biz.Agent{}, apierror.Internal("CHAT_NATIVE", "agent repository not configured")
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
	admit, err := o.admitTurn(ctx, sess, input, ag, dialogMode, prov, mod)
	if err != nil {
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	runID := admit.runID
	dialogMode = admit.dialogMode
	prov = admit.provider
	mod = admit.model

	// ── EARLY ACK ──
	// Signal "running" to the frontend immediately after admission so the
	// client-side 30s Turn ACK timeout is cleared before the BUILD phase.
	// Previously this was sent after BUILD, which could take 2-15s on cache
	// miss, causing the frontend to time out.
	o.runStatus().SetRunStatus(ctx, sessionID, runID, "running", "")
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
		Ctx: ctx, Bus: o.td().Pipeline.Bus, Buffer: o.td().Pipeline.Buffer,
		SessionID: sessionID, RunID: runID, AgentKey: ag.AgentKey, AgentID: ag.ID,
		Domain: event.TraceDomainChat, LG: o.lg(),
	})
	emitter.SetOtelRefs(traceBridge.TraceID(), traceBridge.RootSpanID())
	ctx = event.WithTraceEmitter(ctx, emitter)
	defer func() {
		emitter.FinishRoot(turnStatus)
		endTurnSpan(traceBridge, turnErr)
		o.recordTurnUsage(ctx, emitter, sessionID, runID, ag.AgentKey, ag.ID, prov, mod, turnStatus,
			resultPromptTok, resultCompletionTok, time.Since(turnStart), turnErrMsg)
		if turnStatus != "ok" && resultPromptTok > 0 {
			o.patchSessionContextUsage(ctx, sessionID, sess, ag, prov, mod, resultPromptTok, resultCompletionTok)
		}
	}()

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.turnTimeout())
		defer cancel()
	}

	attachmentRefs, err := o.resolveUserAttachmentRefs(ctx, sessionID, input.Options.AttachmentIDs)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.publishRunStatus(sessionID, runID, "failed", err.Error())
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if err := o.validateTurnAttachmentCapabilities(ctx, prov, mod, attachmentRefs); err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		emitter.LogWarn("chat.attachment.preflight", "模型不支持当前附件类型", "", event.P("provider", prov), event.P("model", mod), event.P("error", err.Error()))
		o.publishRunStatus(sessionID, runID, "failed", err.Error())
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	// ── BUILD + INTENT PASS (parallel) ──
	// Run BUILD and Intent Pass concurrently since they have no data
	// dependency on each other. This saves 0.5-3s when Intent Pass is enabled.
	content := strings.TrimSpace(input.Content)
	prov = admit.provider
	mod = admit.model

	var buildResult turnBuildResult
	var intentRunOpts []trpcagent.RunOption
	eg, egCtx := errgroup.WithContext(ctx)

	// Goroutine 1: BUILD
	eg.Go(func() error {
		var buildErr error
		buildResult, buildErr = o.buildTurnRunner(egCtx, sess, ag, admit, emitter)
		return buildErr
	})

	// Goroutine 2: Intent Pass (only for non-A2A agents with intent enabled)
	if !biz.IsA2AProxyAgent(ag) && intent.ShouldRun(ag, content) {
		eg.Go(func() error {
			intentRunOpts = o.runIntentPass(egCtx, ag, sessionID, content, prov, mod, emitter)
			return nil // Intent Pass failure is non-fatal; it returns empty opts on error
		})
	} else if biz.IsA2AProxyAgent(ag) {
		emitter.LogSkip("chat.intent.pass", "A2A Proxy Agent 跳过意图识别", event.P("agent_kind", ag.Kind))
	} else {
		emitter.LogSkip("chat.intent.pass", "Intent Pass 未启用或消息过短", event.P("intent_pass_enabled", intent.IntentPassFromAgent(ag)))
	}

	if err := eg.Wait(); err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.runs.Finish(sessionID)
		o.publishRunStatus(sessionID, runID, "failed", err.Error())
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
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
		}
		o.runs.Finish(sessionID)
		runner.Close()
		o.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod)
	}()

	// ── EXECUTE ──
	execResult, err := o.executeTurn(ctx, sess, input, ag, admit, emitter, traceBridge, deps, runner, attachmentRefs, intentRunOpts, turnStart)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return execResult.userMsg, biz.ChatMessage{}, err
	}
	defer execResult.stopBudget()
	defer func() {
		o.sessionRunLC().FinishSessionRunLifecycle(ctx, sessionID, execResult.sessionRunID, turnErr)
	}()

	// ── PERSIST ──
	persistResult, err := o.persistTurn(&ctx, sess, ag, admit, execResult, emitter, turnStart, &turnStatus, &turnErr, &turnErrMsg)
	if err != nil {
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
