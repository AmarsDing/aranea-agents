package service

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	a2apkg "aranea-agents/internal/a2a"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/agent/intent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/team"
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// RunNativeAgentTurn executes a full agent/team turn.
func (o *ChatOrchestrator) RunNativeAgentTurn(ctx context.Context, req *chatv1.SendChatMessageRequest) (userMsg biz.ChatMessage, assistantMsg biz.ChatMessage, err error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	content := strings.TrimSpace(req.GetContent())
	if sessionID == "" || content == "" {
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session_id and content are required")
	}

	flow := event.NewFlowLogger(o.td.Pipeline.Bus, o.td.Pipeline.Buffer, sessionID, "")
	flow.LogStart("chat.receive", "收到用户消息", event.P("content_len", len(content)))

	unlock := o.lockSession(sessionID)
	hasActive := o.runs.HasActive(sessionID)
	flow.Log("chat.active_check", event.FlowPhaseDone, "检查活跃运行", event.P("has_active", hasActive))
	if hasActive {
		_, _, hasRunner := o.runs.ActiveRunner(sessionID)
		switch DecideTurnAdmission(true, hasRunner) {
		case TurnAdmitEnqueue:
			unlock()
			accepted, _, _, rejectReason, err := o.chatUC.EnqueueUserMessage(sessionID, content)
			return biz.ChatMessage{}, biz.ChatMessage{}, classifyEnqueueOutcome(accepted, rejectReason, err)
		case TurnRejectBusy:
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, turnBusyError()
		}
	}

	sess, err := o.td.Sessions.Get(ctx, sessionID)
	if err != nil {
		unlock()
		flow.LogError("chat.session_fetch", "获取会话失败", event.P("error", err.Error()))
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("SESSION", "session not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.session_fetch", "会话已获取", event.P("owner_type", sess.OwnerType), event.P("agent_id", sess.AgentID), event.P("team_id", sess.TeamID))

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		if o.team.TeamsNative == nil {
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.InternalServer("CHAT_TEAM_NATIVE", "team runner not wired")
		}
		if qerr := enforceChatTurnQuotas(ctx, o.usage, "", chatagent.UserIDFromCtx(ctx)); qerr != nil {
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, qerr
		}
		if qerr := o.checkTeamMemberQuotas(ctx, strings.TrimSpace(sess.TeamID)); qerr != nil {
			unlock()
			return biz.ChatMessage{}, biz.ChatMessage{}, qerr
		}
		flow.LogStart("chat.team.invoke", "委派团队会话",
			event.P("team_id", strings.TrimSpace(sess.TeamID)), event.P("content_len", len(content)))
		runID := uuid.NewString()
		teamCtx, teamCancel := context.WithCancel(ctx)
		o.runs.StoreCancelable(sessionID, runID, teamCancel)
		o.setRunStatus(sessionID, runID, "running", "")
		unlock()
		defer func() {
			o.runs.Finish(sessionID)
			o.processPendingQueue(sessionID, sess, biz.Agent{}, "", "", "")
		}()
		userMsg, assistantMsg, err := o.team.TeamsNative.RunTurn(teamCtx, sess, req)
		if err != nil {
			o.setRunStatus(sessionID, runID, "failed", err.Error())
			o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		} else {
			o.setRunStatus(sessionID, runID, "completed", "")
			o.recordTeamSessionTurn(ctx, sessionID, strings.TrimSpace(sess.TeamID),
				userMsg.ID, assistantMsg.ID, "", "",
				assistantMsg.TokenIn, assistantMsg.TokenOut, assistantMsg.ContentMarkdown)
		}
		return userMsg, assistantMsg, err
	}

	if rtid := strings.TrimSpace(req.GetTeamId()); rtid != "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.Forbidden("CHAT_TEAM_NATIVE", "team_id is only valid for team sessions")
	}

	agentID := strings.TrimSpace(sess.AgentID)
	if agentID == "" {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.BadRequest("CHAT_NATIVE", "session has no agent_id")
	}
	ag, err := o.hydratedAgent(ctx, agentID)
	if err != nil {
		unlock()
		flow.LogError("chat.agent_hydrate", "加载Agent配置失败", event.P("agent_id", agentID), event.P("error", err.Error()))
		if errors.Is(err, sql.ErrNoRows) {
			return biz.ChatMessage{}, biz.ChatMessage{}, kerrors.NotFound("AGENT", "agent not found")
		}
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	flow.LogDone("chat.agent_hydrate", "Agent配置已加载", event.P("agent_key", ag.AgentKey), event.P("provider", ag.Provider), event.P("model", ag.Model))
	if err := enforceChatTurnQuotas(ctx, o.usage, agentID, chatagent.UserIDFromCtx(ctx)); err != nil {
		unlock()
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	opts := req.GetOptions()
	dialogMode := ""
	prov := ""
	mod := ""
	if opts != nil {
		dialogMode = strings.TrimSpace(opts.GetDialogMode())
		prov = strings.TrimSpace(opts.GetProvider())
		mod = strings.TrimSpace(opts.GetModel())
	}
	dialogMode = strutil.FirstNonEmpty(dialogMode, sess.DialogMode, "default")
	prov = strutil.FirstNonEmpty(prov, sess.DefaultProvider, ag.Provider)
	mod = strutil.FirstNonEmpty(mod, sess.DefaultModel, ag.Model)
	flow.LogDone("chat.provider_resolve", "Provider/Model已解析", event.P("provider", prov), event.P("model", mod), event.P("dialog_mode", dialogMode))

	attN := 0
	if opts != nil {
		attN = len(opts.Attachments)
	}

	flow.LogStart("chat.turn.enter", "进入Agent Turn执行", event.P("dialog_mode", dialogMode), event.P("provider", prov), event.P("model", mod), event.P("attachments", attN))

	agentRunID := uuid.NewString()
	turnCtx, turnCancel := context.WithCancel(ctx)
	o.runs.StoreCancelable(sessionID, agentRunID, turnCancel)
	unlock()
	return o.runSingleAgentViaTRPC(turnCtx, sess, req, ag, dialogMode, prov, mod, attN)
}

// hydratedAgent loads and returns an Agent by ID.
func (o *ChatOrchestrator) hydratedAgent(ctx context.Context, agentID string) (biz.Agent, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return biz.Agent{}, kerrors.BadRequest("CHAT_NATIVE", "agent id is required")
	}
	if o.td.Catalog.AgentsUC != nil {
		return o.td.Catalog.AgentsUC.Get(ctx, agentID)
	}
	if o.td.Catalog.Agents == nil {
		return biz.Agent{}, kerrors.InternalServer("CHAT_NATIVE", "agent repository not configured")
	}
	return o.td.Catalog.Agents.GetAgentByID(ctx, agentID)
}

