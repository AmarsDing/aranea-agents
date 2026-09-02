package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/agent/v2"
	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	"aranea-agents/internal/outbound"
	"aranea-agents/internal/provider"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// llmInvokeSlowLogThreshold is the threshold for logging slow LLM invocations.
const llmInvokeSlowLogThreshold = 60 * time.Second

// ────────────────────────────────────────────────────────────
// EXECUTE phase helpers (called by turnPhases.executeTurn)
// ────────────────────────────────────────────────────────────

// prepareTurnUserOptions builds user options with attachment merge.
// Intent Pass has been moved to run in parallel with BUILD (see runSingleAgentViaTRPC).
// Stability:internal
func (o *ChatOrchestrator) prepareTurnUserOptions(
	ctx context.Context,
	input biz.TurnInput,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	attachmentRefs []artifactbiz.Ref,
	sess biz.Session,
) (string, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	runID := admit.runID
	dialogMode := admit.dialogMode
	prov := admit.provider
	mod := admit.model

	userOpts, err := chatagent.UserOptionsJSON(ag, dialogMode, prov, mod, sess.ContextUsedRatio, nil)
	if err != nil {
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return "", err
	}
	if src := event.EnvelopeSourceFromContext(ctx); src != "" {
		userOpts, err = chatagent.MergeInboundSourceIntoUserOptionsJSON(
			userOpts, src,
			event.EnvelopePlatformFromContext(ctx),
			event.EnvelopeChannelKeyFromContext(ctx),
		)
		if err != nil {
			o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
			return "", err
		}
	}

	// V2-T6 语音输入溯源：input_modality/asr_provider/asr_duration_ms +
	// 展示态留档音频附件（仅 UI 回放，不经 LLM 附件链路——刻意绕开
	// validateTurnAttachmentCapabilities 与 BuildUserMessageFromArtifacts，
	// 防止 audio/* 触发能力拒绝及 WAV 字节注入 LLM 上下文）。
	if v := input.Voice; v != nil {
		userOpts, err = chatagent.MergeVoiceMetaIntoUserOptionsJSON(userOpts, v.ASRProvider, v.DurationMs)
		if err != nil {
			o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
			return "", err
		}
		if v.Archive != nil {
			userOpts, err = mergeUserAttachmentRefs(userOpts, []artifactbiz.Ref{*v.Archive})
			if err != nil {
				o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
				return "", err
			}
		}
	}

	userOpts, err = mergeUserAttachmentRefs(userOpts, attachmentRefs)
	if err != nil {
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return "", err
	}
	return userOpts, nil
}

// publishTurnProgress emits an orchestration_progress SystemNoticeEvent
// (WS-only, not persisted) for the pre-orchestration turn phases
// (routing/recalling/preparing_tools/understanding/assessing/starting).
// The frontend maps meta.phase via ORCHESTRATION_PROGRESS_MAP, closing the
// feedback gap between message ack and TaskPlanner's decomposing/allocated
// events (2026-08-06 20:45 session: >2min silent window). Nil bus or empty
// session → skipped.
// Stability:internal
func (o *ChatOrchestrator) publishTurnProgress(ctx context.Context, sessionID, phase string, extra map[string]any) {
	bus := o.td().Pipeline.EventBus
	if bus == nil || sessionID == "" {
		return
	}
	meta := map[string]any{"phase": phase}
	for k, v := range extra {
		meta[k] = v
	}
	bus.Publish(ctx, biz.NewSystemNoticeEvent(sessionID, "orchestration_progress", "orchestration progress: "+phase, meta))
}

// runIntentPass executes the intent recognition pass and returns the intent
// artifact (nil if the pass was skipped or failed). The artifact is injected
// as run context by the caller AFTER the clarification gate — an auto-resolved
// gate strips clarification residue first, and RunOptionInject serializes
// eagerly, so injecting here would bake the residue in.
// Stability:internal
func (o *ChatOrchestrator) runIntentPass(
	ctx context.Context,
	ag biz.Agent,
	sessionID, content, prov, mod string,
	emitter *event.TraceEmitter,
) *intent.Artifact {
	// P0 性能：允许运维通过 ARANEA_INTENT_PASS_MODEL 指定轻量模型加速意图识别；
	// 覆盖对必须存在于 LLM catalog，否则回退 turn 模型保证 Intent Pass 仍可运行。
	prov, mod = resolveIntentPassProviderModel(ctx, o.td().ReadDeps.LLM, prov, mod, o.lg())
	emitter.LogStart("chat.intent.pass", "意图识别开始", event.P("provider", prov), event.P("model", mod), event.P("content_len", len(content)))
	// P3 aux 缓存：注入 sessionID 供缓存键隔离。
	ctx = intent.WithSessionID(ctx, sessionID)
	intRes := intent.RunForAgent(ctx, ag, o.td().ReadDeps.LLM, o.td().LLMHTTP, prov, mod, content, o.recentIntentHistory(ctx, sessionID, content), o.lg())
	// P1-2 (2026-08-19): 记录 intent pass 旁路用量（此前完全漏记）。
	o.turnMetrics().RecordAuxUsage(ctx, biz.AuxLLMUsageInput{
		Kind:          biz.UsageKindAuxIntent,
		SessionID:     sessionID,
		AgentID:       ag.ID,
		AgentKey:      ag.AgentKey,
		Provider:      prov,
		Model:         mod,
		Status:        "success",
		PromptTok:     intRes.PromptTok,
		CompletionTok: intRes.CompletionTok,
		UsageSource:   biz.UsageSourceResponse,
		Latency:       intRes.Duration,
	})
	if intRes.Artifact != nil {
		emitter.LogDone("chat.intent.pass", "意图识别完成", event.P("outcome", intRes.Outcome), event.P("intent_kind", intRes.Artifact.IntentKind), event.P("refined_goal_len", len(intRes.Artifact.RefinedGoal)), event.P("duration_ms", intRes.Duration.Milliseconds()))
	} else {
		emitter.LogSkip("chat.intent.pass", "意图识别跳过", event.P("outcome", intRes.Outcome), event.P("duration_ms", intRes.Duration.Milliseconds()))
	}
	meta := intent.RunMeta{AgentID: ag.ID, SessionID: sessionID}
	intentPayload := intent.BuildIntentPassPayload(intRes, meta)
	if bus := o.td().Pipeline.EventBus; bus != nil {
		bus.Publish(ctx, biz.NewSystemNoticeEvent(sessionID, "intent_pass", "", intentPayload))
	}
	return intRes.Artifact
}

