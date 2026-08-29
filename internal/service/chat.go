package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/decision"
	"aranea-agents/internal/event"
	"aranea-agents/internal/outbound"
	rt "aranea-agents/internal/runtime"
	subagenttool "aranea-agents/internal/tools/subagent"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/ctxuser"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// ChatService is the thin transport bridge between proto/HTTP/WS and the
// ChatOrchestrator. It only handles request validation, proto mapping, and
// delegates all orchestration work to ChatOrchestrator.
type ChatService struct {
	chatv1.UnimplementedChatServiceServer

	orch           *ChatOrchestrator
	turnPipeline   *TurnPipeline
	sessions       *biz.SessionUsecase
	memberSessions biz.MemberSessionV2Repo
	// taskV2/stepReader back ResumeInterruptedTask (L3): CAS claim +
	// execution-trace assembly. Nil in partial test stubs → Internal error.
	taskV2     biz.TaskV2Repo
	stepReader biz.StepV2Reader
	lg         loggateway.Logger
	// planExec 由 ProvideChatService 后注入；Close 时 Stop 订阅 goroutine。
	planExec *PlanExecutor
	// bridge 是 M76 审批中继（ConfirmActivity 路由 source=external_coding）。
	bridge *AgentBridgeService
}

func NewChatService(deps ChatOrchestratorDeps) *ChatService {
	orch := NewChatOrchestrator(deps)
	svc := &ChatService{
		orch:           orch,
		lg:             deps.Infra.LG,
		memberSessions: deps.Infra.MemberSessions,
		taskV2:         deps.Turn.TaskV2,
		stepReader:     deps.Turn.StepReader,
	}
	// Extract the concrete *biz.SessionUsecase for RetrySession. In production
	// this is always satisfied (Wire binds *biz.SessionUsecase). Test stubs
	// may use other implementations; in that case sessions stays nil and
	// RetrySession degrades to an Internal error.
	if su, ok := deps.Turn.Sessions.(*biz.SessionUsecase); ok {
		svc.sessions = su
	}
	if deps.Turn.Sessions != nil {
		svc.turnPipeline = &TurnPipeline{
			Service:  NewPersistentTurnService(deps.Turn.Sessions),
			Executor: chatTurnExecutor{orch: orch},
			Lg:       deps.Infra.LG,
		}
	}
	// Register session resolver for outbound target resolution.
	// When an agent calls the message tool without explicit channel/target,
	// the resolver looks up the session's channel metadata to infer the target.
	if deps.Turn.RT.Extensions.OutboundRouter != nil && deps.Turn.Sessions != nil {
		sessions := deps.Turn.Sessions
		outbound.RegisterSessionResolver(outbound.NewLoggingSessionResolver(deps.Infra.LG, func(sessionID string) (outbound.DeliveryTarget, error) {
			session, err := sessions.Get(context.Background(), sessionID)
			if err != nil {
				return outbound.DeliveryTarget{}, err
			}
			meta, ok := biz.ParseChannelSessionMeta(session.MetadataJSON)
			if !ok {
				// Not a channel session (e.g. Web Chat). Resolver-chain miss, not an error.
				return outbound.DeliveryTarget{}, nil
			}
			return outbound.DeliveryTarget{
				Channel: meta.ChannelID,
				Target:  meta.PeerID,
			}, nil
		}))
	}
	// Start SubAgentService lifecycle + Mode B agent-card projection.
	if deps.Turn.RT.Extensions.SubAgentService != nil {
		deps.Turn.RT.Extensions.SubAgentService.SetModeBStartedHook(svc.publishModeBMemberSession)
		deps.Turn.RT.Extensions.SubAgentService.SetModeBFinishedHook(svc.finishModeBMemberSession)
		// 包C C4-②：subagent token 预算跳闸的决策记录双写（对齐 team
		// tripRunTokenBudget 的 EmitGate 坐标）。闸本身的取消/拒绝在
		// subagent.Service 内完成，此处仅审计落账；collector nil 时
		// EmitGate 静默跳过。
		deps.Turn.RT.Extensions.SubAgentService.SetBudgetTripHook(func(ctx context.Context, info subagenttool.BudgetTripInfo) {
			action := "cancel_run"
			scenario := "subagent run 累计 input token 超预算"
			if info.Scope == subagenttool.BudgetScopeParentAggregate {
				action = "reject_spawn"
				scenario = "父会话 subagent spawn 合计 input token 超预算"
			}
			event.EmitGate(ctx, deps.Infra.DecisionCollector, decision.GateDecision{
				TriggerRule:   decision.TriggerTokenBudgetTripped,
				Outcome:       "tripped",
				Scenario:      scenario,
				Reasoning:     fmt.Sprintf("subagent input token 用量 %d 超预算上限 %d（scope=%s）", info.UsedInputTokens, info.BudgetInputTokens, info.Scope),
				GuardName:     "token_budget",
				RunID:         info.RunID,
				SessionID:     info.ParentSessionID,
				Entities:      []decision.EntityRef{{Type: "session", Key: info.ParentSessionID}},
				ObservedValue: info.UsedInputTokens,
				Threshold:     info.BudgetInputTokens,
				Action:        action,
			})
		})
		deps.Turn.RT.Extensions.SubAgentService.Start(context.Background())
	}
	return svc
}