// RunAgentTurn implements a2a.AgentTurnRunner for call_agent and HTTP Invoke dispatch.
func (o *ChatOrchestrator) RunAgentTurn(ctx context.Context, agentID, input string, timeoutSec int) (string, error) {
	if o == nil || o.td.Sessions == nil {
		return "", kerrors.InternalServer("A2A", "chat service not configured")
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
	sess, err := o.td.Sessions.Create(runCtx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   strings.TrimSpace(agentID),
		OwnerType: "agent",
		Title:     fmt.Sprintf("a2a-%s", agentID),
		UserID:    uid,
	})
	if err != nil {
		return "", kerrors.InternalServer("A2A", "create session: "+err.Error())
	}
	_, asst, err := o.RunNativeAgentTurn(runCtx, &chatv1.SendChatMessageRequest{
		SessionId: sess.ID,
		Content:   strings.TrimSpace(input),
	})
	if err != nil {
		return "", err
	}
	return asst.ContentMarkdown, nil
}

// RunEvalAgentTurn runs an evaluation agent turn.
func (o *ChatOrchestrator) RunEvalAgentTurn(ctx context.Context, agentID, input string) (string, error) {
	if o == nil || o.td.Sessions == nil {
		return "", fmt.Errorf("eval: chat service not configured")
	}
	agentID = strings.TrimSpace(agentID)
	input = strings.TrimSpace(input)
	if agentID == "" || input == "" {
		return "", fmt.Errorf("eval: agent_id and input are required")
	}
	sess, err := o.td.Sessions.Create(ctx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   agentID,
		OwnerType: "agent",
		Title:     fmt.Sprintf("eval-%s", agentID),
		UserID:    "1",
	})
	if err != nil {
		return "", fmt.Errorf("eval: create session: %w", err)
	}
	_, asst, err := o.RunNativeAgentTurn(ctx, &chatv1.SendChatMessageRequest{
		SessionId: sess.ID,
		Content:   input,
	})
	if err != nil {
		return "", err
	}
	return asst.ContentMarkdown, nil
}

// RunCronTurn dispatches a cron-triggered turn.
func (o *ChatOrchestrator) RunCronTurn(ctx context.Context, sessionID, content, teamID string) (userMsgID, agentMsgID string, err error) {
	req := &chatv1.SendChatMessageRequest{
		SessionId: sessionID,
		Content:   content,
	}
	if strings.TrimSpace(teamID) != "" {
		tid := teamID
		req.TeamId = &tid
	}
	user, asst, err := o.RunNativeAgentTurn(ctx, req)
	if err != nil {
		return "", "", err
	}
	return user.ID, asst.ID, nil
}

// injectA2AContext injects A2A invoker context.
func (o *ChatOrchestrator) injectA2AContext(ctx context.Context, callerAgentID string) context.Context {
	if o == nil || o.a2aUC == nil {
		return ctx
	}
	inv := a2apkg.NewInvoker(o, o.a2aUC, o.td.Catalog.Agents)
	return a2apkg.InjectRunContext(ctx, o.a2aUC, callerAgentID, inv)
}

