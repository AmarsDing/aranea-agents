package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"

	"github.com/google/uuid"
)

const (
	defaultTurnTimeout = chatagent.DefaultTurnTimeout
)

func (s *ChatService) runSingleAgentViaTRPC(
	ctx context.Context,
	sess biz.Session,
	req *chatv1.SendChatMessageRequest,
	ag biz.Agent,
	dialogMode, prov, mod string,
	attN int,
) (biz.ChatMessage, biz.ChatMessage, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	content := strings.TrimSpace(req.GetContent())
	if ak := strings.TrimSpace(req.GetAgentKey()); ak != "" && !strings.EqualFold(ak, ag.AgentKey) {
		return biz.ChatMessage{}, biz.ChatMessage{}, TurnError(TurnErrAgentForbidden, "")
	}

	runID := uuid.NewString()
	turnStart := time.Now()
	biz.DefaultTurnCompletionBridge().RegisterTurnStart(sessionID, runID, turnStart)
	turnStatus := "ok"
	var turnErr error
	var resultPromptTok, resultCompletionTok int
	var turnErrMsg string
	ctx, traceBridge, _ := startTurnSpan(ctx, "chat.turn", sessionID, ag.AgentKey, runID)
	emitter := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx: ctx, Bus: s.td.Pipeline.Bus, Buffer: s.td.Pipeline.Buffer,
		SessionID: sessionID, RunID: runID, AgentKey: ag.AgentKey, AgentID: ag.ID,
		Domain: event.TraceDomainChat,
	})
	emitter.SetOtelRefs(traceBridge.TraceID(), traceBridge.RootSpanID())
	ctx = event.WithTraceEmitter(ctx, emitter)
	defer func() {
		emitter.FinishRoot(turnStatus)
		endTurnSpan(traceBridge, turnErr)
		s.recordTurnUsage(ctx, emitter, sessionID, runID, ag.AgentKey, ag.ID, prov, mod, turnStatus,
			resultPromptTok, resultCompletionTok, time.Since(turnStart), turnErrMsg)
	}()

	// Apply a turn-level timeout so a hanging LLM does not block the HTTP
	// caller indefinitely. If the parent context already has a shorter
	// deadline we keep it as-is.
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTurnTimeout)
		defer cancel()
	}

	deps := chatagent.TRPCBuilderDeps{
		Catalog:     s.td.Catalog.LLM,
		AgentUC:     s.td.Catalog.AgentsUC,
		Agents:      s.td.Catalog.Agents,
		RT:          s.td.RoundTrip(),
		SkillUC:     s.td.Catalog.SkillUC,
		MCPTooling:  s.td.Persist.AgentMCP,
		ToolUC:      s.td.Catalog.ToolUC,
		Sessions:    s.td.Sessions,
		Sys:         s.td.Catalog.Settings,
		Provider:    prov,
		Model:       mod,
		DialogMode:  dialogMode,
		SkillDBRepo: s.skillDBRepo,
		// EP-RT-02: inject await-reply hook so the tool blocks mid-turn.
		AwaitHook: s.makeAwaitReplyFunc(ctx, sessionID, runID),
		// EP-RT-05: signal whether a MemoryService will back the runner.
		HasMemory:          s.td.Persist.Memory.Available(),
		PluginManager:      s.pluginManager,
		MemoryAdmin:        s.td.Persist.Memory.Admin,
		KnowledgeRetriever: s.knowledgeRetriever,
		CodeExecFactory:    s.codeExecFactory,
	}
	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		emitter.LogError("chat.agent.build", "构建Agent实例失败", event.P("agent_id", ag.ID), event.P("error", err.Error()))
		s.runs.Finish(sessionID)
		return biz.ChatMessage{}, biz.ChatMessage{}, TurnError(TurnErrAgentBuildFailed, err.Error())
	}
	emitter.LogDone("chat.agent.build", "Agent实例已构建", event.P("provider", prov), event.P("model", mod))

	var plugins []trpcplugin.Plugin
	if s.pluginManager != nil {
		plugins = s.pluginManager.RunnerPluginsForAgent(ag.ID)
	} else if s.pluginRT != nil {
		plugins = s.pluginRT.PluginsForAgent(ag.ID)
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
	runner, err := s.td.CoalesceRunnerManager().NewTurnRunner(root, rt.TurnRunnerSpec{
		Plugins:               plugins,
		AwaitUserReplyRouting: deps.AwaitHook != nil,
		BuilderDeps:           deps,
		AgentFactoryKeys:      []string{ag.AgentKey},
		LookupAgents:          lookup,
		RalphLoop:             rl.Config,
	})
	if err != nil {
		emitter.LogError("chat.runner.create", "Runner 创建失败", event.P("error", err.Error()))
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		s.runs.Finish(sessionID)
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	emitter.LogDone("chat.runner.create", "Runner 已创建")
	s.runs.StoreRunner(sessionID, runID, runner)
	s.setRunStatus(sessionID, runID, "running", "")
	emitter.LogStart("chat.turn.execute", "开始执行对话轮次", event.P("run_id", runID))
	defer func() {
		s.runs.Finish(sessionID)
		runner.Close()
		s.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod)
	}()
	userOpts, err := chatagent.UserOptionsJSON(ag, dialogMode, prov, mod, sess.ContextUsedRatio, nil)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if src := event.EnvelopeSourceFromContext(ctx); src != "" {
		userOpts, err = chatagent.MergeSourceIntoUserOptionsJSON(userOpts, src)
		if err != nil {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
	}
	sendText := content
	if !biz.IsA2AProxyAgent(ag) {
		emitter.LogStart("chat.intent.pass", "意图识别开始", event.P("provider", prov), event.P("model", mod), event.P("content_len", len(content)))
		intRes := intent.Run(ctx, intent.IntentPassFromAgent(ag), s.td.Catalog.LLM, s.td.LLMHTTP, prov, mod, content)
		if intRes.Artifact != nil {
			emitter.LogDone("chat.intent.pass", "意图识别完成", event.P("outcome", intRes.Outcome), event.P("intent_kind", intRes.Artifact.IntentKind), event.P("refined_goal_len", len(intRes.Artifact.RefinedGoal)), event.P("duration_ms", intRes.Duration.Milliseconds()))
			if strings.TrimSpace(intRes.RawJSON) != "" {
				merged, merr := intent.MergeIntoUserOptionsJSON(userOpts, intRes.RawJSON)
				if merr != nil {
					emitter.LogWarn("chat.intent.merge_fail", "意图合并失败", "将继续执行但不包含 intent_artifact", event.P("error", merr.Error()))
				} else {
					userOpts = merged
				}
			}
			sendText = intent.WrapUserMessage(content, intRes.Artifact)
		} else {
			emitter.LogSkip("chat.intent.pass", "意图识别跳过", event.P("outcome", intRes.Outcome), event.P("duration_ms", intRes.Duration.Milliseconds()))
		}
		meta := intent.RunMeta{AgentID: ag.ID, SessionID: sessionID}
		intentPayload := intent.BuildIntentPassPayload(intRes, meta)
		if s.td.Pipeline.Bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeIntentPass, ag.ID, sessionID)
			env.Metadata = intentPayload
			s.td.Pipeline.Bus.Publish(ctx, env)
		}
	} else {
		emitter.LogSkip("chat.intent.pass", "A2A Proxy Agent 跳过意图识别", event.P("agent_kind", ag.Kind))
	}

	now := chatagent.RFC3339Now()
	userMsg := biz.ChatMessage{
		ID:               uuid.NewString(),
		SessionID:        sessionID,
		Role:             "user",
		ContentMarkdown:  content,
		Status:           "ok",
		OptionsJSON:      userOpts,
		CreatedAt:        now,
		AttachmentsCount: attN,
	}
	if err := s.td.Sessions.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	emitter.LogDone("chat.user_msg_persist", "用户消息已持久化")

	uid := chatagent.UserIDFromCtx(ctx)
	runOpts := []trpcagent.RunOption{trpcagent.WithRequestID(sessionID), skillruntime.RunOptionWithTurnQuery(sendText)}
	if ag.Settings != nil {
		if vars := chatagent.ParseVariablesJSON(ag.Settings.VariablesJSON); vars != nil {
			runOpts = append(runOpts, trpcagent.MergeRuntimeState(vars))
		}
	}
	// Apply a turn-level timeout so a hanging LLM call does not block the
	// HTTP caller indefinitely. If the parent context already has a shorter
	// deadline we keep it as-is.
	firstByteTimeout := chatagent.DefaultFirstByteTimeout
	if custom, ok := firstByteTimeoutFromContext(ctx); ok {
		firstByteTimeout = custom
	}
	runCtx := serviceawaitreply.WithReplyFunc(ctx, deps.AwaitHook)
	runCtx = s.injectA2AContext(runCtx, ag.ID)
	if s.knowledgeRetriever != nil {
		runCtx = knowledgetool.WithRetriever(runCtx, s.knowledgeRetriever)
	}

	safego.Go(runCtx, "llm-call-timeout-log", func() {
		select {
		case <-time.After(60 * time.Second):
			emitter.Log("chat.llm.invoke", event.FlowPhaseStart, "语言模型调用超过 60 秒仍在等待", event.P("run_id", runID))
		case <-runCtx.Done():
		}
	})
	emitter.LogStart("chat.llm.invoke", "正在调用语言模型")
	var attachmentRefs []*chatv1.AttachmentRef
	if req.GetOptions() != nil {
		attachmentRefs = req.GetOptions().GetAttachments()
	}
	userTurnMsg, err := s.buildUserMessage(runCtx, sessionID, sendText, attachmentRefs)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		emitter.LogError("chat.llm.invoke", "附件装配失败", event.P("error", err.Error()))
		s.setRunStatus(sessionID, runID, "failed", err.Error())
		return userMsg, biz.ChatMessage{}, TurnError(TurnErrAttachmentFailed, err.Error())
	}
	llmCtx, llmSpan := traceBridge.StartChild(runCtx, "chat.llm.invoke")
	events, err := chatagent.RunTRPCUserTurnMsg(llmCtx, runner, uid, sessionID, userTurnMsg, runOpts...)
	turntrace.EndChild(llmSpan, err)
	if err != nil {
		turnStatus = "error"
		turnErr = err
		turnErrMsg = err.Error()
		emitter.LogError("chat.llm.invoke", "语言模型调用失败", event.P("error", err.Error()))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "error").Observe(time.Since(turnStart).Seconds())
		s.setRunStatus(sessionID, runID, "failed", err.Error())
		return userMsg, biz.ChatMessage{}, TurnError(TurnErrLLMCallFailed, err.Error())
	}
	emitter.LogDone("chat.llm.invoke", "模型已返回，开始处理输出流")

	projectMeta := chatagent.ProjectMeta{
		SessionID:        sessionID,
		RequestID:        sessionID,
		InvocationID:     runID,
		RunID:            runID,
		TraceID:          emitter.TraceID(),
		AgentID:          ag.ID,
		AgentDisplayName: ag.DisplayName,
		Source:           event.EnvelopeSourceFromContext(ctx),
	}
	events = event.WrapFrameworkEventsWithOtel(events, emitter, traceBridge, traceBridge)
	streamOpts := NewChatStreamConsumeOptions(s.td.Catalog.ToolUC, s.td.Catalog.Agents, s.td.Sessions)
	result, streamErr := chatagent.ConsumeWithFirstByteGuard(runCtx, firstByteTimeout, events, s.td.Pipeline.Bus, projectMeta, streamOpts)
	resultPromptTok = result.PromptTok
	resultCompletionTok = result.CompletionTok
	if streamErr != nil {
		if errors.Is(streamErr, chatagent.ErrFirstByteTimeout) {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, streamErr)
			emitter.LogCritical("chat.first_byte_timeout", "首字节超时，模型响应过慢", event.P("timeout", firstByteTimeout.String()))
			arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "first_byte_timeout").Observe(time.Since(turnStart).Seconds())
			s.setRunStatus(sessionID, runID, "failed", "first byte timeout")
			return userMsg, biz.ChatMessage{}, TurnError(TurnErrFirstByteTimeout, firstByteTimeout.String())
		}
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, streamErr)
		s.setRunStatus(sessionID, runID, "failed", streamErr.Error())
		return userMsg, biz.ChatMessage{}, streamErr
	}
	emitter.LogDone("chat.stream.consume", "模型输出流处理完成",
		event.P("reply_len", result.Reply.Len()),
		event.P("has_error", result.HasError),
		event.P("has_content", result.HasContent),
		event.P("prompt_tok", result.PromptTok),
		event.P("completion_tok", result.CompletionTok),
	)
	if ctx.Err() != nil {
		turnStatus = "timeout"
		turnErr = ctx.Err()
		turnErrMsg = "turn timeout"
		emitter.LogCritical("chat.turn.timeout", "对话请求超时", event.P("timeout", defaultTurnTimeout.String()), event.P("reason", "sync_cap"))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "timeout").Observe(time.Since(turnStart).Seconds())
		s.setRunStatus(sessionID, runID, "failed", "turn timeout")
		return userMsg, biz.ChatMessage{}, TurnError(TurnErrTurnTimeout, defaultTurnTimeout.String())
	}

	displayMarkdown := chatagent.DisplayMarkdownFromStream(result)
	if displayMarkdown == "" {
		emitter.LogCritical("chat.turn.empty_reply", "未收到助手回复", event.P("has_error", result.HasError), event.P("last_error", result.LastError), event.P("has_content", result.HasContent))
		detail := ""
		if result.HasError {
			detail = result.LastError
		} else if !result.HasContent {
			detail = "no content produced"
		}
		if detail == "" {
			detail = "empty reply"
		}
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, errors.New(detail))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "empty_reply").Observe(time.Since(turnStart).Seconds())
		s.setRunStatus(sessionID, runID, "failed", detail)
		return userMsg, biz.ChatMessage{}, TurnError(TurnErrEmptyReply, detail)
	}

	promptTok, completionTok := chatagent.EstimateTokensIfMissing(resultPromptTok, resultCompletionTok, content, displayMarkdown)

	assistantOptsStr, err := chatagent.AssistantOptionsJSON(ag, nil)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return userMsg, biz.ChatMessage{}, err
	}
	if s := result.Reasoning.String(); s != "" {
		if assistantOptsStr, err = chatagent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, s); err != nil {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
			return userMsg, biz.ChatMessage{}, err
		}
	}

	assistantMsg := biz.ChatMessage{
		ID:              uuid.NewString(),
		SessionID:       sessionID,
		Role:            "assistant",
		ContentMarkdown: displayMarkdown,
		ModelName:       mod,
		Status:          "ok",
		OptionsJSON:     assistantOptsStr,
		CreatedAt:       chatagent.RFC3339Now(),
		TokenIn:         promptTok,
		TokenOut:        completionTok,
	}
	if err := s.td.Sessions.AppendChatMessage(ctx, sessionID, assistantMsg, true); err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		return userMsg, biz.ChatMessage{}, err
	}
	emitter.LogDone("chat.assistant_msg_persist", "助手消息已持久化", event.P("reply_len", len(displayMarkdown)))
	patchSessionContextUsage(ctx, s, sessionID, ag, promptTok, completionTok)

	arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "ok").Observe(time.Since(turnStart).Seconds())
	s.recordSessionTurn(ctx, sessionID, ag, userMsg.ID, assistantMsg.ID, prov, mod, promptTok, completionTok, assistantMsg.ContentMarkdown)
	s.setRunStatus(sessionID, runID, "completed", "")
	s.bumpSessionRevisionAndPublish(ctx, sessionID, runID, userMsg.ID)
	notifyNativeTurnHooks(ctx, s, sessionID, ag, content, assistantMsg.ContentMarkdown)
	emitter.LogDone("chat.turn.execute", "对话轮次执行完成",
		event.P("run_id", runID),
		event.P("reply_len", len(displayMarkdown)),
		event.P("prompt_tok", promptTok),
		event.P("completion_tok", completionTok),
	)

	return userMsg, assistantMsg, nil
}