// resolveIntentPassProviderModel applies the optional ARANEA_INTENT_PASS_MODEL /
// ARANEA_INTENT_PASS_PROVIDER override (see intent.ModelOverrideFromEnv) so
// operators can point Intent Pass at a lighter model than the turn model.
// The override pair must exist in the LLM catalog; otherwise (or when no
// catalog is wired) the turn's provider/model is kept so Intent Pass still
// runs on a valid model.
// Stability:internal
func resolveIntentPassProviderModel(ctx context.Context, catalog biz.TeamModelCatalog, prov, mod string, lg loggateway.Logger) (string, string) {
	ovProvider, ovModel := intent.ModelOverrideFromEnv()
	if ovModel == "" {
		return prov, mod
	}
	if ovProvider == "" {
		ovProvider = prov
	}
	if catalog == nil {
		lg.Warn("intent pass model override ignored: no LLM catalog",
			loggateway.StepID("chat.intent.model_override"),
			loggateway.Str("override_provider", ovProvider),
			loggateway.Str("override_model", ovModel))
		return prov, mod
	}
	if _, err := catalog.GetByProviderAndModel(ctx, ovProvider, ovModel); err != nil {
		lg.Warn("intent pass model override not in catalog, falling back to turn model",
			loggateway.StepID("chat.intent.model_override"),
			loggateway.Str("override_provider", ovProvider),
			loggateway.Str("override_model", ovModel),
			loggateway.Err(err))
		return prov, mod
	}
	return ovProvider, ovModel
}

// persistTurnUserMessage persists the user message and returns it.
// Stability:internal
func (o *ChatOrchestrator) persistTurnUserMessage(
	ctx context.Context,
	input biz.TurnInput,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	userOpts string,
	attN int,
) (biz.ChatMessage, bool, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	durableCtx := admit.durableCtx

	now := chatagent.RFC3339Now()
	var userMsg biz.ChatMessage
	userMsgPersisted := false
	if durableCtx.active {
		userMsg = durableCtx.buildUserMessage(sessionID, userOpts, attN, emitter)
	} else {
		// userMsg.ID 规则：根 turn 复用 RootTaskActivityID（消息 ID == Task ID，
		// v2 模型约定 Task 即用户输入根活动）；续跑 turn 的消息是系统合成的
		// （synthesisMsg 等），必须发独立 ID——否则与父 Task（即原始用户消息）
		// 主键冲突，失败路径 UpdateChatMessageStatus 会把原始任务串写为 failed。
		msgID := string(chatagent.RootTaskActivityIDFromCtx(ctx))
		if msgID == "" || strings.TrimSpace(input.ParentTaskID) != "" {
			msgID = uuid.NewString()
		}
		userMsg = biz.ChatMessage{
			ID:               msgID,
			SessionID:        sessionID,
			Role:             "user",
			ContentMarkdown:  content,
			Status:           "pending",
			OptionsJSON:      userOpts,
			CreatedAt:        now,
			AttachmentsCount: attN,
		}
		// AF-correlation: TurnID 必须等于 userMsg.ID，使前端通过 API 加载的
		// user message 的 turn_id 非空，useActivityTimeline 才能将 Activity
		// 记录关联到此 UserTurn。缺失会导致 loadMessages 用服务器消息替换
		// pending-user 占位消息后 turn_id 丢失，思考和回复 UI 不显示。
		//
		// Phase 1c-3: messages 表已删除，用户消息由 ActivityProjector.OnTurnStart
		// 持久化为 Task Activity。userMsg.ID 仍作为 RequestID 传给 ActivityProjector。
		userMsg.TurnID = userMsg.ID
		userMsgPersisted = true
		emitter.LogDone("chat.user_msg_persist", "用户消息已委托给 ActivityProjector")
		if !input.EntryConfig.AllowStream {
			o.bumpSessionRevision(ctx, sessionID)
		}
	}
	return userMsg, userMsgPersisted, nil
}

// invokeTurnLLMAndStream builds run options, invokes the LLM, and consumes the stream.
// Stability:internal
func (o *ChatOrchestrator) invokeTurnLLMAndStream(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	traceBridge *turntrace.Bridge,
	deps chatagent.TRPCBuilderDeps,
	runner trpcrunner.Runner,
	userMsg biz.ChatMessage,
	userMsgPersisted bool,
	userOpts string,
	intentRunOpts []trpcagent.RunOption,
	turnStart time.Time,
) (turnExecuteResult, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	durableCtx := admit.durableCtx

	// Session run lifecycle
	var sessionRunID string
	ctx, sessionRunID = o.durableSessionRunLifecycle(ctx, emitter, sess, ag, durableCtx, userMsg, content)

	// Build run options + prepare run context
	runOpts := o.buildTurnRunOptions(ctx, sess, input, ag, admit, deps, intentRunOpts)
	firstByteTimeout := chatagent.ResolveFirstByteTimeout(ctx, deps.ModelCatalog, ag.Provider, ag.Model)
	if custom, ok := firstByteTimeoutFromContext(ctx); ok {
		firstByteTimeout = custom
	}
	runCtx := o.prepareRunContext(ctx, input, ag, deps)
	// P6-N3：向模型调用链注入首字节静默重试预算——每次 GenerateContent 在
	// budget 内零产出即取消该发并重连（S09 t32 deepseek 首字节 stall
	// 直接 503 整轮的修复）。消费端守卫总时限必须按全部重连尝试放宽，
	// 否则守卫会在重连中途取消 runCtx 误杀恢复。
	// 2026-09-01 活性守卫治理：同点注入流中段 stall 预算（与 team 路径
	// 同口径），模型目录 config_json 可覆盖包默认值。
	runCtx = provider.WithFirstByteRetryBudget(runCtx, firstByteTimeout)
	stallTimeout, stallMaxReconnects := chatagent.ResolveStallPolicy(runCtx, deps.ModelCatalog, ag.Provider, ag.Model)
	runCtx = provider.WithStallBudget(runCtx, stallTimeout, stallMaxReconnects)
	firstByteTimeout = provider.FirstByteGuardWithRetry(firstByteTimeout, stallMaxReconnects)
	runCtx, abortRun := context.WithCancel(runCtx)
	defer abortRun()

	// N-21/N-03: Pre-configure the v2 ActivityProjector and inject it into
	// runCtx so that plugins (cost_guard, model_router) and hooks
	// (tool_confirmation) can emit notice/confirm events via
	// biz.ActivityEmitterFromContext during the LLM call. Configure sets the
	// meta without emitting events. OnTurnStart (called later by the stream
	// consumer) will emit task.created + turn.started.
	//
	var turnProjector *v2.ActivityProjector
	if o.infraDeps.V2ProjectorFactory != nil {
		// B-06: after process restart, restore SeqAssigner from persisted MAX(seq)
		// so new steps do not reuse Seq values already shown to clients.
		if sr := o.stepReader(); sr != nil {
			if maxSeq, err := sr.MaxSeqBySpiritSession(ctx, sessionID); err == nil {
				o.infraDeps.V2ProjectorFactory.RestoreSeqIfNeeded(sessionID, maxSeq)
			}
		}
		// R4-Q3: restore the dedicated per-session TURN counter from
		// turns_v2 MAX(seq) so new turns continue the session's 1,2,3…
		// numbering instead of restarting at 1 (which would collide with
		// persisted turns in the session's seq ordering).
		if tr := o.turnReader(); tr != nil {
			if maxTurnSeq, err := tr.MaxSeqBySession(ctx, sessionID); err == nil {
				o.infraDeps.V2ProjectorFactory.RestoreTurnSeqIfNeeded(sessionID, maxTurnSeq)
			}
		}
		earlyMeta := chatagent.ProjectMeta{
			SessionID:        sessionID,
			RequestID:        userMsg.ID,
			InvocationID:     admit.runID,
			RunID:            admit.runID,
			TraceID:          emitter.TraceID(),
			AgentID:          ag.ID,
			AgentDisplayName: ag.DisplayName,
			ContextWindow:    o.resolveContextWindowTokens(runCtx, sess, ag, admit.provider, admit.model),
			Source:           event.EnvelopeSourceFromContext(runCtx),
			TaskContent:      content,
			ParentTaskID:     input.ParentTaskID,
		}
		v2Meta := chatagent.V2ProjectMetaFromV1(earlyMeta)
		turnProjector = o.infraDeps.V2ProjectorFactory.NewProjector()
		turnProjector.Configure(v2Meta)
		runCtx = biz.WithActivityEmitter(runCtx, turnProjector)
	}

	// LLM invocation
	events, err := o.invokeLLMCall(runCtx, ctx, runner, sess, input, ag, admit, emitter, traceBridge, runOpts, content, turnStart, turnProjector)
	if err != nil {
		// sessionRunID 随错误路径上抛：runSingleAgentViaTRPC 的 EXECUTE 失败
		// 分支需要它终结 session run（phase=failed），否则残留 interactive/
		// durable 相位的 run 会被 durable worker 静默续跑（2026-08-19 事故）。
		return turnExecuteResult{userMsg: userMsg, sessionRunID: sessionRunID}, err
	}

	// Stream consumption
	result, streamErr := o.consumeTurnStream(runCtx, abortRun, sess, ag, admit, emitter, traceBridge, events, firstByteTimeout, turnProjector, time.Now(), content, input.ParentTaskID, input.Synthesis)
	if streamErr != nil {
		res := o.handleStreamError(ctx, ag, admit, emitter, userMsg, streamErr, firstByteTimeout, turnStart, turnProjector)
		res.sessionRunID = sessionRunID
		return res, wrapLLMFailure(streamErr, firstByteTimeout)
	}

	execResult, err := o.assembleTurnResult(ctx, sessionID, admit, result, userMsg, userMsgPersisted, sessionRunID, emitter, ag, turnStart, turnProjector)
	if err != nil {
		return execResult, err
	}
	// Propagate stream-consumer cancellation: the consumer sets c.canceled=true
	// when runCtx.Err()!=nil, marking turns_v2.status=cancelled. The outer ctx
	// (parent of runCtx) may not carry the signal because cancellation
	// propagates parent→child, not child→parent. Flag it so the
	// FinishSessionRunLifecycle defer can align session_runs.phase.
	if !execResult.cancelled && errors.Is(runCtx.Err(), context.Canceled) {
		execResult.cancelled = true
	}
	return execResult, nil
}