// makeAwaitReplyFunc returns a ReplyFunc closure for await-reply.
func (o *ChatOrchestrator) makeAwaitReplyFunc(runCtx context.Context, sessionID, runID string) func(context.Context) (string, error) {
	return func(toolCtx context.Context) (string, error) {
		ch := make(chan awaitReplyCh, 1)
		o.chatUC.RegisterAwaitChannel(sessionID, ch)
		awaitMeta := AwaitStatusMeta{Kind: biz.ChatAwaitKindReply}
		if req, ok := serviceawaitreply.ToolConfirmRequestFromContext(toolCtx); ok {
			awaitMeta = AwaitStatusMeta{
				Kind:       biz.ChatAwaitKindToolConfirm,
				ToolKey:    req.ToolKey,
				ToolCallID: req.ToolCallID,
			}
		}
		o.setRunStatusWithAwait(sessionID, runID, "awaiting_user", "", &awaitMeta)
		o.persistAwaitMarkers(toolCtx, sessionID, runID, awaitMeta, true)
		defer func() {
			o.chatUC.DeleteAwaitChannel(sessionID)
			o.clearAwaitMetaCache(sessionID)
			o.setRunStatus(sessionID, runID, "running", "")
		}()
		select {
		case r := <-ch:
			return r.Reply, nil
		case <-toolCtx.Done():
			return "", toolCtx.Err()
		case <-runCtx.Done():
			return "", runCtx.Err()
		}
	}
}

// resumeAwaitAfterRestart resumes an await after process restart.
func (o *ChatOrchestrator) resumeAwaitAfterRestart(ctx context.Context, sessionID, reply, runID string) error {
	if !o.tryBeginResume(sessionID) {
		return errResumeInFlight
	}
	if err := o.clearAwaitingRunStateSync(ctx, sessionID); err != nil {
		o.endResume(sessionID)
		return err
	}
	o.publishAwaitResumed(sessionID, runID)
	req := &chatv1.SendChatMessageRequest{
		SessionId: sessionID,
		Content:   reply,
	}
	safego.Go(ctx, "chat.resume_await_turn", func() {
		defer o.endResume(sessionID)
		bgCtx, cancel := context.WithTimeout(context.Background(), defaultTurnTimeout)
		defer cancel()
		_, _, turnErr := o.RunNativeAgentTurn(bgCtx, req)
		if turnErr != nil && !IsTurnMessageQueued(turnErr) {
			o.setRunStatus(sessionID, runID, "failed", turnErr.Error())
			o.publishTurnFailure(sessionID, runID, "chat-service", turnErr, "")
		}
	})
	return nil
}

// checkTeamMemberQuotas rejects the turn when any enabled team member exceeds agent scope quota.
func (o *ChatOrchestrator) checkTeamMemberQuotas(ctx context.Context, teamID string) error {
	if o == nil || o.team.Teams == nil {
		return nil
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil
	}
	t, err := o.team.Teams.GetTeamByID(ctx, teamID)
	if err != nil {
		return err
	}
	def, err := team.ParseDefinition(t.DefinitionJSON)
	if err != nil {
		return nil
	}
	for _, m := range team.EnabledMembers(def) {
		if err := enforceQuota(ctx, o.usage, "agent", m.AgentID); err != nil {
			return err
		}
	}
	return nil
}

// bumpSessionRevisionAndPublish bumps revision and publishes event.
func (o *ChatOrchestrator) bumpSessionRevisionAndPublish(ctx context.Context, sessionID, runID, turnID string) {
	if o == nil || o.td.Sessions == nil || o.td.Pipeline.Bus == nil {
		return
	}
	event.BumpAndPublishSessionRevision(
		ctx,
		o.td.Sessions,
		o.td.Pipeline.Bus,
		sessionID,
		runID,
		turnID,
		event.EnvelopeSourceFromContext(ctx),
	)
}