// BindAgentBridge 把编程桥审批中继接到 ConfirmActivity（newApp BeforeStart）。
func (s *ChatService) BindAgentBridge(bridge *AgentBridgeService) {
	if s == nil || bridge == nil {
		return
	}
	s.bridge = bridge
	if s.orch != nil {
		bridge.SetConfirmSink(s.orch.stepWriter())
	}
}

// publishModeBMemberSession projects a background subagent start as an orphan
// MemberSession (empty TeamRunID/TeamStageID) for Mode B agent-card UI.
func (s *ChatService) publishModeBMemberSession(ctx context.Context, info subagenttool.ModeBStartedInfo) {
	if s == nil || s.orch == nil || s.orch.v2Seq == nil {
		return
	}
	parent := strings.TrimSpace(info.ParentSessionID)
	child := strings.TrimSpace(info.ChildSessionID)
	if parent == "" || child == "" {
		return
	}
	spiritID := s.resolveSpiritSessionID(ctx, parent)
	now := time.Now().UTC()
	agentKey := "subagent:" + strings.TrimSpace(info.RunID)
	if agentKey == "subagent:" {
		agentKey = "subagent:" + child
	}
	msID := uuid.NewSHA1(uuid.NameSpaceDNS, []byte("aranea.member_session.v2:modeb:"+child)).String()
	name := strings.TrimSpace(info.Task)
	if name == "" {
		name = "Sub-agent"
	} else if len([]rune(name)) > 48 {
		name = string([]rune(name)[:48]) + "…"
	}
	ms := biz.MemberSession{
		ID:              msID,
		TeamRunID:       "",
		TeamStageID:     "",
		TaskID:          "", // frontend hosts on latest/running spirit task
		SessionID:       child,
		SpiritSessionID: spiritID,
		AgentKey:        agentKey,
		AgentName:       name,
		Status:          biz.MemberSessionStatusRunning,
		StartedAt:       now,
		Version:         biz.MemberSessionVersionCreated,
	}
	s.orch.v2Seq.Publish(ctx, biz.NewMemberSessionCreatedEvent(ms))
}

func (s *ChatService) finishModeBMemberSession(ctx context.Context, info subagenttool.ModeBFinishedInfo) {
	if s == nil {
		return
	}
	child := strings.TrimSpace(info.ChildSessionID)
	if child == "" {
		return
	}
	status := biz.MemberSessionStatusCompleted
	switch strings.TrimSpace(info.Status) {
	case "failed":
		status = biz.MemberSessionStatusFailed
	case "cancelled":
		status = biz.MemberSessionStatusSkipped
	}
	s.syncMemberSessionStatus(ctx, child, status)
}

// assertSessionAccess verifies the authenticated caller owns sessionID (E2E-P0-01).
// System callers (default_user) and sessions with empty UserID are allowed so
// cron/channel/durable entry points keep working. Cross-user access is Forbidden.
func (s *ChatService) assertSessionAccess(ctx context.Context, sessionID string) error {
	authUserID := ctxuser.FromContext(ctx)
	if authUserID == ctxuser.DefaultUserID {
		return nil
	}
	if s == nil || s.sessions == nil {
		return nil
	}
	sess, err := s.sessions.Get(ctx, sessionID)
	if err != nil {
		if apierror.IsCode(err, apierror.CodeNotFound) {
			return apierror.NotFound(apierror.DomainSession, "session not found")
		}
		return err
	}
	if sess.UserID != "" && authUserID != sess.UserID {
		return apierror.Forbidden(apierror.DomainChat, "session does not belong to the authenticated user")
	}
	return nil
}

