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
	artifactbiz "aranea-agents/internal/biz/artifact"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/event"
	arametrics "aranea-agents/internal/metrics"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/runtime/turn"
	sessctx "aranea-agents/internal/session"
	"aranea-agents/internal/team"
	"aranea-agents/internal/telemetry/turntrace"
	knowledgetool "aranea-agents/internal/tools/knowledge"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
	"aranea-agents/pkg/strutil"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
	trpcplugin "trpc.group/trpc-go/trpc-agent-go/plugin"

	"github.com/google/uuid"
	"google.golang.org/protobuf/types/known/structpb"

	kerrors "github.com/go-kratos/kratos/v2/errors"
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
		return biz.NativeTurnResult{}, kerrors.BadRequest("CHAT_NATIVE", "session_id and content are required")
	}

	if ep := strings.TrimSpace(string(input.EntryConfig.EntryPoint)); ep != "" {
		ctx = event.WithEnvelopeSource(ctx, ep)
	}

	flow := event.NewFlowLogger(o.td.Pipeline.Bus, o.td.Pipeline.Buffer, sessionID, "")
	flow.LogStart("chat.receive", "收到用户消息", event.P("content_len", len(content)))

	hasActive := o.runs.HasActive(sessionID)
	flow.Log("chat.active_check", event.FlowPhaseDone, "检查活跃运行", event.P("has_active", hasActive))
	contextPressure := o.sessionContextPressure(ctx, input)
	if verdict, handled := o.checkTurnAdmission(input, hasActive, contextPressure); handled {
		return nativeResultFromAdmissionVerdict(verdict)
	}

	userMsg, assistantMsg, err := o.runNativeAgentTurnBody(ctx, input, flow)
	if err != nil {
		if IsTurnMessageQueued(err) {
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
	if o == nil || o.admitGate == nil || !hasActive {
		return turn.AdmissionVerdict{}, false
	}
	hasRunner := o.HasActiveRunner(input.SessionID)
	policy := ingressPolicyFromTurnInput(input, true, hasRunner, contextPressure)
	recordIngressIntentMetric(policy.Intent)
	if policy.Decision == IngressRejectBusy {
		return turn.AdmissionVerdict{Action: turn.AdmissionRejectBusy}, true
	}
	verdict := o.admitGate.Check(input)
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

	releaseLane := rt.AcquireTurnLane(ctx, input, sess.OwnerType)
	defer releaseLane()

	if strings.EqualFold(strings.TrimSpace(sess.OwnerType), "team") {
		return o.executeTeamTurnViaHooks(ctx, sess, input, flow, unlock)
	}

	if rtid := strings.TrimSpace(input.TeamID); rtid != "" {
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
	if o.td.Catalog.Settings != nil {
		if refine, err := o.td.Catalog.Settings.GetRefineLLM(ctx); err == nil {
			prov = strutil.FirstNonEmpty(prov, refine.Provider)
			mod = strutil.FirstNonEmpty(mod, refine.Model)
		}
	}
	if prov != "" && mod != "" {
		return prov, mod
	}
	if o.td.Catalog.LLM != nil {
		if models, err := o.td.Catalog.LLM.List(ctx); err == nil {
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
	if o.td.Sessions == nil {
		return
	}
	p, m := prov, mod
	_, _ = o.td.Sessions.Update(ctx, sessionID, biz.SessionUpdateFields{
		DefaultProvider: &p,
		DefaultModel:    &m,
	})
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
		return "", kerrors.InternalServer("CHAT", "a2a turn outcome: "+string(tr.Outcome))
	}
	return tr.AssistantMsg.ContentMarkdown, nil
}

// RunEvalAgentTurn runs an evaluation agent turn.
func (o *ChatOrchestrator) RunEvalAgentTurn(ctx context.Context, agentID, input string) (string, error) {
	if o == nil || o.td.Sessions == nil {
		return "", kerrors.InternalServer("CHAT", "eval: chat service not configured")
	}
	agentID = strings.TrimSpace(agentID)
	input = strings.TrimSpace(input)
	if agentID == "" || input == "" {
		return "", kerrors.BadRequest("CHAT", "eval: agent_id and input are required")
	}
	sess, err := o.td.Sessions.Create(ctx, biz.Session{
		ID:        uuid.NewString(),
		AgentID:   agentID,
		OwnerType: "agent",
		Title:     fmt.Sprintf("eval-%s", agentID),
		UserID:    "1",
	})
	if err != nil {
		return "", kerrors.InternalServer("CHAT", "eval: create session: "+err.Error())
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

// injectA2AContext injects A2A invoker context.
func (o *ChatOrchestrator) injectA2AContext(ctx context.Context, callerAgentID string) context.Context {
	if o == nil || o.a2aUC == nil {
		return ctx
	}
	inv := a2apkg.NewInvoker(o, o.a2aUC, o.td.Catalog.Agents, o.lg)
	return a2apkg.InjectRunContext(ctx, o.a2aUC, callerAgentID, inv)
}

// makeAwaitReplyFunc returns a ReplyFunc closure for await-reply.
func (o *ChatOrchestrator) makeAwaitReplyFunc(runCtx context.Context, sessionID, runID string) func(context.Context) (string, error) {
	return func(toolCtx context.Context) (string, error) {
		ch := make(biz.AwaitChannel, 1)
		o.chatUC.RegisterAwaitChannel(sessionID, ch)
		awaitMeta := AwaitStatusMeta{Kind: biz.ChatAwaitKindReply}
		if req, ok := serviceawaitreply.ToolConfirmRequestFromContext(toolCtx); ok {
			awaitMeta = AwaitStatusMeta{
				Kind:       biz.ChatAwaitKindToolConfirm,
				ToolKey:    req.ToolKey,
				ToolCallID: req.ToolCallID,
			}
		}
		o.setRunStatusWithAwait(toolCtx, sessionID, runID, "awaiting_user", "", &awaitMeta)
		if awaitMeta.Kind == biz.ChatAwaitKindToolConfirm {
			o.transitionSessionStatus(toolCtx, sessionID, sessstatus.SessionStatusAwaitingConfirmation, sessstatus.StatusReasonToolConfirmation)
		} else {
			o.transitionSessionStatus(toolCtx, sessionID, sessstatus.SessionStatusAwaitingConfirmation, sessstatus.StatusReasonAgentAwaitingReply)
		}
		o.persistAwaitMarkers(toolCtx, sessionID, runID, awaitMeta, true)
		defer func() {
			o.chatUC.DeleteAwaitChannel(sessionID)
			o.clearAwaitMetaCache(sessionID)
			o.setRunStatus(toolCtx, sessionID, runID, "running", "")
			o.transitionSessionStatus(toolCtx, sessionID, sessstatus.SessionStatusRunning, "")
		}()
		select {
		case r, ok := <-ch:
			if !ok {
				return "", toolCtx.Err()
			}
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
	safego.Go(ctx, "chat.resume_await_turn", func() {
		defer o.endResume(sessionID)
		bgCtx, cancel := context.WithTimeout(context.Background(), o.turnTimeout)
		defer cancel()
		_, _, turnErr := o.RunNativeAgentTurnFromInput(bgCtx, biz.TurnInput{
			SessionID: sessionID,
			Content:   reply,
		})
		if turnErr != nil && !IsTurnMessageQueued(turnErr) {
			o.setRunStatus(bgCtx, sessionID, runID, "failed", turnErr.Error())
			o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
			o.publishTurnFailure(sessionID, runID, "chat-service", turnErr, "")
		}
	})
	return nil
}

// checkTeamMemberQuotas rejects the turn when any enabled team member exceeds agent scope quota.
func (o *ChatOrchestrator) checkTeamMemberQuotas(ctx context.Context, teamID string) error {
	if o == nil || o.team.TeamUC == nil {
		return nil
	}
	teamID = strings.TrimSpace(teamID)
	if teamID == "" {
		return nil
	}
	t, err := o.team.TeamUC.Get(ctx, teamID)
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

// bumpSessionRevisionAndPublish bumps revision after turn completion (status=completed).
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
		o.lg,
	)
}

// bumpSessionRevisionSyncAndPublish bumps revision after user message persist (status=sync).
func (o *ChatOrchestrator) bumpSessionRevisionSyncAndPublish(ctx context.Context, sessionID, runID, turnID string) {
	if o == nil || o.td.Sessions == nil || o.td.Pipeline.Bus == nil {
		return
	}
	event.BumpAndPublishSessionRevisionSync(
		ctx,
		o.td.Sessions,
		o.td.Pipeline.Bus,
		sessionID,
		runID,
		turnID,
		event.EnvelopeSourceFromContext(ctx),
		o.lg,
	)
}

// notifySessionRevisionSync notifies Web of the current revision without incrementing (durable resume).
func (o *ChatOrchestrator) notifySessionRevisionSync(ctx context.Context, sessionID, runID, turnID string) {
	if o == nil || o.td.Sessions == nil || o.td.Pipeline.Bus == nil {
		return
	}
	event.NotifySessionRevisionSync(
		ctx,
		o.td.Sessions,
		o.td.Pipeline.Bus,
		sessionID,
		runID,
		turnID,
		event.EnvelopeSourceFromContext(ctx),
	)
}

// buildUserMessage constructs a trpcmodel.Message from content and attachment IDs.
func (o *ChatOrchestrator) buildUserMessage(ctx context.Context, sessionID, content string, attachmentIDs []string) (trpcmodel.Message, error) {
	return chatagent.BuildUserMessageFromArtifacts(ctx, o.artifacts, sessionID, content, attachmentIDs)
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
	content := strings.TrimSpace(input.Content)
	if ak := strings.TrimSpace(input.AgentKey); ak != "" && !strings.EqualFold(ak, ag.AgentKey) {
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
	if durableCtx.active {
		if comp, ok := o.td.Compress.(biz.DurableTurnCompressor); ok {
			if err := comp.BeforeDurableTurn(ctx, sessionID, ag); err != nil {
				o.lg.Warn("BeforeDurableTurn failed", loggateway.StepID("chat.turn.before_durable"), loggateway.Err(err))
			}
		}
	}
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
		if turnStatus != "ok" && resultPromptTok > 0 {
			o.patchSessionContextUsage(ctx, sessionID, sess, ag, prov, mod, resultPromptTok, resultCompletionTok)
		}
	}()

	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, o.turnTimeout)
		defer cancel()
	}

	attachmentRefs, err := o.resolveUserAttachmentRefs(ctx, sessionID, input.Options.AttachmentIDs)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	if err := o.validateTurnAttachmentCapabilities(ctx, prov, mod, attachmentRefs); err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		emitter.LogWarn("chat.attachment.preflight", "模型不支持当前附件类型", "", event.P("provider", prov), event.P("model", mod), event.P("error", err.Error()))
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}

	deps := chatagent.TRPCBuilderDeps{
		Catalog:               o.td.Catalog.LLM,
		AgentUC:               o.td.Catalog.AgentsUC,
		Agents:                o.td.Catalog.Agents,
		RT:                    o.td.RoundTrip(),
		SkillUC:               o.td.Catalog.SkillUC,
		MCPTooling:            o.td.Persist.AgentMCP,
		ToolUC:                o.td.Catalog.ToolUC,
		Sessions:              o.td.Sessions,
		Sys:                   o.td.Catalog.Settings,
		Provider:              prov,
		Model:                 mod,
		DialogMode:            dialogMode,
		SkillDBRepo:           o.rt.SkillDBRepo,
		AwaitHook:             o.makeAwaitReplyFunc(ctx, sessionID, runID),
		HasMemory:             o.td.Persist.Memory.Available(),
		MemoryService:         o.td.Persist.Memory.TRPC,
		PluginManager:         o.rt.PluginManager,
		MemoryAdmin:           o.td.Persist.Memory.Admin,
		MemoryL2Recall:        o.td.Persist.Memory.L2Recall,
		MemoryL3Recall:        o.td.Persist.Memory.L3Recall,
		MemoryCompositeRecall: o.td.Persist.Memory.CompositeRecall,
		KnowledgeRetriever:    o.rt.KnowledgeRetriever,
		KnowledgeUsecase:      o.rt.KnowledgeUC,
		CodeExecFactory:       o.rt.CodeExecFactory,
		CustomTools:           o.cliAdminTools(ctx, ag),
		KanbanBridge:          o.rt.KanbanBridge,
		IndustryUC:            o.rt.IndustryUC,
		DepartmentUC:          o.rt.DepartmentUC,
		PositionUC:            o.rt.PositionUC,
		ToolResultGate:        o.rt.ToolResultGate,
	}
	deps.CustomTools = append(deps.CustomTools, o.spiritCustomTools(ag)...)
	deps.CustomTools = append(deps.CustomTools, o.skillsButlerTools(ctx, ag)...)
	root, err := chatagent.BuildTRPCAgentCached(ctx, ag, deps, o.lg)
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
	runnerMgr := o.td.CoalesceRunnerManager()
	runner, err := runnerMgr.NewTurnRunner(root, rt.TurnRunnerSpec{
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
	rollbackBoundary, rbErr := runnerMgr.MarkRollbackBoundary(ctx, sessionID, runID, "")
	if rbErr != nil {
		emitter.LogWarn("chat.runner.rollback_boundary", "Runner 回滚边界记录失败", "", event.P("error", rbErr.Error()))
	}
	rollbackDone := false
	rollbackRunnerSession := func() {
		if rollbackDone {
			return
		}
		rollbackDone = true
		if err := runnerMgr.RollbackToBoundary(context.Background(), rollbackBoundary); err != nil {
			emitter.LogWarn("chat.runner.rollback", "Runner 会话回滚失败", "", event.P("error", err.Error()))
		}
	}
	o.setRunStatus(ctx, sessionID, runID, "running", "")
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusRunning, "")
	emitter.LogStart("chat.turn.execute", "开始执行对话轮次", event.P("run_id", runID))
	defer func() {
		if turnStatus != "ok" {
			rollbackRunnerSession()
		}
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
	var intentRunOpts []trpcagent.RunOption
	if !biz.IsA2AProxyAgent(ag) {
		if intent.ShouldRun(ag, content) {
			emitter.LogStart("chat.intent.pass", "意图识别开始", event.P("provider", prov), event.P("model", mod), event.P("content_len", len(content)))
			intRes := intent.RunForAgent(ctx, ag, o.td.Catalog.LLM, o.td.LLMHTTP, prov, mod, content)
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
				intentRunOpts = append(intentRunOpts, intent.RunOptionInject(intRes.Artifact))
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
			emitter.LogSkip("chat.intent.pass", "Intent Pass 未启用或消息过短", event.P("intent_pass_enabled", intent.IntentPassFromAgent(ag)))
		}
	} else {
		emitter.LogSkip("chat.intent.pass", "A2A Proxy Agent 跳过意图识别", event.P("agent_kind", ag.Kind))
	}

	userOpts, err = mergeUserAttachmentRefs(userOpts, attachmentRefs)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return biz.ChatMessage{}, biz.ChatMessage{}, err
	}
	attN := len(attachmentRefs)

	now := chatagent.RFC3339Now()
	var userMsg biz.ChatMessage
	userMsgPersisted := false
	defer func() {
		if userMsgPersisted && turnStatus != "ok" {
			if err := o.td.Sessions.UpdateChatMessageStatus(ctx, sessionID, userMsg.ID, "failed", turnErrMsg); err != nil {
				event.CtxFlowLogWarn(ctx, "chat.user_msg_status_fail", "用户消息失败状态更新失败", event.P("message_id", userMsg.ID), event.P("error", err.Error()))
			}
		}
	}()
	if durableCtx.active {
		userMsg = durableCtx.buildUserMessage(sessionID, userOpts, attN, emitter)
		o.notifySessionRevisionSync(ctx, sessionID, runID, userMsg.ID)
	} else {
		userMsg = biz.ChatMessage{
			ID:               uuid.NewString(),
			SessionID:        sessionID,
			Role:             "user",
			ContentMarkdown:  content,
			Status:           "pending",
			OptionsJSON:      userOpts,
			CreatedAt:        now,
			AttachmentsCount: attN,
		}
		if err := o.td.Sessions.AppendChatMessage(ctx, sessionID, userMsg, false); err != nil {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
			o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
			return biz.ChatMessage{}, biz.ChatMessage{}, err
		}
		userMsgPersisted = true
		emitter.LogDone("chat.user_msg_persist", "用户消息已持久化")
		if !input.EntryConfig.AllowStream {
			o.bumpSessionRevisionSyncAndPublish(ctx, sessionID, runID, userMsg.ID)
		}
	}

	var sessionRunID string
	var stopBudget context.CancelFunc
	ctx, sessionRunID, stopBudget = o.durableSessionRunLifecycle(ctx, emitter, sess, ag, durableCtx, userMsg, content)
	defer stopBudget()
	defer func() {
		o.finishSessionRunLifecycle(ctx, sessionID, sessionRunID, turnErr)
	}()

	uid := chatagent.UserIDFromCtx(ctx)
	runOpts := durableResumeRunOpts(durableCtx.active, []trpcagent.RunOption{trpcagent.WithRequestID(sessionID), skillruntime.RunOptionWithTurnQuery(content)})
	if input.EntryConfig.AllowStream {
		runOpts = append(runOpts, trpcagent.WithStream(true))
	}
	runOpts = append(runOpts, intentRunOpts...)
	if ag.Settings != nil {
		if vars := chatagent.ParseVariablesJSON(ag.Settings.VariablesJSON, o.lg); vars != nil {
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
	if o.rt.KnowledgeRouter != nil {
		runCtx = knowledgetool.WithAdaptiveRouter(runCtx, o.rt.KnowledgeRouter)
	}
	if o.rt.KnowledgeFederatedRetriever != nil {
		runCtx = knowledgetool.WithFederatedRetriever(runCtx, o.rt.KnowledgeFederatedRetriever)
	}
	if o.rt.KnowledgeEvaluator != nil {
		runCtx = knowledgetool.WithRetrievalEvaluator(runCtx, o.rt.KnowledgeEvaluator)
	}
	if len(input.Options.KnowledgeBases) > 0 {
		runCtx = knowledgetool.WithKnowledgeCollections(runCtx, input.Options.KnowledgeBases)
	}
	var turnArtCollector *artifactbiz.TurnCollector
	runCtx, turnArtCollector = artifactbiz.WithTurnCollector(runCtx)

	safego.Go(runCtx, "llm-call-timeout-log", func() {
		select {
		case <-time.After(60 * time.Second):
			emitter.Log("chat.llm.invoke", event.FlowPhaseStart, "语言模型调用超过 60 秒仍在等待", event.P("run_id", runID))
		case <-runCtx.Done():
		}
	})
	emitter.LogStart("chat.llm.invoke", "正在调用语言模型")
	userTurnMsg, err := o.buildUserMessage(runCtx, sessionID, content, input.Options.AttachmentIDs)
	if err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		emitter.LogError("chat.llm.invoke", "附件装配失败", event.P("error", err.Error()))
		o.setRunStatus(ctx, sessionID, runID, "failed", err.Error())
		te := TurnError(TurnErrAttachmentFailed, err.Error())
		rollbackRunnerSession()
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
		o.setRunStatus(ctx, sessionID, runID, "failed", err.Error())
		te := TurnError(TurnErrLLMCallFailed, err.Error())
		rollbackRunnerSession()
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return userMsg, biz.ChatMessage{}, te
	}
	emitter.LogDone("chat.llm.invoke", "模型已返回，开始处理输出流")

	contextWin := o.resolveContextWindowTokens(ctx, sess, ag, prov, mod)
	projectMeta := chatagent.ProjectMeta{
		SessionID:        sessionID,
		RequestID:        sessionID,
		InvocationID:     runID,
		RunID:            runID,
		TraceID:          emitter.TraceID(),
		AgentID:          ag.ID,
		AgentDisplayName: ag.DisplayName,
		ContextWindow:    contextWin,
		Source:           event.EnvelopeSourceFromContext(ctx),
	}
	events = event.WrapFrameworkEventsWithOtel(events, emitter, traceBridge, traceBridge)
	streamOpts := NewChatStreamConsumeOptions(o.td.Catalog.ToolUC, o.td.Catalog.Agents, o.td.Sessions)
	result, streamErr := chatagent.ConsumeWithFirstByteGuard(runCtx, firstByteTimeout, events, o.td.Pipeline.Bus, projectMeta, streamOpts, o.lg)
	resultPromptTok = result.PromptTok
	resultCompletionTok = result.CompletionTok
	if streamErr != nil {
		if errors.Is(streamErr, chatagent.ErrFirstByteTimeout) {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, streamErr)
			emitter.LogCritical("chat.first_byte_timeout", "首字节超时，模型响应过慢", event.P("timeout", firstByteTimeout.String()))
			arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "first_byte_timeout").Observe(time.Since(turnStart).Seconds())
			o.setRunStatus(ctx, sessionID, runID, "failed", "first byte timeout")
			o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
			te := TurnError(TurnErrFirstByteTimeout, firstByteTimeout.String())
			o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
			return userMsg, biz.ChatMessage{}, te
		}
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, streamErr)
		o.setRunStatus(ctx, sessionID, runID, "failed", streamErr.Error())
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
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
	if ctx.Err() != nil && !result.HasContent {
		turnStatus = "timeout"
		turnErr = ctx.Err()
		turnErrMsg = "turn timeout"
		emitter.LogCritical("chat.turn.timeout", "对话请求超时", event.P("timeout", o.turnTimeout.String()), event.P("reason", "sync_cap"))
		arametrics.ChatTurnDuration.WithLabelValues(ag.ID, "timeout").Observe(time.Since(turnStart).Seconds())
		o.setRunStatus(ctx, sessionID, runID, "failed", "turn timeout")
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
		te := TurnError(TurnErrTurnTimeout, o.turnTimeout.String())
		o.publishTurnFailure(sessionID, runID, "chat-service", te, "")
		return userMsg, biz.ChatMessage{}, te
	}
	if ctx.Err() != nil && result.HasContent {
		turnStatus = "timeout_degraded"
		emitter.LogWarn("chat.turn.timeout_with_reply", "对话超时但模型已输出，保存回复", "", event.P("timeout", o.turnTimeout.String()), event.P("reply_len", result.Reply.Len()))
		bgCtx, bgCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer bgCancel()
		ctx = bgCtx
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
		o.setRunStatus(ctx, sessionID, runID, "failed", detail)
		o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
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
	if turnArtCollector != nil {
		if merged, merr := mergeTurnArtifactRefs(assistantOptsStr, turnArtCollector.Refs()); merr != nil {
			markTurnError(&turnStatus, &turnErr, &turnErrMsg, merr)
			o.publishTurnFailure(sessionID, runID, "chat-service", merr, "")
			return userMsg, biz.ChatMessage{}, merr
		} else {
			assistantOptsStr = merged
		}
	}

	assistantAttN := 0
	if turnArtCollector != nil {
		assistantAttN = len(turnArtCollector.Refs())
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
	}
	if err := o.td.Sessions.AppendChatMessage(ctx, sessionID, assistantMsg, true); err != nil {
		markTurnError(&turnStatus, &turnErr, &turnErrMsg, err)
		o.publishTurnFailure(sessionID, runID, "chat-service", err, "")
		return userMsg, biz.ChatMessage{}, err
	}
	if userMsgPersisted {
		if err := o.td.Sessions.UpdateChatMessageStatus(ctx, sessionID, userMsg.ID, "ok", ""); err != nil {
			event.CtxFlowLogWarn(ctx, "chat.user_msg_status_fail", "用户消息成功状态更新失败", event.P("message_id", userMsg.ID), event.P("error", err.Error()))
		} else {
			userMsg.Status = "ok"
		}
	}
	emitter.LogDone("chat.assistant_msg_persist", "助手消息已持久化", event.P("reply_len", len(displayMarkdown)))
	o.patchSessionContextUsage(ctx, sessionID, sess, ag, prov, mod, promptTok, completionTok)

	metricsLabel := "ok"
	if turnStatus == "timeout_degraded" {
		metricsLabel = "timeout_degraded"
	}
	arametrics.ChatTurnDuration.WithLabelValues(ag.ID, metricsLabel).Observe(time.Since(turnStart).Seconds())
	o.recordSessionTurn(ctx, sessionID, ag, userMsg.ID, assistantMsg.ID, prov, mod, promptTok, completionTok, assistantMsg.ContentMarkdown)
	o.setRunStatus(ctx, sessionID, runID, "completed", "")
	o.transitionSessionStatus(ctx, sessionID, sessstatus.SessionStatusCompleted, "")
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
		bgCtx, cancel := context.WithTimeout(o.svcCtx, o.turnTimeout)
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
			_, _, err = o.team.TeamsNative.RunTurnFromInput(bgCtx, sess, pendingInput)
			spiritSessionID := strings.TrimSpace(sess.ParentSessionID)
			teamID := strings.TrimSpace(sess.TeamID)
			if err != nil {
				o.publishTurnFailure(sessionID, "", "pending-queue", err, pendingEntryID)
				o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusInterrupted, sessstatus.StatusReasonError)
				if spiritSessionID != "" && teamID != "" {
					o.teamStarter.HandleTeamTurnResult(bgCtx, spiritSessionID, teamID, "failed", err.Error())
				}
			} else {
				o.transitionSessionStatus(bgCtx, sessionID, sessstatus.SessionStatusCompleted, "")
				if spiritSessionID != "" && teamID != "" {
					o.teamStarter.HandleTeamTurnResult(bgCtx, spiritSessionID, teamID, "completed", "")
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

// recordSessionTurn records a completed agent turn.
func (o *ChatOrchestrator) recordSessionTurn(ctx context.Context, sessionID string, ag biz.Agent, userMsgID, assistantMsgID, prov, mod string, promptTok, completionTok int, contentPreview string) {
	if o == nil || o.td.Sessions == nil {
		return
	}
	now := time.Now().UTC().Format(time.RFC3339)
	preview := strutil.ProtoPreview(contentPreview, 200)
	if turnID := admittedTurnIDFromContext(ctx); turnID != "" {
		_, err := o.td.Sessions.UpdateTurn(ctx, turnID, biz.SessionTurnUpdateFields{
			Status:              ptrString("completed"),
			EndedAt:             ptrString(now),
			UserMessageID:       ptrString(userMsgID),
			AssistantMessageID:  ptrString(assistantMsgID),
			OwnerType:           ptrString("agent"),
			AgentID:             ptrString(ag.ID),
			InputTokens:         ptrInt(promptTok),
			OutputTokens:        ptrInt(completionTok),
			TotalTokens:         ptrInt(promptTok + completionTok),
			ModelCallCount:      ptrInt(1),
			FinalProvider:       ptrString(prov),
			FinalModel:          ptrString(mod),
			FinalContentPreview: ptrString(preview),
		})
		if err != nil {
			event.CtxFlowLogWarn(ctx, "chat.usage_record_fail", "会话轮次更新失败", event.P("session_id", sessionID), event.P("turn_id", turnID), event.P("error", err.Error()))
		}
		return
	}
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
	if turnID := admittedTurnIDFromContext(ctx); turnID != "" {
		_, err := o.td.Sessions.UpdateTurn(ctx, turnID, biz.SessionTurnUpdateFields{
			Status:              ptrString("completed"),
			EndedAt:             ptrString(now),
			UserMessageID:       ptrString(userMsgID),
			AssistantMessageID:  ptrString(assistantMsgID),
			OwnerType:           ptrString("team"),
			TeamID:              ptrString(teamID),
			InputTokens:         ptrInt(promptTok),
			OutputTokens:        ptrInt(completionTok),
			TotalTokens:         ptrInt(promptTok + completionTok),
			ModelCallCount:      ptrInt(1),
			FinalProvider:       ptrString(prov),
			FinalModel:          ptrString(mod),
			FinalContentPreview: ptrString(preview),
		})
		if err != nil {
			event.CtxFlowLogWarn(ctx, "chat.usage_record_fail", "团队会话轮次更新失败", event.P("session_id", sessionID), event.P("turn_id", turnID), event.P("error", err.Error()))
		}
		return
	}
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
	if o.td.Sessions != nil && strings.TrimSpace(sessionID) != "" {
		o.td.Sessions.AccumulateMetricsDelta(sessstatus.SessionMetricsDelta{
			SessionID:        sessionID,
			ModelCallCount:   ev.CallCount,
			InputTokens:      int64(ev.InputTokens),
			OutputTokens:     int64(ev.OutputTokens),
			TotalTokens:      int64(ev.TotalTokens),
			TotalCostMicroUsd: ev.TotalCostMicroUSD,
		})
	}
	biz.PublishTokenUsageEnvelope(ctx, o.td.Pipeline.Bus, ev)
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
func (o *ChatOrchestrator) patchSessionContextUsage(ctx context.Context, sessionID string, sess biz.Session, ag biz.Agent, prov, mod string, promptTok, completionTok int) {
	sessctx.PatchContextFromLLMUsage(ctx, o.td.Sessions, o.td.Compress, o.llmContextCatalog(), sessionID, sess, ag, prov, mod, promptTok, completionTok, o.lg)
}

// notifyNativeTurnHooks runs post-turn side effects.
func (o *ChatOrchestrator) notifyNativeTurnHooks(ctx context.Context, sessionID string, ag biz.Agent, userInput, assistantOutput string) {
	if o == nil || o.td.AfterTurn == nil {
		return
	}
	o.td.AfterTurn.AfterNativeTurn(ctx, biz.NativeTurnEvent{
		AgentID:         ag.ID,
		AgentConfigJSON: ag.ConfigJSON,
		AgentSettings:   ag.Settings,
		SessionID:       sessionID,
		UserInput:       userInput,
		AssistantOutput: assistantOutput,
	})
}

// nativeSendChatMessage is the native implementation of SendChatMessage.
func (o *ChatOrchestrator) nativeSendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	tr, err := o.Execute(ctx, turnInputFromProto(req))
	if err != nil {
		if IsTurnMessageQueued(err) {
			return &chatv1.SendChatMessageResponse{}, nil
		}
		return nil, err
	}
	if tr.Outcome != biz.TurnOutcomeCompleted {
		return &chatv1.SendChatMessageResponse{}, nil
	}
	userMsg, assistantMsg := tr.UserMsg, tr.AssistantMsg
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

// turnInputFromProto converts a proto SendChatMessageRequest to a biz-level TurnInput.
// This adapter lives in the service layer (the proto boundary) so that internal
// packages never need to import api/*/v1.
func turnInputFromProto(req *chatv1.SendChatMessageRequest) biz.TurnInput {
	if req == nil {
		return biz.TurnInput{}
	}
	input := biz.TurnInput{
		SessionID: req.GetSessionId(),
		Content:   req.GetContent(),
		AgentKey:  req.GetAgentKey(),
		EntryConfig: biz.TurnEntryPointConfig{
			EntryPoint:  biz.EntryPointWeb,
			AllowQueue:  true,
			AllowStream: true,
		},
	}
	if req.TeamId != nil {
		input.TeamID = *req.TeamId
	}
	if opts := req.GetOptions(); opts != nil {
		input.Options = biz.TurnOptions{
			DialogMode:     opts.GetDialogMode(),
			Provider:       opts.GetProvider(),
			Model:          opts.GetModel(),
			KnowledgeBases: opts.GetKnowledgeBases(),
		}
		for _, att := range opts.GetAttachments() {
			if att != nil {
				input.Options.AttachmentIDs = append(input.Options.AttachmentIDs, att.GetId())
			}
		}
	}
	return input
}