// assembleTurnResult checks for context timeout and assembles the final turnExecuteResult.
// When the turn timeout fires without content, we no longer fail immediately — instead we
// push a timeout notification via WS so the user knows the turn is taking longer than
// expected, and continue waiting.
//
// No-Timeout principle (T1.1, 2026-06-18): the previous 24h hard deadline
// (longTaskHardDeadline = turnTimeout * 12) was removed. Tasks now run until
// completion or user cancel. The turnTimeout (10min default) serves only as
// a sync-cap notification threshold — when it fires without content, a
// timeout notification is pushed via WS but the turn is NOT cancelled.
// Stability:internal
func (o *ChatOrchestrator) assembleTurnResult(
	ctx context.Context,
	sessionID string,
	admit turnAdmissionResult,
	result chatagent.EventStreamResult,
	userMsg biz.ChatMessage,
	userMsgPersisted bool,
	sessionRunID string,
	emitter *event.TraceEmitter,
	ag biz.Agent,
	turnStart time.Time,
	turnProjector *v2.ActivityProjector,
) (turnExecuteResult, error) {
	if errors.Is(ctx.Err(), context.DeadlineExceeded) && !result.HasContent {
		emitter.LogCritical("chat.turn.timeout", "对话请求超时，推送超时提醒并继续等待", event.P("timeout", o.turnTimeout().String()), event.P("reason", "sync_cap"))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "timeout").Observe(time.Since(turnStart).Seconds())
		// Push a timeout notification via WS instead of failing the turn.
		// The turn continues to wait for the LLM to respond.
		o.publishTurnTimeoutNotification(ctx, sessionID, admit.runID, o.turnTimeout())
		return turnExecuteResult{userMsg: userMsg, sessionRunID: sessionRunID}, TurnError(TurnErrTurnTimeout, o.turnTimeout().String())
	}

	var turnArtCollector *artifactbiz.TurnCollector
	ctx, turnArtCollector = artifactbiz.WithTurnCollector(ctx)
	return turnExecuteResult{
		userMsg: userMsg, userMsgPersisted: userMsgPersisted, result: result,
		resultPromptTok: result.PromptTok, resultCompletionTok: result.CompletionTok,
		sessionRunID: sessionRunID, turnArtCollector: turnArtCollector,
		turnProjector: turnProjector,
	}, nil
}