// Close gracefully shuts down the ChatService, including SubAgentService and
// orchestrator lifecycle resources.
func (s *ChatService) Close() error {
	var firstErr error
	if s.planExec != nil {
		s.planExec.Stop()
	}
	if s.orch != nil {
		if s.orch.rt().Extensions.SubAgentService != nil {
			if err := s.orch.rt().Extensions.SubAgentService.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		s.orch.Close()
	}
	return firstErr
}

// ProvideChatOrchestrator extracts the ChatOrchestrator from ChatService for
// Wire binding to biz.TurnExecutor.
// TaskOrchestrator backfill is now handled by provideChatServiceDeps in wire.go,
// which injects it directly into TeamOrchestrationDeps without requiring this
// provider to be called.
func ProvideChatOrchestrator(svc *ChatService) *ChatOrchestrator {
	return svc.orch
}

func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	return s.orch.nativeSendChatMessage(ctx, req)
}

// SubmitChatMessage submits a user message asynchronously and returns an ACK
// only (B2 channel separation). Turn execution runs in a background goroutine
// using the process-lifecycle context; all message/state/streaming data is
// delivered via the WS data channel.
//
// This is the additive, non-breaking companion to SendChatMessage. The legacy
// synchronous RPC remains available for clients that need the full response
// inline; WS-connected clients should prefer SubmitChatMessage.
func (s *ChatService) SubmitChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SubmitChatMessageResponse, error) {
	return s.orch.submitChatMessageAsync(ctx, req)
}

func (s *ChatService) GetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
	return s.orch.nativeGetChatOptions(ctx, req)
}

func (s *ChatService) StopGeneration(ctx context.Context, req *chatv1.StopGenerationRequest) (*chatv1.StopGenerationResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	stopped := s.orch.CancelRun(ctx, sessionID)
	return &chatv1.StopGenerationResponse{Stopped: stopped}, nil
}

// retryRecentMessagesLimit bounds the look-back window when searching for the
// latest user message in a session. User messages interleave with assistant
// replies, so a small window suffices; 50 is a safety bound against runaway
// scans on long sessions.
const retryRecentMessagesLimit = 50

// RetrySession re-triggers the last failed/interrupted turn for a session by
// finding the most recent user message and re-enqueuing it. Used by AgentCard
// retry button to recover a stuck/failed sub-agent run.
//
// Lookup flow: ListMessagesRecent → reverse-iterate to last Role=="user" →
// EnqueueUserMessage. Returns retried=true on success; returns an error
// (BAD_REQUEST / INTERNAL / NOT_FOUND / RATE_LIMITED / CONFLICT) on validation or lookup
// failures.
func (s *ChatService) RetrySession(ctx context.Context, req *chatv1.RetrySessionRequest) (*chatv1.RetrySessionResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if s.sessions == nil {
		return nil, apierror.Internal(apierror.DomainChat, "session usecase unavailable for retry")
	}
	msgs, err := s.sessions.ListMessagesRecent(ctx, sessionID, retryRecentMessagesLimit)
	if err != nil {
		return nil, apierror.Internal(apierror.DomainChat, "load recent messages failed").WithCause(err)
	}
	var lastUserContent string
	for i := len(msgs) - 1; i >= 0; i-- {
		if msgs[i].Role == "user" {
			lastUserContent = msgs[i].ContentMarkdown
			break
		}
	}
	if strings.TrimSpace(lastUserContent) == "" {
		return nil, apierror.NotFound(apierror.DomainChat, "no user message to retry in session %s", sessionID)
	}
	accepted, _, _, rejectReason, err := s.orch.EnqueueUserMessage(sessionID, lastUserContent)
	if err != nil {
		// EnqueueUserMessage returns framework-level errors; wrap as Internal
		// to keep apierror consistency (BL2). The original cause is preserved.
		return nil, apierror.Internal(apierror.DomainChat, "enqueue retry failed").WithCause(err)
	}
	if !accepted {
		return nil, enqueueRejectError(rejectReason)
	}
	return &chatv1.RetrySessionResponse{Retried: true}, nil
}

// CancelRun stops the active run for a session (WS cancel and HTTP stop share this path).
func (s *ChatService) CancelRun(ctx context.Context, sessionID string) bool {
	return s.orch.CancelRun(ctx, sessionID)
}

// setRunStatus atomically updates the run status for a session and publishes a WS envelope.
func (s *ChatService) setRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	s.orch.setRunStatus(ctx, sessionID, runID, status, errMsg)
}

// SetRunStatus implements biz.ChannelTurnGateway.
func (s *ChatService) SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	s.orch.setRunStatus(ctx, sessionID, runID, status, errMsg)
}