// buildUserMessage constructs a trpcmodel.Message from content and attachment refs.
func (o *ChatOrchestrator) buildUserMessage(ctx context.Context, sessionID, content string, refs []*chatv1.AttachmentRef) (trpcmodel.Message, error) {
	content = strings.TrimSpace(content)
	if len(refs) == 0 {
		return trpcmodel.NewUserMessage(content), nil
	}
	if o == nil || o.artifacts == nil {
		if content == "" {
			return trpcmodel.Message{}, fmt.Errorf("attachments require artifact service")
		}
		return trpcmodel.NewUserMessage(content), nil
	}
	parts := make([]trpcmodel.ContentPart, 0, len(refs)+1)
	if content != "" {
		text := content
		parts = append(parts, trpcmodel.ContentPart{Type: trpcmodel.ContentTypeText, Text: &text})
	}
	for _, ref := range refs {
		if ref == nil {
			continue
		}
		id := strings.TrimSpace(ref.GetId())
		if id == "" {
			continue
		}
		meta, data, err := o.artifacts.Load(ctx, id, 0)
		if err != nil {
			return trpcmodel.Message{}, fmt.Errorf("load attachment %s: %w", id, err)
		}
		if strings.TrimSpace(meta.SessionID) != "" && strings.TrimSpace(sessionID) != "" && meta.SessionID != sessionID {
			return trpcmodel.Message{}, fmt.Errorf("attachment %s belongs to another session", id)
		}
		mime := strings.ToLower(strings.TrimSpace(meta.MimeType))
		if strings.HasPrefix(mime, "image/") {
			format := strings.TrimPrefix(mime, "image/")
			parts = append(parts, trpcmodel.ContentPart{
				Type: trpcmodel.ContentTypeImage,
				Image: &trpcmodel.Image{
					Data:   data,
					Format: format,
					Detail: "auto",
				},
			})
			continue
		}
		parts = append(parts, trpcmodel.ContentPart{
			Type: trpcmodel.ContentTypeFile,
			File: &trpcmodel.File{
				Name:     meta.Name,
				Data:     data,
				MimeType: meta.MimeType,
			},
		})
	}
	if len(parts) == 0 {
		return trpcmodel.NewUserMessage(content), nil
	}
	return trpcmodel.Message{Role: trpcmodel.RoleUser, ContentParts: parts}, nil
}