// invokeLLMCall builds the user turn message, calls the LLM, and returns the event stream.
// turnProjector is the pre-configured per-turn v2 projector (may be nil); on
// LLM call failure it persists the failure as an error step via
// ProjectTurnFailure, since the stream consumer never runs on this path.
// Stability:internal
func (o *ChatOrchestrator) invokeLLMCall(
	runCtx, ctx context.Context,
	runner trpcrunner.Runner,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	traceBridge *turntrace.Bridge,
	runOpts []trpcagent.RunOption,
	content string,
	turnStart time.Time,
	turnProjector *v2.ActivityProjector,
) (<-chan *trpcevent.Event, error) {
	sessionID := strings.TrimSpace(input.SessionID)
	runID := admit.runID
	prov := admit.provider
	mod := admit.model

	safego.Go(runCtx, "llm-call-timeout-log", func() {
		select {
		case <-time.After(llmInvokeSlowLogThreshold):
			emitter.Log("chat.llm.invoke", event.FlowPhaseStart, "语言模型调用超过 60 秒仍在等待", event.P("run_id", runID))
		case <-runCtx.Done():
		}
	})

	emitter.LogStart("chat.llm.invoke", "正在调用语言模型")
	o.lg().With(loggateway.SessionID(sessionID)).Info("runSingleAgentViaTRPC: 开始构建 userMessage + 调用 LLM",
		loggateway.StepID("chat.llm_invoke_start"),
		loggateway.Any("provider", prov), loggateway.Any("model", mod), loggateway.Any("run_id", runID))

	userTurnMsg, err := o.buildUserMessage(runCtx, sessionID, content, input.Options.AttachmentIDs)
	if err != nil {
		emitter.LogError("chat.llm.invoke", "附件装配失败", event.P("error", err.Error()))
		if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, "failed", err.Error()); serr != nil {
			o.lg().Warn("set run status failed on attachment error",
				loggateway.StepID("chat.turn.attach_fail"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(serr))
		}
		te := TurnError(TurnErrAttachmentFailed, err.Error())
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return nil, te
	}

	llmCtx, llmSpan := traceBridge.StartChild(runCtx, "chat.llm.invoke")
	uid := chatagent.UserIDFromCtx(llmCtx)
	llmCallStart := time.Now()
	events, err := chatagent.RunTRPCUserTurnMsg(llmCtx, runner, uid, sessionID, userTurnMsg, runOpts...)
	turntrace.EndChild(llmSpan, err)
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: RunTRPCUserTurnMsg (LLM call)",
		loggateway.StepID("chat.llm_call"),
		loggateway.Any("elapsed_ms", time.Since(llmCallStart).Milliseconds()),
		loggateway.Any("has_error", err != nil))
	o.lg().With(loggateway.SessionID(sessionID)).Info("runSingleAgentViaTRPC: LLM 调用返回",
		loggateway.StepID("chat.llm_invoke_done"),
		loggateway.Any("elapsed_ms", time.Since(turnStart).Milliseconds()),
		loggateway.Any("has_error", err != nil))
	if err != nil {
		emitter.LogError("chat.llm.invoke", "语言模型调用失败", event.P("error", err.Error()))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "error").Observe(time.Since(turnStart).Seconds())
		if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, "failed", err.Error()); serr != nil {
			o.lg().Warn("set run status failed on llm error",
				loggateway.StepID("chat.turn.llm_fail"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(serr))
		}
		te := TurnError(TurnErrLLMCallFailed, err.Error())
		fail := provider.ClassifyFailure(err.Error(), err)
		o.publishLLMFailureNotice(ctx, sessionID, fail, err.Error())
		if fail.Kind == provider.FailureBilling || fail.Kind == provider.FailureAuth {
			te = TurnError(turnCodeFromFailure(fail), err.Error())
		}
		// 2026-08-19 00:48 no-reply incident fix: the stream consumer never
		// runs on this path, so without this call the failed turn left zero
		// trace in the v2 activity store (user message + error both lost on
		// reload). ProjectTurnFailure materializes task+turn, emits exactly
		// one error step, and finalizes the turn as failed.
		// Cancel 排除：用户取消期间 LLM 调用以 ctx.Canceled 返回时不得落
		// error step（cancelled wins，C-10）。
		if !o.runWasCancelled(ctx, sessionID, err) {
			turnProjector.ProjectTurnFailure(ctx, err.Error(), string(fail.Kind), "")
		}
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return nil, te
	}
	emitter.LogDone("chat.llm.invoke", "模型已返回，开始处理输出流")
	return events, nil
}

// prepareRunContext assembles the run context with knowledge, A2A, and reply hooks.
// Stability:internal
func (o *ChatOrchestrator) prepareRunContext(
	ctx context.Context,
	input biz.TurnInput,
	ag biz.Agent,
	deps chatagent.TRPCBuilderDeps,
) context.Context {
	runCtx := serviceawaitreply.WithReplyFunc(ctx, deps.AwaitHook)
	// Voice Fast-Path（2026-08-09）：语音轮次打标 → BeforeModel 回调 per-request
	// 关 thinking（见 internal/agent/voice_fastpath.go）。
	if input.Voice != nil {
		runCtx = chatagent.WithVoiceFastPath(runCtx)
	}
	runCtx = o.injectA2AContext(runCtx, ag.ID)
	if o.rt().Knowledge.Retriever != nil {
		runCtx = knowledgetool.WithRetriever(runCtx, o.rt().Knowledge.Retriever)
	}
	if o.rt().Knowledge.Router != nil {
		runCtx = knowledgetool.WithAdaptiveRouter(runCtx, o.rt().Knowledge.Router)
	}
	if o.rt().Knowledge.FederatedRetriever != nil {
		runCtx = knowledgetool.WithFederatedRetriever(runCtx, o.rt().Knowledge.FederatedRetriever)
	}
	// P1（2026-08-21）：对话路径默认不注入 RetrievalEvaluator——评估 LLM
	// 约 1.4s，且会诱导模型把 knowledge_reflect 当主检索。管理端 Search
	// 仅在显式评估时走 SearchWithEvaluation。
	if len(input.Options.KnowledgeBases) > 0 {
		runCtx = knowledgetool.WithKnowledgeCollections(runCtx, input.Options.KnowledgeBases)
	}
	return runCtx
}