func (s *ChatService) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string) {
	entry, ok := s.chatUC.DequeuePendingMessage(sessionID)
	if !ok {
		return
	}
	pendingContent := entry.Content
	pendingEntryID := entry.ID
	pendingEmitter := event.NewFlowLogger(s.td.Pipeline.Bus, s.td.Pipeline.Buffer, sessionID, ag.AgentKey)
	pendingEmitter.LogStart("chat.pending_dequeue", "排队消息开始处理", event.P("entry_id", pendingEntryID), event.P("content_len", len(pendingContent)))
	safego.Go(context.Background(), "pending-queue", func() {
		unlock := s.lockSession(sessionID)
		defer unlock()
		if s.runs.HasActive(sessionID) {
			s.chatUC.EnqueuePendingMessage(sessionID, pendingContent)
			pendingEmitter.Log("chat.pending_dequeue", event.FlowPhaseDone, "会话仍活跃，消息已重新入队", event.P("entry_id", pendingEntryID))
			return
		}
		bgCtx, cancel := context.WithTimeout(context.Background(), defaultTurnTimeout)
		s.runs.SetPendingCancel(sessionID, cancel)
		defer func() {
			cancel()
			s.runs.ClearPendingCancel(sessionID)
		}()
		req := &chatv1.SendChatMessageRequest{
			SessionId: sessionID,
			Content:   pendingContent,
		}
		var err error
		if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
			_, _, err = s.teamsNative.RunTurn(bgCtx, sess, req)
		} else {
			_, _, err = s.runSingleAgentViaTRPC(bgCtx, sess, req, ag, dialogMode, prov, mod, 0)
		}
		if err != nil {
			pendingEmitter.LogError("chat.pending_dequeue", "排队消息处理失败", event.P("entry_id", pendingEntryID), event.P("error", err.Error()))
			if s.td.Pipeline.Bus != nil {
				env := event.NewEnvelope(event.EnvelopeTypeError, "pending-queue", sessionID)
				env.Error = &event.EnvelopeError{
					Type:      "pending_failed",
					Message:   fmt.Sprintf("pending message failed: %s", err.Error()),
					PendingID: pendingEntryID,
				}
				publishCtx, publishCancel := context.WithTimeout(context.Background(), 3*time.Second)
				defer publishCancel()
				s.td.Pipeline.Bus.Publish(publishCtx, env)
			}
		} else {
			pendingEmitter.LogDone("chat.pending_dequeue", "排队消息处理完成", event.P("entry_id", pendingEntryID))
		}
	})
}