// runSingleAgentViaTRPC runs a single agent turn via the trpc-agent-go framework.
func (o *ChatOrchestrator) runSingleAgentViaTRPC(
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
		te := TurnError(TurnErrAgentForbidden, "")
		o.publishTurnFailure(sessionID, "", "chat-service", te, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, te
	}

	runID := uuid.NewString()
	durableCtx := durableResumeTurnCtxFrom(ctx, runID, dialogMode, prov, mod)
	runID = durableCtx.runID
	dialogMode = durableCtx.dialogMode
	prov = durableCtx.provider
	mod = durableCtx.model
	turnStart := time.Now()
	biz.DefaultTurnCompletionBridge().RegisterTurnStart(sessionID, runID, turnStart)
	turnStatus := "ok"
	var turnErr error
	var resultPromptTok, resultCompletionTok int
	var turnErrMsg string
	ctx, traceBridge, _ := startTurnSpan(ctx, "chat.turn", sessionID, ag.AgentKey, runID)
	emitter := event.NewTraceEmitterForRun(event.TraceEmitterOpts{
		Ctx: ctx, Bus: o.td.Pipeline.Bus, Buffer: o.td.Pipeline.Buffer,
		SessionID: sessionID, RunID: runID, AgentKey: ag.AgentKey, AgentID: ag.ID,
		Domain: event.TraceDomainChat,
	})
	emitter.SetOtelRefs(traceBridge.TraceID(), traceBridge.RootSpanID())
	ctx = event.WithTraceEmitter(ctx, emitter)
	defer func() {
		emitter.FinishRoot(turnStatus)
		endTurnSpan(traceBridge, turnErr)
		o.recordTurnUsage(ctx, emitter, sessionID, runID, ag.AgentKey, ag.ID, prov, mod, turnStatus,
			resultPromptTok, resultCompletionTok, time.Since(turnStart), turnErrMsg)
	}()

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, defaultTurnTimeout)
		defer cancel()
	}

	deps := chatagent.TRPCBuilderDeps{
		Catalog:            o.td.Catalog.LLM,
		AgentUC:            o.td.Catalog.AgentsUC,
		Agents:             o.td.Catalog.Agents,
		RT:                 o.td.RoundTrip(),
		SkillUC:            o.td.Catalog.SkillUC,
		MCPTooling:         o.td.Persist.AgentMCP,
		ToolUC:             o.td.Catalog.ToolUC,
		Sessions:           o.td.Sessions,
		Sys:                o.td.Catalog.Settings,
		Provider:           prov,
		Model:              mod,
		DialogMode:         dialogMode,
		SkillDBRepo:        o.rt.SkillDBRepo,
		AwaitHook:          o.makeAwaitReplyFunc(ctx, sessionID, runID),
		HasMemory:          o.td.Persist.Memory.Available(),
		PluginManager:      o.rt.PluginManager,
		MemoryAdmin:        o.td.Persist.Memory.Admin,
		KnowledgeRetriever: o.rt.KnowledgeRetriever,
		CodeExecFactory:    o.rt.CodeExecFactory,
	}
	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		emitter.LogError("chat.agent.build", "构建Agent实例失败", event.P("agent_id", ag.ID), event.P("error", err.Error()))
		o.runs.Finish(sessionID)
		te := TurnError(TurnErrAgentBuildFailed, err.Error())
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, te
	}
	emitter.LogDone("chat.agent.build", "Agent实例已构建", event.P("provider", prov), event.P("model", mod))

	var plugins []trpcplugin.Plugin
	if o.rt.PluginManager != nil {
		plugins = o.rt.PluginManager.RunnerPluginsForAgent(ag.ID)
	} else if o.rt.PluginRT != nil {
		plugins = o.rt.PluginRT.PluginsForAgent(ag.ID)
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
	runner, err := o.td.CoalesceRunnerManager().NewTurnRunner(root, rt.TurnRunnerSpec{
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
		o.runs.Finish(sessionID)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	emitter.LogDone("chat.runner.create", "Runner 已创建")
	o.runs.StoreRunner(sessionID, runID, runner)
	o.setRunStatus(sessionID, runID, "running", "")
	emitter.LogStart("chat.turn.execute", "开始执行对话轮次", event.P("run_id", runID))
	defer func() {
		o.runs.Finish(sessionID)
		runner.Close()
		o.processPendingQueue(sessionID, sess, ag, dialogMode, prov, mod)
	}()
	userOpts, err := chatagent.UserOptionsJSON(ag, dialogMode, prov, mod, sess.ContextUsedRatio, nil)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if src := event.EnvelopeSourceFromContext(ctx); src != "" {
		userOpts, err = chatagent.MergeInboundSourceIntoUserOptionsJSON(
			userOpts,
			src,
			event.EnvelopePlatformFromContext(ctx),
			event.EnvelopeChannelKeyFromContext(ctx),
		)
		if err != nil {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
			o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
	}
	sendText := content
	if !biz.IsA2AProxyAgent(ag) {
		emitter.LogStart("chat.intent.pass", "意图识别开始", event.P("provider", prov), event.P("model", mod), event.P("content_len", len(content)))
		intRes := intent.Run(ctx, intent.IntentPassFromAgent(ag), o.td.Catalog.LLM, o.td.LLMHTTP, prov, mod, content)
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
		if o.td.Pipeline.Bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeIntentPass, ag.ID, sessionID)
			env.Metadata = intentPayload
			o.td.Pipeline.Bus.Publish(ctx, env)
		}
	} else {
		emitter.LogSkip("chat.intent.pass", "A2A Proxy Agent 跳过意图识别", event.P("agent_kind", ag.Kind))
	}

	now := chatagent.RFC3339Now()
	var userMsg biz.ChatMessage
	if durableCtx.active {
		userMsg = durableCtx.buildUserMessage(sessionID, userOpts, attN, emitter)
	} else {
		userMsg = biz.ChatMessage{
			ID:               uuid.NewString(),
			SessionID:        sessionID,
			Role:             "user",
			ContentMarkdown:  content,
			Status:           "ok",
			OptionsJSON:      userOpts,
			CreatedAt:        now,
			AttachmentsCount: attN,
		}
		if err := o.td.Sessions.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
			o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
		emitter.LogDone("chat.user_msg_persist", "用户消息已持久化")
	}

	var sessionRunID string
	var stopBudget context.CancelFunc
	sessionRunID, stopBudget = o.durableSessionRunLifecycle(ctx, emitter, sess, ag, durableCtx, userMsg, content)
	defer stopBudget()
	defer func() {
		o.finishSessionRunLifecycle(ctx, sessionID, sessionRunID, turnErr)
	}()

	uid := chatagent.UserIDFromCtx(ctx)
	runOpts := durableResumeRunOpts(durableCtx.active, []trpcagent.RunOption{trpcagent.WithRequestID(sessionID), skillruntime.RunOptionWithTurnQuery(sendText)})
	if ag.Settings != nil {
		if vars := chatagent.ParseVariablesJSON(ag.Settings.VariablesJSON); vars != nil {
			runOpts = append(runOpts, trpcagent.MergeRuntimeState(vars))
		}
	}
	firstByteTimeout := chatagent.DefaultFirstByteTimeout
	if custom, ok := firstByteTimeoutFromContext(ctx); ok {
		firstByteTimeout = custom
	}
	runCtx := serviceawaitreply.WithReplyFunc(ctx, deps.AwaitHook)
	runCtx = o.injectA2AContext(runCtx, ag.ID)
	if o.rt.KnowledgeRetriever != nil {
		runCtx = knowledgetool.WithRetriever(runCtx, o.rt.KnowledgeRetriever)
	}
	if req.GetOptions() != nil {
		if cols := req.GetOptions().GetKnowledgeBases(); len(cols) > 0 {
			runCtx = knowledgetool.WithKnowledgeCollections(runCtx, cols)
		}
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
	userTurnMsg, err := o.buildUserMessage(runCtx, sessionID, sendText, attachmentRefs)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		emitter.LogError("chat.llm.invoke", "附件装配失败", event.P("error", err.Error()))
		o.setRunStatus(sessionID, runID, "failed", err.Error())
		te := TurnError(TurnErrAttachmentFailed, err.Error())
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return userMsg, biz.ChatMessage{}, te
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
		o.setRunStatus(sessionID, runID, "failed", err.Error())
		te := TurnError(TurnErrLLMCallFailed, err.Error())
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return userMsg, biz.ChatMessage{}, te
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
		ContextWindow:    ag.ContextWindow,
		Source:           event.EnvelopeSourceFromContext(ctx),
	}
	if projectMeta.ContextWindow <= 0 {
		projectMeta.ContextWindow = 128000
	}
	events = event.WrapFrameworkEventsWithOtel(events, emitter, traceBridge, traceBridge)
	streamOpts := NewChatStreamConsumeOptions(o.td.Catalog.ToolUC, o.td.Catalog.Agents, o.td.Sessions)
	result, streamErr := chatagent.ConsumeWithFirstByteGuard(runCtx, firstByteTimeout, events, o.td.Pipeline.Bus, projectMeta, streamOpts)
	resultPromptTok = result.PromptTok
	resultCompletionTok = result.CompletionTok
	if streamErr != nil {
		if errors.Is(streamErr, chatagent.ErrFirstByteTimeout) {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, streamErr)
			emitter.LogCritical("chat.first_byte_timeout", "首字节超时，模型响应过慢", event.P("timeout", firstByteTimeout.String()))
			arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "first_byte_timeout").Observe(time.Since(turnStart).Seconds())
			o.setRunStatus(sessionID, runID, "failed", "first byte timeout")
			te := TurnError(TurnErrFirstByteTimeout, firstByteTimeout.String())
			o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
			return userMsg, biz.ChatMessage{}, te
		}
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, streamErr)
		o.setRunStatus(sessionID, runID, "failed", streamErr.Error())
		o.publishTurnFailure(sessionID, runID, "chat-service", streamErr, "")
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
		o.setRunStatus(sessionID, runID, "failed", "turn timeout")
		te := TurnError(TurnErrTurnTimeout, defaultTurnTimeout.String())
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return userMsg, biz.ChatMessage{}, te
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
		o.setRunStatus(sessionID, runID, "failed", detail)
		te := TurnError(TurnErrEmptyReply, detail)
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return userMsg, biz.ChatMessage{}, te
	}

	promptTok, completionTok := chatagent.EstimateTokensIfMissing(resultPromptTok, resultCompletionTok, content, displayMarkdown)

	assistantOptsStr, err := chatagent.AssistantOptionsJSON(ag, nil)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return userMsg, biz.ChatMessage{}, err
	}
	if s := result.Reasoning.String(); s != "" {
		if assistantOptsStr, err = chatagent.MergeReasoningIntoAssistantOptionsJSON(assistantOptsStr, s); err != nil {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
			o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
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
	if err := o.td.Sessions.AppendChatMessage(ctx, sessionID, assistantMsg, true); err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return userMsg, biz.ChatMessage{}, err
	}
	emitter.LogDone("chat.assistant_msg_persist", "助手消息已持久化", event.P("reply_len", len(displayMarkdown)))
	o.patchSessionContextUsage(ctx, sessionID, ag, promptTok, completionTok)

	arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "ok").Observe(time.Since(turnStart).Seconds())
	o.recordSessionTurn(ctx, sessionID, ag, userMsg.ID, assistantMsg.ID, prov, mod, promptTok, completionTok, assistantMsg.ContentMarkdown)
	o.setRunStatus(sessionID, runID, "completed", "")
	o.bumpSessionRevisionAndPublish(ctx, sessionID, runID, userMsg.ID)
	o.notifyNativeTurnHooks(ctx, sessionID, ag, content, assistantMsg.ContentMarkdown)
	emitter.LogDone("chat.turn.execute", "对话轮次执行完成",
		event.P("run_id", runID),
		event.P("reply_len", len(displayMarkdown)),
		event.P("prompt_tok", promptTok),
		event.P("completion_tok", completionTok),
	)

	return userMsg, assistantMsg, nil
}