// buildTurnRunOptions assembles the trpc-agent RunOption slice for the LLM call.
// Stability:evolving
func (o *ChatOrchestrator) buildTurnRunOptions(
	ctx context.Context,
	sess biz.Session,
	input biz.TurnInput,
	ag biz.Agent,
	admit turnAdmissionResult,
	deps chatagent.TRPCBuilderDeps,
	intentRunOpts []trpcagent.RunOption,
) []trpcagent.RunOption {
	sessionID := strings.TrimSpace(input.SessionID)
	content := strings.TrimSpace(input.Content)
	durableCtx := admit.durableCtx

	// typed-nil 防护：ag.Settings 为 nil 时必须传 nil interface，
	// 否则 GetSkillRuntimeJSON() 会 panic（同 trpc_build.go runtimeIface）。
	var skillRuntime skillruntime.RuntimeSettings
	if ag.Settings != nil {
		skillRuntime = ag.Settings
	}
	runOpts := durableResumeRunOpts(durableCtx.active, []trpcagent.RunOption{
		trpcagent.WithRequestID(sessionID),
		// R6 fork id 空间统一（79-runtime-governance，路线A 框架补丁
		// ADR 2026-08-27）：v2 turns_v2.id = admit.runID，框架事件
		// invocationId 同源对齐，session_fork_repo 的
		// event->>'invocationId' 边界匹配才命中。缺省会退化为框架
		// uuid → fork 恒 404 "turn has no runtime events"。
		trpcagent.WithRunInvocationID(admit.runID),
		skillruntime.RunOptionWithTurnQuery(content),
		// 批次 B：按 agent policy 安装概览预算渲染器（显式 0 = 不安装）。
		skillruntime.RunOptionWithOverviewBudget(skillRuntime),
		// 框架 v1.11 修复管线：参数 JSON 修复（response 阶段，自建
		// tool_args_repair_guard 降为兜底）+ 文本工具调用提取（模型把
		// 工具调用写成 <tool_call> 文本时挽回整轮）。
		trpcagent.WithToolCallArgumentsJSONRepairEnabled(true),
		trpcagent.WithToolCallTextRepairEnabled(true),
	})
	if input.EntryConfig.AllowStream {
		runOpts = append(runOpts, trpcagent.WithStream(true))
		// NOTE: StreamModeMessages filter is intentionally NOT enabled here.
		// That filter only forwards ChatCompletion/Chunk events and silently
		// drops tool.response events, which prevents ActivityProjector from
		// calling OnToolResult — every tool then ends up failed via the
		// OnStuckTools timeout fallback path. With AF-3 (ActivityProjector
		// directly consumes trpc events), we need tool.response events to
		// reach the stream consumer so tool results are projected correctly.
	}
	runOpts = append(runOpts, intentRunOpts...)
	// Install per-run tool permission policy: blocks protected tools
	// (exec_command/shell_exec/file/etc.) from accessing sensitive paths
	// (.aws/.ssh/.kube/.env/credentials). Non-protected tools pass through
	// with zero overhead (immediate AllowPermission).
	runOpts = append(runOpts, trpcagent.WithToolPermissionPolicyFunc(o.cmdSafetyChecker.CheckPermission))
	if ag.Settings != nil {
		if vars := chatagent.ParseVariablesJSON(ag.Settings.VariablesJSON, o.lg()); vars != nil {
			runOpts = append(runOpts, trpcagent.MergeRuntimeState(vars))
		}
		// Install TransferController for agent transfer safety (depth limit + timeout).
		// Created per-turn (per-run) so the depth counter is scoped to a single run,
		// preventing cross-run leakage while correctly tracking nested transfers
		// within one run. See TransferControllerImpl docs for details.
		runOpts = append(runOpts, trpcagent.MergeRuntimeState(
			chatagent.NewTransferController(o.lg()).RuntimeState()))
	}
	if chMeta, ok := biz.ParseChannelSessionMeta(sess.MetadataJSON); ok {
		if deliveryState := outbound.RuntimeStateForTarget(outbound.DeliveryTarget{
			Channel: chMeta.ChannelID,
			Target:  chMeta.PeerID,
		}); len(deliveryState) > 0 {
			runOpts = append(runOpts, trpcagent.MergeRuntimeState(deliveryState))
		}
	}

	// Inject per-request Provider/Model override via RunOption.
	// The cached agent is built with its default model (ag.Provider/ag.Model),
	// or a system default model when the agent has no model configured.
	// If the request specifies a different Provider/Model (or the agent has
	// no model), we resolve it and inject agent.WithModel() so resolveBaseModel()
	// picks it up at run time.
	// This is the key enabler for removing Provider/Model from the cache key.
	prov := admit.provider
	mod := admit.model
	agentDefaultProv := strings.TrimSpace(ag.Provider)
	agentDefaultMod := strings.TrimSpace(ag.Model)
	agentModelEmpty := agentDefaultProv == "" || agentDefaultMod == ""
	// When the agent has no model, always inject the request-level model.
	// When the agent has a model, inject only if the request model differs.
	if prov != "" && mod != "" && (agentModelEmpty || prov != agentDefaultProv || mod != agentDefaultMod) {
		if m, err := chatagent.ResolveModelForRunOption(ctx, deps, prov, mod, o.lg()); err == nil && m != nil {
			runOpts = append(runOpts, trpcagent.WithModel(m))
			o.lg().Debug("请求级 Model RunOption 已注入",
				loggateway.StepID("chat.run_option_model"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Str("agent_default_provider", agentDefaultProv),
				loggateway.Str("agent_default_model", agentDefaultMod),
				loggateway.Str("request_provider", prov),
				loggateway.Str("request_model", mod))
		} else if err != nil {
			o.lg().Warn("请求级 Model 解析失败，将使用 Agent 默认模型",
				loggateway.StepID("chat.run_option_model_fail"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Str("provider", prov),
				loggateway.Str("model", mod),
				loggateway.Err(err))
		}
	}

	// Apply runtime profile overrides (prompt/tools/skills/knowledge/workspace/
	// credentials/isolation/extra-model). ProfileResolver returns nil when no
	// active profile is configured — callers treat nil as "use agent defaults".
	if pr := o.profileResolver(); pr != nil {
		if profOpts, prErr := pr.ResolveRunOptions(ctx, ag.ID); prErr == nil && len(profOpts) > 0 {
			runOpts = append(runOpts, profOpts...)
		} else if prErr != nil {
			o.lg().Warn("runtime profile resolve failed, using agent defaults",
				loggateway.StepID("chat.run_option_profile_fail"),
				loggateway.Str("agent_id", ag.ID),
				loggateway.Err(prErr))
		}
	}

	return runOpts
}

// consumeTurnStream wraps the stream consumption with logging.
// v2Projector is the per-turn projector (created in invokeTurnLLMAndStream
// via V2ProjectorFactory). When non-nil, the stream consumer calls
// OnTurnStart/OnTurnEndEnhanced on this instance. Passing the same instance
// ensures plugin notice/confirm events emitted during the LLM call share
// state with the stream consumer's event handling.
// Stability:internal
func (o *ChatOrchestrator) consumeTurnStream(
	runCtx context.Context,
	abortRun context.CancelFunc,
	sess biz.Session,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	traceBridge *turntrace.Bridge,
	events <-chan *trpcevent.Event,
	firstByteTimeout time.Duration,
	v2Projector *v2.ActivityProjector,
	llmStart time.Time,
	userContent string,
	parentTaskID string,
	synthesis bool,
) (chatagent.EventStreamResult, error) {
	sessionID := strings.TrimSpace(sess.ID)
	runID := admit.runID
	prov := admit.provider
	mod := admit.model

	contextWin := o.resolveContextWindowTokens(runCtx, sess, ag, prov, mod)
	projectMeta := chatagent.ProjectMeta{
		SessionID: sessionID, RequestID: event.TurnIDFromContext(runCtx),
		InvocationID: runID, RunID: runID,
		TraceID: emitter.TraceID(), AgentID: ag.ID,
		AgentDisplayName: ag.DisplayName, ContextWindow: contextWin,
		Source:          event.EnvelopeSourceFromContext(runCtx),
		TaskContent:     userContent,
		SpiritSessionID: sessionID,
		ParentTaskID:    parentTaskID,
		Synthesis:       synthesis,
	}
	events = event.WrapFrameworkEventsWithOtel(events, emitter, traceBridge, traceBridge)
	streamOpts := NewChatStreamConsumeOptions(v2Projector)
	if streamOpts == nil {
		streamOpts = &chatagent.StreamConsumeOptions{}
	}
	streamOpts.AbortOnStall = abortRun
	o.lg().With(loggateway.SessionID(sessionID)).Info("runSingleAgentViaTRPC: 开始消费事件流",
		loggateway.StepID("chat.stream_consume_start"),
		loggateway.Any("first_byte_timeout", firstByteTimeout.String()))
	result, streamErr := chatagent.ConsumeWithFirstByteGuard(runCtx, firstByteTimeout, events, projectMeta, streamOpts, o.lg())
	o.lg().With(loggateway.SessionID(sessionID)).Info("runSingleAgentViaTRPC: 事件流消费完成",
		loggateway.StepID("chat.stream_consume_done"),
		loggateway.Any("elapsed_ms", time.Since(llmStart).Milliseconds()),
		loggateway.Any("has_stream_error", streamErr != nil),
		loggateway.Any("stream_error", fmt.Sprintf("%v", streamErr)),
		loggateway.Any("has_content", result.HasContent),
		loggateway.Any("has_error", result.HasError),
		loggateway.Any("reply_len", result.Reply.Len()))
	if streamErr == nil {
		emitter.LogDone("chat.stream.consume", "模型输出流处理完成",
			event.P("reply_len", result.Reply.Len()),
			event.P("has_error", result.HasError),
			event.P("has_content", result.HasContent),
			event.P("prompt_tok", result.PromptTok),
			event.P("completion_tok", result.CompletionTok))
	}
	return result, streamErr
}

// handleStreamError classifies and records stream errors.
// turnProjector (may be nil) persists the failure as an error step when the
// stream consumer did not already project one (e.g. first-byte timeout and
// doom-loop aborts never produce an in-stream Response.Error).
// Stability:internal
func (o *ChatOrchestrator) handleStreamError(
	ctx context.Context,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	userMsg biz.ChatMessage,
	streamErr error,
	firstByteTimeout time.Duration,
	turnStart time.Time,
	turnProjector *v2.ActivityProjector,
) turnExecuteResult {
	sessionID := strings.TrimSpace(userMsg.SessionID)
	runID := admit.runID
	fail := provider.ClassifyFailure(streamErr.Error(), streamErr)
	if errors.Is(streamErr, chatagent.ErrFirstByteTimeout) {
		o.publishLLMFailureNotice(ctx, sessionID, fail, streamErr.Error())
		emitter.LogCritical("chat.first_byte_timeout", "首字节超时，供应商无响应", event.P("timeout", firstByteTimeout.String()))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "first_byte_timeout").Observe(time.Since(turnStart).Seconds())
		if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, "failed", "first byte timeout"); serr != nil {
			o.lg().Warn("set run status failed on first byte timeout",
				loggateway.StepID("chat.turn.first_byte_timeout"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(serr))
		}
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonTimeout)
	} else {
		o.publishLLMFailureNotice(ctx, sessionID, fail, streamErr.Error())
		if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, "failed", streamErr.Error()); serr != nil {
			o.lg().Warn("set run status failed on stream error",
				loggateway.StepID("chat.turn.stream_fail"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(serr))
		}
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
	}
	// 2026-08-19 no-reply incident fix: persist a durable error step. When the
	// consumer already routed an in-stream Response.Error through OnError,
	// ProjectTurnFailure skips the duplicate; the consumer's finalize already
	// closed the turn, so only the missing pieces are emitted.
	turnProjector.ProjectTurnFailure(ctx, streamErr.Error(), string(fail.Kind), "")
	o.publishTurnFailure(sessionID, runID, "chat-service", streamErr, "")
	return turnExecuteResult{userMsg: userMsg}
}