// TryEnqueueUserMessage implements biz.ChannelTurnGateway — delegates to ChatOrchestrator.
func (s *ChatService) TryEnqueueUserMessage(sessionID, content string) (bool, error) {
	if s == nil || s.orch == nil {
		return false, nil
	}
	accepted, _, _, rejectReason, err := s.orch.EnqueueUserMessage(sessionID, content)
	if err != nil {
		return false, err
	}
	if !accepted {
		return false, enqueueRejectError(rejectReason)
	}
	return true, nil
}

// SetSessionPendingMergeFollowup implements biz.ChannelTurnGateway — delegates to ChatOrchestrator.
func (s *ChatService) SetSessionPendingMergeFollowup(sessionID string, merge bool) {
	if s == nil || s.orch == nil {
		return
	}
	s.orch.SetSessionPendingMergeFollowup(sessionID, merge)
}

// InterruptAndSend implements biz.PendingQueueGateway — delegates to ChatOrchestrator.
func (s *ChatService) InterruptAndSend(ctx context.Context, sessionID, pendingEntryID string) error {
	if s == nil || s.orch == nil {
		return nil
	}
	return s.orch.InterruptAndSendMessage(ctx, sessionID, pendingEntryID)
}

// RunGateway exposes the shared session run registry (Chat, Team, Cron, Channel, WS).
func (s *ChatService) RunGateway() *rt.RunRegistry {
	return s.orch.runs
}

// HasActiveRun reports whether a session has an in-flight run on the shared gateway.
func (s *ChatService) HasActiveRun(sessionID string) bool {
	return s.orch.HasActiveRun(sessionID)
}

// ActiveSessionRunPhase returns the phase of the active session run, if any (CC-FIX-CHANNEL UX).
// Legacy DB rows with phase='escalating' are mapped to 'durable' via ParseSessionRunPhase.
func (s *ChatService) ActiveSessionRunPhase(ctx context.Context, sessionID string) string {
	if s == nil || s.orch == nil || s.orch.chJobs().SessionRuns == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	run, err := s.orch.chJobs().SessionRuns.GetActiveForSession(ctx, sessionID)
	if err != nil || run.ID == "" {
		return ""
	}
	return string(biz.ParseSessionRunPhase(run.Phase))
}

// LastPendingMessageID returns the most recently enqueued pending message id for a session, if any.
func (s *ChatService) LastPendingMessageID(sessionID string) string {
	return s.orch.LastPendingMessageID(sessionID)
}

func (s *ChatService) GetPendingMessages(ctx context.Context, req *chatv1.GetPendingMessagesRequest) (*chatv1.GetPendingMessagesResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	entries := s.orch.GetPendingMessages(sessionID)
	items := make([]*chatv1.PendingMessage, 0, len(entries))
	for i := range entries {
		items = append(items, &chatv1.PendingMessage{
			Id:        entries[i].ID,
			Content:   entries[i].Content,
			Status:    entries[i].Status,
			CreatedAt: entries[i].CreatedAt,
		})
	}
	return &chatv1.GetPendingMessagesResponse{Items: items}, nil
}

func (s *ChatService) CancelPendingMessage(ctx context.Context, req *chatv1.CancelPendingMessageRequest) (*chatv1.CancelPendingMessageResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	pendingID := strings.TrimSpace(req.GetPendingId())
	if pendingID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "pending_id is required")
	}
	cancelled := s.orch.CancelPendingMessage(sessionID, pendingID)
	return &chatv1.CancelPendingMessageResponse{Cancelled: cancelled}, nil
}

func (s *ChatService) UpdatePendingMessage(ctx context.Context, req *chatv1.UpdatePendingMessageRequest) (*chatv1.UpdatePendingMessageResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	pendingID := strings.TrimSpace(req.GetPendingId())
	if pendingID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "pending_id is required")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "content is required")
	}
	updated := s.orch.UpdatePendingMessage(sessionID, pendingID, content)
	return &chatv1.UpdatePendingMessageResponse{Updated: updated}, nil
}

func (s *ChatService) InterruptAndSendMessage(ctx context.Context, req *chatv1.InterruptAndSendMessageRequest) (*chatv1.InterruptAndSendMessageResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if err := s.assertSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	pendingID := strings.TrimSpace(req.GetPendingId())
	if pendingID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "pending_id is required")
	}
	if err := s.orch.InterruptAndSendMessage(ctx, sessionID, pendingID); err != nil {
		return nil, err
	}
	return &chatv1.InterruptAndSendMessageResponse{Sent: true}, nil
}