// processPendingQueue handles the next pending message after a turn completes.
func (o *ChatOrchestrator) processPendingQueue(sessionID string, sess biz.Session, ag biz.Agent, dialogMode, prov, mod string) {
	entry, ok := o.chatUC.DequeuePendingMessage(sessionID)
	if !ok {
		return
	}
	pendingContent := entry.Content
	pendingEntryID := entry.ID
	pendingEmitter := event.NewFlowLogger(o.td.Pipeline.Bus, o.td.Pipeline.Buffer, sessionID, ag.AgentKey)
	pendingEmitter.LogStart("chat.pending_dequeue", "排队消息开始处理", event.P("entry_id", pendingEntryID), event.P("content_len", len(pendingContent)))
	safego.Go(context.Background(), "pending-queue", func() {
		unlock := o.lockSession(sessionID)
		defer unlock()
		if o.runs.HasActive(sessionID) {
			o.chatUC.EnqueuePendingMessage(sessionID, pendingContent)
			pendingEmitter.Log("chat.pending_dequeue", event.FlowPhaseDone, "会话仍活跃，消息已重新入队", event.P("entry_id", pendingEntryID))
			return
		}
		bgCtx, cancel := context.WithTimeout(context.Background(), defaultTurnTimeout)
		o.runs.SetPendingCancel(sessionID, cancel)
		defer func() {
			cancel()
			o.runs.ClearPendingCancel(sessionID)
		}()
		req := &chatv1.SendChatMessageRequest{
			SessionId: sessionID,
			Content:   pendingContent,
		}
		var err error
		if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
			_, _, err = o.team.TeamsNative.RunTurn(bgCtx, sess, req)
		} else {
			_, _, err = o.runSingleAgentViaTRPC(bgCtx, sess, req, ag, dialogMode, prov, mod, 0)
		}
		if err != nil {
			pendingEmitter.LogError("chat.pending_dequeue", "排队消息处理失败", event.P("entry_id", pendingEntryID), event.P("error", err.Error()))
			o.publishTurnFailure(sessionID, "", "pending-queue", err, pendingEntryID)
		} else {
			pendingEmitter.LogDone("chat.pending_dequeue", "排队消息处理完成", event.P("entry_id", pendingEntryID))
		}
	})
}