// ────────────────────────────────────────────────────────────
// PERSIST phase helpers (called by turnPhases.persistTurn)
// ────────────────────────────────────────────────────────────

// handleEmptyReply records an empty reply error and returns it.
// turnProjector (may be nil) persists the failure as an error step: the
// stream consumer finalized the turn normally (no in-stream error), so
// without this call an empty reply left no durable trace in the activity
// store (2026-08-19 no-reply incident fix).
// Stability:internal
func (o *ChatOrchestrator) handleEmptyReply(
	ctx context.Context,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	result chatagent.EventStreamResult,
	turnStart time.Time,
	turnStatus *string,
	turnErr *error,
	turnErrMsg *string,
	sessionID string,
	turnProjector *v2.ActivityProjector,
) error {
	runID := admit.runID
	fail := provider.ClassifyFailure(result.LastError, nil)
	emitter.LogCritical("chat.turn.empty_reply", "未收到助手回复", event.P("has_error", result.HasError), event.P("last_error", result.LastError), event.P("has_content", result.HasContent), event.P("failure_kind", string(fail.Kind)))
	detail := ""
	if result.HasError {
		detail = result.LastError
	} else if !result.HasContent {
		detail = "no content produced"
	}
	if detail == "" {
		detail = "empty reply"
	}
	o.publishLLMFailureNotice(ctx, sessionID, fail, detail)
	markTurnError(turnStatus, turnErr, turnErrMsg, errors.New(detail))
	arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "empty_reply").Observe(time.Since(turnStart).Seconds())
	if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, "failed", detail); serr != nil {
		o.lg().Warn("set run status failed on empty reply",
			loggateway.StepID("chat.turn.empty_reply"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(serr))
	}
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
	code := TurnErrEmptyReply
	if fail.Kind == provider.FailureBilling || fail.Kind == provider.FailureAuth || fail.Kind == provider.FailureStall {
		code = turnCodeFromFailure(fail)
	}
	te := TurnError(code, detail)
	// Cancel 排除：用户取消早于首字节时流消费者以 nil error 返回空结果，
	// 会走到本路径；此时 turn/task 已被消费者以 cancelled 终结，不得再落
	// error step（cancelled wins，C-10）。
	if !o.runWasCancelled(ctx, sessionID, nil) {
		turnProjector.ProjectTurnFailure(ctx, detail, string(fail.Kind), "")
	}
	o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
	return te
}

// buildAndPersistAssistantMessage constructs and persists the assistant message.
// Stability:internal
func (o *ChatOrchestrator) buildAndPersistAssistantMessage(
	ctx context.Context,
	ag biz.Agent,
	admit turnAdmissionResult,
	execResult turnExecuteResult,
	emitter *event.TraceEmitter,
	displayMarkdown string,
	reasoningAsDisplay bool,
	promptTok, completionTok int,
	turnStatus *string,
	turnErr *error,
	turnErrMsg *string,
	turnStart time.Time,
) (biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(execResult.userMsg.SessionID)
	runID := admit.runID
	mod := admit.model
	result := execResult.result

	assistantOptsStr, err := chatagent.AssistantOptionsJSON(ag, nil)
	if err != nil {
		o.markAndPublish(sessionID, runID, turnStatus, turnErr, turnErrMsg, err)
		return biz.ChatMessage{}, err
	}
	if s := result.Reasoning.String(); s != "" {
		if assistantOptsStr, err = chatagent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, s); err != nil {
			o.markAndPublish(sessionID, runID, turnStatus, turnErr, turnErrMsg, err)
			return biz.ChatMessage{}, err
		}
	}
	// Mark reasoning_as_display when content_markdown is a reasoning fallback.
	// Frontend uses this flag to render ThinkActivity instead of SayActivity,
	// ensuring thinking and replying are visually separated per the Activity Timeline proposal.
	if reasoningAsDisplay {
		if assistantOptsStr, err = chatagent.MergeReasoningAsDisplayFlag(assistantOptsStr, true); err != nil {
			o.markAndPublish(sessionID, runID, turnStatus, turnErr, turnErrMsg, err)
			return biz.ChatMessage{}, err
		}
	}
	if execResult.turnArtCollector != nil {
		if merged, merr := mergeTurnArtifactRefs(assistantOptsStr, execResult.turnArtCollector.Refs()); merr != nil {
			o.markAndPublish(sessionID, runID, turnStatus, turnErr, turnErrMsg, merr)
			return biz.ChatMessage{}, merr
		} else {
			assistantOptsStr = merged
		}
	}

	assistantAttN := 0
	if execResult.turnArtCollector != nil {
		assistantAttN = len(execResult.turnArtCollector.Refs())
	}
	assistantMsg := biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		Role:             "assistant",
		ContentMarkdown:  displayMarkdown,
		ModelName:        mod,
		Status:           "ok",
		OptionsJSON:      assistantOptsStr,
		CreatedAt:        chatagent.RFC3339Now(),
		TokenIn:          promptTok,
		TokenOut:         completionTok,
		AttachmentsCount: assistantAttN,
		LatencyMS:        int(time.Since(turnStart).Milliseconds()),
	}
	// Phase 1c-3: messages 表已删除。assistant 消息由 ActivityProjector.OnAssistantMessage
	// 持久化为 Reply Activity，用户消息状态由 OnTurnEnd 终结为 completed。
	// assistantMsg.ID 仍保留用于 recordSessionTurn 的 user/assistant 关联记录。
	return assistantMsg, nil
}