func (s *ChatService) recordSessionTurn(ctx context.Context, sessionID string, ag biz.Agent, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	if s == nil || s.td.Sessions == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	preview := strutil.ProtoPreview(contentPreview, 200)
	turn := biz.SessionTurn{
		SessionID:           sessionID,
		UserMessageID:       userMsgID,
		AssistantMessageID:  assistantMsgID,
		OwnerType:           "agent",
		AgentID:             ag.ID,
		Status:              "completed",
		StartedAt:           now,
		EndedAt:             now,
		InputTokens:         promptTok,
		OutputTokens:        completionTok,
		TotalTokens:         promptTok + completionTok,
		ModelCallCount:      1,
		FinalProvider:       prov,
		FinalModel:          mod,
		FinalContentPreview: preview,
	}
	if _, err := s.td.Sessions.CreateTurn(ctx, turn); err != nil {
		event.CtxFlowLogWarn(ctx, "chat.usage_record_fail", "会话轮次记录失败", event.P("session_id", sessionID), event.P("error", err.Error()))
	}
}

func markTurnError(turnStatus *string, turnErr *error, turnErrMsg *string, err error) {
	if err == nil {
		return
	}
	*turnStatus = "error"
	*turnErr = err
	*turnErrMsg = err.Error()
}

func (s *ChatService) recordTeamSessionTurn(ctx context.Context, sessionID, teamID, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	if s == nil || s.td.Sessions == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	preview := strutil.ProtoPreview(contentPreview, 200)
	turn := biz.SessionTurn{
		SessionID:           sessionID,
		UserMessageID:       userMsgID,
		AssistantMessageID:  assistantMsgID,
		OwnerType:           "team",
		TeamID:              teamID,
		Status:              "completed",
		StartedAt:           now,
		EndedAt:             now,
		InputTokens:         promptTok,
		OutputTokens:        completionTok,
		TotalTokens:         promptTok + completionTok,
		ModelCallCount:      1,
		FinalProvider:       prov,
		FinalModel:          mod,
		FinalContentPreview: preview,
	}
	if _, err := s.td.Sessions.CreateTurn(ctx, turn); err != nil {
		event.CtxFlowLogWarn(ctx, "chat.usage_record_fail", "团队会话轮次记录失败", event.P("session_id", sessionID), event.P("team_id", teamID), event.P("error", err.Error()))
	}
}