// recordSessionTurn records a completed agent turn.
func (o *ChatOrchestrator) recordSessionTurn(ctx context.Context, sessionID string, ag biz.Agent, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	if o == nil || o.td.Sessions == nil {
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
	if _, err := o.td.Sessions.CreateTurn(ctx, turn); err != nil {
		event.CtxFlowLogWarn(ctx, "chat.usage_record_fail", "会话轮次记录失败", event.P("session_id", sessionID), event.P("error", err.Error()))
	}
}

// recordTeamSessionTurn records a completed team turn.
func (o *ChatOrchestrator) recordTeamSessionTurn(ctx context.Context, sessionID, teamID, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	if o == nil || o.td.Sessions == nil {
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
	if _, err := o.td.Sessions.CreateTurn(ctx, turn); err != nil {
		event.CtxFlowLogWarn(ctx, "chat.usage_record_fail", "会话轮次记录失败", event.P("session_id", sessionID), event.P("error", err.Error()))
	}
}

// recordTurnUsage records token usage for a turn.
func (o *ChatOrchestrator) recordTurnUsage(
	ctx context.Context,
	emitter *event.TraceEmitter,
	sessionID, runID, agentKey, agentID, prov, mod, status string,
	promptTok, completionTok int,
	latency time.Duration,
	errMsg string,
) {
	if o == nil || o.usage == nil {
		return
	}
	now := time.Now().UTC()
	meta := "{}"
	if emitter != nil {
		meta = emitter.MetadataJSON()
	}
	usageID := uuid.NewString()
	traceID := ""
	if emitter != nil {
		traceID = emitter.TraceID()
	}
	ev := biz.TokenUsageEvent{
		ID:               usageID,
		SessionID:        sessionID,
		AgentKey:         agentKey,
		AgentID:          agentID,
		ModelAPIID:       mod,
		ModelDisplayName: mod,
		ProviderCode:     prov,
		InputTokens:      promptTok,
		OutputTokens:     completionTok,
		TotalTokens:      promptTok + completionTok,
		LatencyMS:        int(latency.Milliseconds()),
		Status:           status,
		UsageKind:        biz.UsageKindChatTurn,
		MetadataJSON:     meta,
		OccurredAt:       now.Format(time.RFC3339),
		DateKey:          now.Format("2006-01-02"),
		HourKey:          now.Format("2006-01-02T15"),
		ErrorMessage:     errMsg,
	}
	if runID != "" {
		ev.MessageID = runID
	}
	recCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 45*time.Second)
	defer cancel()
	if _, err := o.usage.RecordTokenUsageEvent(recCtx, ev); err != nil && emitter != nil {
		emitter.LogError("chat.usage_record", "用量落库失败",
			event.P("error", err.Error()),
			event.P("run_id", runID),
			event.P("usage_kind", ev.UsageKind),
			event.P("status", status),
		)
		return
	}
	if o.monitor != nil && sessionID != "" && runID != "" {
		linkCtx, linkCancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer linkCancel()
		if err := biz.LinkRunnerCompletionUsage(linkCtx, o.monitor, sessionID, runID, usageID, traceID); err != nil && emitter != nil {
			emitter.LogWarn("chat.completion_link", "关联 runner.completion 失败", err.Error(),
				event.P("run_id", runID),
				event.P("usage_event_id", usageID),
			)
		}
	}
}

// patchSessionContextUsage updates session context usage after a turn.
func (o *ChatOrchestrator) patchSessionContextUsage(ctx context.Context, sessionID string, ag biz.Agent, promptTok, completionTok int) {
	if o == nil || o.td.Sessions == nil {
		return
	}
	win := ag.ContextWindow
	if win <= 0 {
		win = 128000
	}
	_ = o.td.Sessions.UpdateSessionContextFromLLMUsage(ctx, sessionID, promptTok, completionTok, win)
	if o.td.Compress != nil {
		o.td.Compress.AfterNativeTurn(ctx, sessionID, ag)
	}
}

// notifyNativeTurnHooks runs post-turn side effects.
func (o *ChatOrchestrator) notifyNativeTurnHooks(ctx context.Context, sessionID string, ag biz.Agent, userInput, assistantOutput string) {
	if o == nil || o.td.AfterTurn == nil {
		return
	}
	o.td.AfterTurn.AfterNativeTurn(ctx, biz.NativeTurnEvent{
		AgentID:         ag.ID,
		AgentConfigJSON: ag.ConfigJSON,
		SessionID:       sessionID,
		UserInput:       userInput,
		AssistantOutput: assistantOutput,
	})
}

// nativeSendChatMessage is the native implementation of SendChatMessage.
func (o *ChatOrchestrator) nativeSendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	userMsg, assistantMsg, err := o.RunNativeAgentTurn(ctx, req)
	if err != nil {
		if IsTurnMessageQueued(err) {
			return &chatv1.SendChatMessageResponse{}, nil
		}
		return nil, err
	}
	um := chatMessageToMap(userMsg)
	am := chatMessageToMap(assistantMsg)
	out := &chatv1.SendChatMessageResponse{}
	if st, err := structpb.NewStruct(um); err != nil {
		return nil, kerrors.InternalServer("CHAT_NATIVE", fmt.Sprintf("encode user_message: %v", err))
	} else {
		out.UserMessage = st
	}
	if st, err := structpb.NewStruct(am); err != nil {
		return nil, kerrors.InternalServer("CHAT_NATIVE", fmt.Sprintf("encode agent_message: %v", err))
	} else {
		out.AgentMessage = st
	}
	if tid := strings.TrimSpace(req.GetTeamId()); tid != "" {
		if o.td.Pipeline.Bus != nil {
			env := event.NewEnvelope(event.EnvelopeTypeTeamRunFinished, "chat-native", "")
			env.TeamID = tid
			env.Metadata = map[string]any{"hint": true}
			o.td.Pipeline.Bus.Publish(ctx, env)
		}
	}
	return out, nil
}