// ────────────────────────────────────────────────────────────
// POST-PROCESS phase
// ────────────────────────────────────────────────────────────

// postProcessTurn records metrics, completes status, bumps revision, and fires hooks.
// clarifySuspended 为 true 表示 P2 后置澄清已把 session 翻转为
// awaiting_confirmation（running→awaiting_confirmation 合法），此处跳过落
// completed（FSM 不允许 awaiting_confirmation→completed）；run 状态仍照常
// 落 completed——LLM turn 确已完成，等待用户是 Session 层语义。
// Stability:internal
func (o *ChatOrchestrator) postProcessTurn(
	ctx context.Context,
	sess biz.Session,
	ag biz.Agent,
	input biz.TurnInput,
	admit turnAdmissionResult,
	execResult turnExecuteResult,
	persistResult turnPersistResult,
	emitter *event.TraceEmitter,
	turnStart time.Time,
	turnStatus string,
	clarifySuspended bool,
) {
	sessionID := strings.TrimSpace(input.SessionID)
	runID := admit.runID
	prov := admit.provider
	mod := admit.model
	content := strings.TrimSpace(input.Content)

	metricsLabel := "ok"
	if turnStatus == "timeout_degraded" {
		metricsLabel = "timeout_degraded"
	}
	arametrics.ChatTurnDuration.WithLabelValues(ag.ID, metricsLabel).Observe(time.Since(turnStart).Seconds())
	o.recordSessionTurn(ctx, sessionID, ag, execResult.userMsg.ID, persistResult.assistantMsg.ID, prov, mod, persistResult.promptTok, persistResult.completionTok, execResult.result.CachedTok, persistResult.assistantMsg.ContentMarkdown,
		int(time.Since(turnStart).Milliseconds()), execResult.result.FirstTokenMs, execResult.result.ModelCallCount, execResult.result.ToolCallCount)
	// F4 取消竞态：cancelActiveRun 已将 run 置 cancelled（终态）后，EXECUTE/PERSIST
	// 成功路径才走到这里。cancelled wins——跳过 completed 状态发布 / Session 翻转 /
	// revision bump / after_turn 钩子与"执行完成"流程日志，避免 cancelled 之后再冒出
	// completed 的矛盾噪音。用量记账（上方 recordSessionTurn）保留：助手消息已落库，
	// token 真实消耗。
	if o.runWasCancelled(ctx, sessionID, nil) {
		emitter.LogSkip("chat.turn.execute", "对话轮次已取消，跳过完成状态发布", event.P("run_id", runID))
		return
	}
	if clarifySuspended {
		// G2：awaiting_input 悬挂不得把 run 记 completed（与澄清门同口径 awaiting_user）。
		if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, string(biz.RunStateAwaitingUser), ""); serr != nil {
			o.lg().Warn("set run status failed on clarify suspend",
				loggateway.StepID("chat.turn.clarify_suspend"),
				loggateway.Str("session_id", sessionID),
				loggateway.Str("run_id", runID),
				loggateway.Err(serr))
		}
		o.persistRememberIfRequested(ctx, sess, ag, input, execResult)
		o.bumpSessionRevision(ctx, sessionID)
		emitter.LogDone("chat.turn.awaiting_input", "对话轮次挂起等待用户澄清",
			event.P("run_id", runID),
			event.P("reply_len", len(persistResult.assistantMsg.ContentMarkdown)),
		)
		return
	}
	if serr := o.runStatus().SetRunStatus(ctx, sessionID, runID, "completed", ""); serr != nil {
		o.lg().Warn("set run status failed on complete",
			loggateway.StepID("chat.turn.complete"),
			loggateway.Str("session_id", sessionID),
			loggateway.Str("run_id", runID),
			loggateway.Err(serr))
	}
	if !clarifySuspended && !o.spiritWorkInFlight(ctx, sessionID) {
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, "")
	}
	o.persistRememberIfRequested(ctx, sess, ag, input, execResult)
	o.bumpSessionRevision(ctx, sessionID)
	// Synthetic evaluation-case turns must not fire post-turn hooks: the
	// after_turn auto-eval hook would spawn a new run per case and cascade.
	if input.EntryConfig.EntryPoint != biz.EntryPointEvaluation {
		o.notifyNativeTurnHooks(ctx, sessionID, ag, content, persistResult.assistantMsg.ContentMarkdown)
	}
	emitter.LogDone("chat.turn.execute", "对话轮次执行完成",
		event.P("run_id", runID),
		event.P("reply_len", len(persistResult.assistantMsg.ContentMarkdown)),
		event.P("prompt_tok", persistResult.promptTok),
		event.P("completion_tok", persistResult.completionTok),
	)
}

// ────────────────────────────────────────────────────────────
// BUILD phase helper
// ────────────────────────────────────────────────────────────

// turnBuildResult holds the outputs of the BUILD phase.
// Stability:internal
type turnBuildResult struct {
	deps             chatagent.TRPCBuilderDeps
	runner           trpcrunner.ManagedRunner
	rollbackBoundary rt.RunnerRollbackBoundary
	prefetch         *chatagent.TurnCuePrefetch
}

