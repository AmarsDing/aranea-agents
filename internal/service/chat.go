package service

import (
	"context"
	"errors"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/outbound"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
)

// ChatService is the thin transport bridge between proto/HTTP/WS and the
// ChatOrchestrator. It only handles request validation, proto mapping, and
// delegates all orchestration work to ChatOrchestrator.
type ChatService struct {
	chatv1.UnimplementedChatServiceServer

	orch         *ChatOrchestrator
	turnPipeline *TurnPipeline
	// sessions is the concrete *biz.SessionUsecase wired via TurnDeps.Sessions
	// (typed as biz.SessionTurnManager interface). Used by RetrySession to look
	// up the last user message before re-enqueuing. Nil when the wired
	// Sessions implementation is not *biz.SessionUsecase (test stubs).
	sessions *biz.SessionUsecase
	lg       loggateway.Logger
}

func NewChatService(deps ChatOrchestratorDeps) *ChatService {
	orch := NewChatOrchestrator(deps)
	svc := &ChatService{orch: orch, lg: deps.Infra.LG}
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
	if deps.Turn.RT.OutboundRouter != nil && deps.Turn.Sessions != nil {
		outbound.RegisterSessionResolver(func(sessionID string) (outbound.DeliveryTarget, bool) {
			ctx := context.Background()
			session, err := deps.Turn.Sessions.Get(ctx, sessionID)
			if err != nil {
				return outbound.DeliveryTarget{}, false
			}
			meta, ok := biz.ParseChannelSessionMeta(session.MetadataJSON)
			if !ok {
				return outbound.DeliveryTarget{}, false
			}
			return outbound.DeliveryTarget{
				Channel: meta.ChannelID,
				Target:  meta.PeerID,
			}, true
		})
	}
	// Start SubAgentService lifecycle.
	if deps.Turn.RT.SubAgentService != nil {
		deps.Turn.RT.SubAgentService.Start(context.Background())
	}
	return svc
}

// Close gracefully shuts down the ChatService, including SubAgentService and
// orchestrator lifecycle resources.
func (s *ChatService) Close() error {
	var firstErr error
	if s.orch != nil {
		if s.orch.rt().SubAgentService != nil {
			if err := s.orch.rt().SubAgentService.Close(); err != nil && firstErr == nil {
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
// (BAD_REQUEST / INTERNAL / NOT_FOUND / CONFLICT) on validation or lookup
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
		return nil, apierror.Conflict(apierror.DomainChat, "retry rejected: %s", rejectReason)
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
	accepted, _, _, _, err := s.orch.EnqueueUserMessage(sessionID, content)
	return accepted, err
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
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, apierror.BadRequest(apierror.DomainChat, "content is required")
	}

	accepted, queued, pendingID, _, err := s.orch.EnqueueUserMessage(sessionID, content)
	if err != nil {
		return nil, err
	}
	if !accepted {
		return &chatv1.EnqueueUserMessageResponse{Accepted: false}, nil
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
		runID = *req.RunId
	}
	if s.orch.TrySendAwaitChannel(sessionID, biz.AwaitReplyMsg{RunID: runID, Reply: reply}) {
		return &chatv1.AwaitUserReplyResponse{Accepted: true}, nil
	}
	// Send failed: channel may be full (normal) or closed by GC (race).
	// If the entry no longer exists in the map, it was cleaned up — try resume.
	if _, stillExists := s.orch.LoadAwaitChannel(sessionID); !stillExists {
		runID2, canResume := s.orch.canResumeAwait(ctx, sessionID)
		if canResume {
			if req.RunId != nil && strings.TrimSpace(*req.RunId) != "" {
				runID2 = strings.TrimSpace(*req.RunId)
			}
			if err := s.orch.resumeAwaitAfterRestart(ctx, sessionID, reply, runID2); err != nil {
				if errors.Is(err, errResumeInFlight) {
					return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
				}
				return nil, err
			}
			return &chatv1.AwaitUserReplyResponse{Accepted: true}, nil
		}
	}
	return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
}