// nativeGetChatOptions returns chat options.
func (o *ChatOrchestrator) nativeGetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
	typed := strings.TrimSpace(req.GetType())
	switch typed {
	case "", "dialog_mode":
		return &chatv1.GetChatOptionsResponse{Items: nativeDialogModeChatOptions()}, nil
	case "provider":
		return o.nativeGetProviderOptions(ctx)
	case "model":
		return o.nativeGetModelOptions(ctx)
	default:
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
}

func (o *ChatOrchestrator) nativeGetProviderOptions(ctx context.Context) (*chatv1.GetChatOptionsResponse, error) {
	if o.td.Catalog.LLM == nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	rows, err := o.td.Catalog.LLM.List(ctx)
	if err != nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	seen := make(map[string]struct{})
	var items []*chatv1.ChatOption
	for _, row := range rows {
		p := strings.TrimSpace(row.Provider)
		if p == "" || row.Enabled == false {
			continue
		}
		if _, ok := seen[p]; ok {
			continue
		}
		seen[p] = struct{}{}
		items = append(items, &chatv1.ChatOption{
			Type:      "provider",
			Key:       p,
			Label:     p,
			Enabled:   true,
			SortOrder: int32(len(items) + 1),
		})
	}
	return &chatv1.GetChatOptionsResponse{Items: items}, nil
}

func (o *ChatOrchestrator) nativeGetModelOptions(ctx context.Context) (*chatv1.GetChatOptionsResponse, error) {
	if o.td.Catalog.LLM == nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	rows, err := o.td.Catalog.LLM.List(ctx)
	if err != nil {
		return &chatv1.GetChatOptionsResponse{Items: nil}, nil
	}
	var items []*chatv1.ChatOption
	for i, row := range rows {
		if row.Enabled == false {
			continue
		}
		type modelMeta struct {
			Provider string `json:"provider,omitempty"`
			Model    string `json:"model,omitempty"`
		}
		mj := "{}"
		if row.Provider != "" || row.Model != "" {
			if b, err := json.Marshal(modelMeta{Provider: row.Provider, Model: row.Model}); err == nil {
				mj = string(b)
			}
		}
		label := row.Name
		if label == "" {
			label = row.Key
		}
		if label == "" {
			label = row.Model
		}
		items = append(items, &chatv1.ChatOption{
			Type:         "model",
			Key:          row.Key,
			Label:        label,
			Enabled:      true,
			SortOrder:    int32(i + 1),
			MetadataJson: mj,
		})
	}
	return &chatv1.GetChatOptionsResponse{Items: items}, nil
}