// buildTurnRunner constructs the agent deps, builds the agent, creates the runner,
// and wires the sub-agent service.
// Stability:evolving
func (o *ChatOrchestrator) buildTurnRunner(
	ctx context.Context,
	sess biz.Session,
	ag biz.Agent,
	admit turnAdmissionResult,
	emitter *event.TraceEmitter,
	content string,
) (turnBuildResult, error) {
	sessionID := strings.TrimSpace(sess.ID)
	runID := admit.runID
	dialogMode := admit.dialogMode
	prov := admit.provider
	mod := admit.model

	depsStart := time.Now()
	deps, err := o.agentBuild.BuildTRPCDeps(ctx, AgentBuildParams{
		Session: sess, Agent: ag, RunID: runID,
		DialogMode: dialogMode, Provider: prov, Model: mod, Emitter: emitter,
	})
	if err != nil {
		emitter.LogError("chat.agent.build", "构建Agent依赖失败", event.P("agent_id", ag.ID), event.P("error", err.Error()))
		return turnBuildResult{}, TurnError(TurnErrAgentBuildFailed, err.Error())
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: BuildTRPCDeps",
		loggateway.StepID("chat.build_deps"),
		loggateway.Any("elapsed_ms", time.Since(depsStart).Milliseconds()))

	prefetchCh := make(chan *chatagent.TurnCuePrefetch, 1)
	safego.Go(ctx, "chat.cue.prefetch", func() {
		pctx, cancel := context.WithTimeout(ctx, 4*time.Second)
		defer cancel()
		prefetchCh <- chatagent.PrefetchTurnCues(pctx, deps, ag, content, sess.ID, chatagent.UserIDFromCtx(ctx))
	})

	agentStart := time.Now()
	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps, o.lg())
	if err != nil {
		emitter.LogError("chat.agent.build", "构建Agent实例失败", event.P("agent_id", ag.ID), event.P("error", err.Error()))
		return turnBuildResult{}, TurnError(TurnErrAgentBuildFailed, err.Error())
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: BuildTRPCAgentCached",
		loggateway.StepID("chat.build_agent"),
		loggateway.Any("elapsed_ms", time.Since(agentStart).Milliseconds()))
	emitter.LogDone("chat.agent.build", "Agent实例已构建", event.P("provider", prov), event.P("model", mod))

	var plugins []trpcplugin.Plugin
	wsID := workspace.IDFromContext(ctx)
	if o.rt().Plugin.Manager != nil {
		plugins = o.rt().Plugin.Manager.RunnerPluginsForAgent(ag.ID, wsID)
	} else if o.rt().Plugin.RT != nil {
		plugins = o.rt().Plugin.RT.PluginsForAgent(ag.ID, wsID)
	}
	emitter.LogDone("chat.plugins_load", "插件已加载", event.P("plugin_count", len(plugins)))
	deps.Plugins = plugins
	lookup := map[string]trpcagent.Agent{}
	if key := strings.TrimSpace(ag.AgentKey); key != "" {
		lookup[key] = root
	}
	rl := chatagent.ResolveRalphLoopTurn(ag.Settings)
	if rl.SkipErr != nil {
		emitter.LogWarn("chat.runner.ralph_loop", "Ralph Loop 配置无效，已跳过", "",
			event.P("agent_id", ag.ID), event.P("error", rl.SkipErr.Error()))
	}
	emitter.LogStart("chat.runner.create", "创建 Runner", event.P("agent_key", ag.AgentKey), event.P("plugin_count", len(plugins)))
	runnerMgr := o.tdPtr().CoalesceRunnerManager()
	runnerStart := time.Now()
	runner, err := runnerMgr.NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins: plugins, AwaitUserReplyRouting: deps.AwaitHook != nil,
		BuilderDeps: deps, AgentFactoryKeys: []string{ag.AgentKey},
		LookupAgents: lookup, RalphLoop: rl.Config,
		// P0-03 fix: use real agent ID as AppName so framework memory tools
		// and product proactive recall share the same scope.
		AppName: ag.ID,
	})
	if err != nil {
		emitter.LogError("chat.runner.create", "Runner 创建失败", event.P("error", err.Error()))
		return turnBuildResult{}, err
	}
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: NewTurnRunner",
		loggateway.StepID("chat.runner_create"),
		loggateway.Any("elapsed_ms", time.Since(runnerStart).Milliseconds()))
	emitter.LogDone("chat.runner.create", "Runner 已创建")
	o.runs.StoreRunner(sessionID, runID, runner)
	if o.subAgentService() != nil {
		o.subAgentService().SetRunner(runner)
		// P1-2 (2026-08-19): 同步计费归因，供异步 subagent 运行的
		// aux_subagent 用量记录按父 turn 的 provider/model/agent 计价。
		o.subAgentService().SetAttribution(prov, mod, ag.ID, ag.AgentKey)
		if ag.Settings != nil {
			o.subAgentService().SetSessionRunes(sessionID, ag.Settings.SubagentsStoredResultRunes, ag.Settings.SubagentsStoredSummaryRunes)
		}
	}
	rbStart := time.Now()
	rollbackBoundary, rbErr := runnerMgr.MarkRollbackBoundary(ctx, sessionID, runID, "")
	o.lg().With(loggateway.SessionID(sessionID)).Info("turn timing: MarkRollbackBoundary",
		loggateway.StepID("chat.rollback_boundary"),
		loggateway.Any("elapsed_ms", time.Since(rbStart).Milliseconds()),
		loggateway.Any("has_error", rbErr != nil))
	if rbErr != nil {
		emitter.LogWarn("chat.runner.rollback_boundary", "Runner 回滚边界记录失败", "", event.P("error", rbErr.Error()))
	}
	var prefetch *chatagent.TurnCuePrefetch
	select {
	case prefetch = <-prefetchCh:
	case <-ctx.Done():
	}
	return turnBuildResult{
		deps:             deps,
		runner:           runner,
		rollbackBoundary: rollbackBoundary,
		prefetch:         prefetch,
	}, nil
}

// spiritWorkInFlight reports whether this Spirit session still has background
// orchestration. The root session must stay running until synthesis, instead
// of flipping to completed after the first plan_and_execute reply.
//
// Two signals are required because executeOrchestratePhase returns as soon as
// the PlanBoard is published — teams may not be persisted yet:
//  1. persisted teams in orchestrating / interrupted
//  2. newest non-direct TaskPlan still in a recoverable status (draft/executing)
func (o *ChatOrchestrator) spiritWorkInFlight(ctx context.Context, sessionID string) bool {
	if o == nil || strings.TrimSpace(sessionID) == "" {
		return false
	}
	switch o.resolveSpiritTurnOrchestration(ctx, sessionID).Phase {
	case biz.SpiritPhaseOrchestrating, biz.SpiritPhaseInterrupted:
		return true
	}
	planner := o.team().TaskPlanner
	if planner == nil {
		return false
	}
	plans, err := planner.ListPlans(ctx, sessionID)
	if err != nil || len(plans) == 0 || plans[0] == nil {
		return false
	}
	p := plans[0]
	if p.Strategy == biz.StrategyDirect {
		return false
	}
	return biz.IsRecoverableTaskPlanStatus(p.Status)
}

// persistRememberIfRequested writes an explicit preference when the user said
// 记住/以后都/我的习惯是 but the LLM did not call memory_remember.
func (o *ChatOrchestrator) persistRememberIfRequested(ctx context.Context, sess biz.Session, ag biz.Agent, input biz.TurnInput, execResult turnExecuteResult) {
	if o == nil || !intent.LooksLikeRememberRequest(input.Content) {
		return
	}
	w := o.factWriter()
	if w == nil {
		return
	}
	sourceID := ""
	if execResult.userMsg.ID != "" {
		sourceID = execResult.userMsg.ID
	}
	w.WriteFacts(ctx, sess.ID, ag.ID, sess.UserID, sourceID, []biz.FactMark{{
		Type:       "preference",
		Confidence: "high",
		Content:    strings.TrimSpace(input.Content),
	}})
}