func (s *ChatService) EnqueueUserMessage(ctx context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if err := s.assertSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "content is required")
	}
	// P2-3 三级注入语义：""/"steer"=插话（下一 step 边界），"followup"=显式
	// 追问（排队新 turn），"inject"=静默上下文（不唤醒）。非法 kind 在边界拒掉。
	kind := strings.ToLower(strings.TrimSpace(req.GetKind()))
	switch kind {
	case "", biz.ChatEnqueueKindSteer, biz.ChatEnqueueKindFollowup, biz.ChatEnqueueKindInject:
	default:
		return nil, apierror.BadRequest(apierror.DomainChat, "invalid kind: must be steer|followup|inject")
	}

	accepted, queued, pendingID, rejectReason, err := s.orch.EnqueueUserMessageWithKind(sessionID, content, kind)
	if err != nil {
		return nil, err
	}
	if !accepted {
		return nil, enqueueRejectError(rejectReason)
	}
	if queued {
		if pendingID == "" {
			return nil, apierror.Internal(apierror.DomainChat, "queued message missing pending id")
		}
		return &chatv1.EnqueueUserMessageResponse{
			Accepted:  true,
			Queued:    true,
			PendingId: pendingID,
		}, nil
	}
	return &chatv1.EnqueueUserMessageResponse{Accepted: true}, nil
}

// GetRunStatus returns the current run lifecycle state for a session.
func (s *ChatService) GetRunStatus(ctx context.Context, req *chatv1.GetRunStatusRequest) (*chatv1.RunStatus, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	if err := s.assertSessionAccess(ctx, sessionID); err != nil {
		return nil, err
	}
	resp := &chatv1.RunStatus{Status: "idle"}
	if runID, status, errMsg, updatedAt, ok := s.orch.GetRunStatus(ctx, sessionID); ok {
		resp.RunId = runID
		resp.Status = status
		resp.ErrorMessage = errMsg
		resp.UpdatedAt = updatedAt
	} else if snap, ok := s.orch.hydrateRunStatusFromSession(ctx, sessionID); ok {
		resp.RunId = snap.RunID
		resp.Status = snap.Status
		resp.ErrorMessage = snap.ErrorMessage
		resp.UpdatedAt = snap.UpdatedAt
		// Use await fields from hydrated snapshot when available.
		if strings.TrimSpace(snap.Status) == "awaiting_user" {
			resp.AwaitKind = snap.AwaitKind
			resp.AwaitToolKey = snap.AwaitToolKey
			resp.AwaitToolCallId = snap.AwaitToolCallID
		}
	}
	if runnerIface, _, active := s.orch.ActiveRunner(sessionID); active {
		if runner, ok := runnerIface.(trpcrunner.Runner); ok {
			applyFrameworkRunStatus(resp, runner, sessionID)
		}
	}
	if meta := s.orch.resolveAwaitMeta(ctx, sessionID, resp.Status); strings.TrimSpace(resp.Status) == "awaiting_user" {
		resp.AwaitKind = meta.Kind
		resp.AwaitToolKey = meta.ToolKey
		resp.AwaitToolCallId = meta.ToolCallID
	}
	return resp, nil
}

func applyFrameworkRunStatus(resp *chatv1.RunStatus, runner trpcrunner.Runner, requestID string) {
	if resp == nil || runner == nil {
		return
	}
	st, ok := rt.FrameworkRunStatusFromRunner(runner, requestID)
	if !ok {
		return
	}
	resp.InvocationId = st.InvocationID
	resp.AgentName = st.AgentName
	if !st.StartedAt.IsZero() {
		resp.StartedAt = st.StartedAt.Format(time.RFC3339)
	}
	if !st.LastEventAt.IsZero() {
		resp.LastEventAt = st.LastEventAt.Format(time.RFC3339)
	}
	resp.EventCount = int32(st.EventCount)
}

// AwaitUserReply submits a human reply for a run that is paused in awaiting_user state.
func (s *ChatService) AwaitUserReply(ctx context.Context, req *chatv1.AwaitUserReplyRequest) (*chatv1.AwaitUserReplyResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "session_id is required")
	}
	reply := strings.TrimSpace(req.GetReply())
	if reply == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "reply is required")
	}
	runID := ""
	if req.RunId != nil {
		runID = strings.TrimSpace(*req.RunId)
	}
	// Unified delivery: fast path via in-memory channel, restart recovery via
	// resumeAwaitAfterRestart. The user's own words serve as resume content.
	outcome, err := s.orch.submitAwaitReply(ctx, sessionID, awaitReply{
		runID:         runID,
		token:         reply,
		resumeContent: reply,
	})
	if err != nil {
		return nil, err
	}
	return &chatv1.AwaitUserReplyResponse{Accepted: outcome != awaitReplyRejected}, nil
}
