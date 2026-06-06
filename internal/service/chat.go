package service

import (
	"context"
	"errors"
	"strings"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/outbound"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/tools/mcpobserve"
	"aranea-agents/pkg/loggateway"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// ChatService is the thin transport bridge between proto/HTTP/WS and the
// ChatOrchestrator. It only handles request validation, proto mapping, and
// delegates all orchestration work to ChatOrchestrator.
type ChatService struct {
	chatv1.UnimplementedChatServiceServer

	orch         *ChatOrchestrator
	turnPipeline *TurnPipeline
	lg           loggateway.Logger
}

func NewChatService(deps ChatOrchestratorDeps) *ChatService {
	orch := NewChatOrchestrator(deps)
	svc := &ChatService{orch: orch, lg: deps.LG}
	if deps.Sessions != nil {
		svc.turnPipeline = &TurnPipeline{
			Service:  NewPersistentTurnService(deps.Sessions),
			Executor: chatTurnExecutor{orch: orch},
			Lg:       deps.LG,
		}
	}
	// Register session resolver for outbound target resolution.
	// When an agent calls the message tool without explicit channel/target,
	// the resolver looks up the session's channel metadata to infer the target.
	if deps.RT.OutboundRouter != nil && deps.Sessions != nil {
		outbound.RegisterSessionResolver(func(sessionID string) (outbound.DeliveryTarget, bool) {
			ctx := context.Background()
			session, err := deps.Sessions.Get(ctx, sessionID)
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
	if deps.RT.SubAgentService != nil {
		deps.RT.SubAgentService.Start(context.Background())
	}
	return svc
}

// Close gracefully shuts down the ChatService, including SubAgentService and
// orchestrator lifecycle resources.
func (s *ChatService) Close() error {
	var firstErr error
	if s.orch != nil {
		if s.orch.rt.SubAgentService != nil {
			if err := s.orch.rt.SubAgentService.Close(); err != nil && firstErr == nil {
				firstErr = err
			}
		}
		s.orch.Close()
	}
	return firstErr
}

// ProvideChatOrchestrator extracts the ChatOrchestrator from ChatService for
// Wire binding to biz.TurnExecutor.
func ProvideChatOrchestrator(svc *ChatService) *ChatOrchestrator {
	return svc.orch
}

func configureMCPObserve(bus event.Bus, mcp *biz.MCPServerUsecase) {
	if bus != nil {
		mcpobserve.SetBus(bus)
	}
	if mcp == nil {
		return
	}
	lg := loggateway.NewNoop()
	mcpobserve.SetMetadataRecorder(func(ctx context.Context, serverKey string, at time.Time) {
		if err := mcp.RecordReconnectMetadata(ctx, serverKey, at); err != nil {
			lg.Warn("MCP reconnect metadata record failed",
				loggateway.StepID("chat.mcp.reconnect_meta"),
				loggateway.Str("server_key", serverKey),
				loggateway.Err(err),
			)
		}
	})
}

func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	return s.orch.nativeSendChatMessage(ctx, req)
}

func (s *ChatService) GetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
	return s.orch.nativeGetChatOptions(ctx, req)
}

func (s *ChatService) StopGeneration(ctx context.Context, req *chatv1.StopGenerationRequest) (*chatv1.StopGenerationResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	stopped := s.orch.CancelRun(ctx, sessionID)
	return &chatv1.StopGenerationResponse{Stopped: stopped}, nil
}

// CancelRun stops the active run for a session (WS cancel and HTTP stop share this path).
func (s *ChatService) CancelRun(ctx context.Context, sessionID string) bool {
	return s.orch.CancelRun(ctx, sessionID)
}

// setRunStatus atomically updates the run status for a session and publishes a WS envelope.
func (s *ChatService) setRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	s.orch.setRunStatus(ctx, sessionID, runID, status, errMsg)
}

// SetRunStatus implements biz.NativeTurnGateway.
func (s *ChatService) SetRunStatus(ctx context.Context, sessionID, runID, status, errMsg string) {
	s.orch.setRunStatus(ctx, sessionID, runID, status, errMsg)
}

// TryEnqueueUserMessage implements biz.NativeTurnGateway — delegates to ChatOrchestrator.
func (s *ChatService) TryEnqueueUserMessage(sessionID, content string) (bool, error) {
	if s == nil || s.orch == nil {
		return false, nil
	}
	accepted, _, _, _, err := s.orch.EnqueueUserMessage(sessionID, content)
	return accepted, err
}

// SetSessionPendingMergeFollowup implements biz.NativeTurnGateway — delegates to ChatOrchestrator.
func (s *ChatService) SetSessionPendingMergeFollowup(sessionID string, merge bool) {
	if s == nil || s.orch == nil {
		return
	}
	s.orch.SetSessionPendingMergeFollowup(sessionID, merge)
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
func (s *ChatService) ActiveSessionRunPhase(ctx context.Context, sessionID string) string {
	if s == nil || s.orch == nil || s.orch.chJobs.SessionRuns == nil {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	run, err := s.orch.chJobs.SessionRuns.GetActiveForSession(ctx, sessionID)
	if err != nil || run.ID == "" {
		return ""
	}
	return run.Phase
}

// LastPendingMessageID returns the most recently enqueued pending message id for a session, if any.
func (s *ChatService) LastPendingMessageID(sessionID string) string {
	return s.orch.LastPendingMessageID(sessionID)
}

func (s *ChatService) GetPendingMessages(ctx context.Context, req *chatv1.GetPendingMessagesRequest) (*chatv1.GetPendingMessagesResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
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
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	pendingID := strings.TrimSpace(req.GetPendingId())
	if pendingID == "" {
		return nil, kerrors.BadRequest("CHAT", "pending_id is required")
	}
	cancelled := s.orch.CancelPendingMessage(sessionID, pendingID)
	return &chatv1.CancelPendingMessageResponse{Cancelled: cancelled}, nil
}

func (s *ChatService) UpdatePendingMessage(ctx context.Context, req *chatv1.UpdatePendingMessageRequest) (*chatv1.UpdatePendingMessageResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	pendingID := strings.TrimSpace(req.GetPendingId())
	if pendingID == "" {
		return nil, kerrors.BadRequest("CHAT", "pending_id is required")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, kerrors.BadRequest("CHAT", "content is required")
	}
	updated := s.orch.UpdatePendingMessage(sessionID, pendingID, content)
	return &chatv1.UpdatePendingMessageResponse{Updated: updated}, nil
}

func (s *ChatService) EnqueueUserMessage(ctx context.Context, req *chatv1.EnqueueUserMessageRequest) (*chatv1.EnqueueUserMessageResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	content := strings.TrimSpace(req.GetContent())
	if content == "" {
		return nil, kerrors.BadRequest("CHAT", "content is required")
	}

	accepted, queued, pendingID, rejectReason, err := s.orch.EnqueueUserMessage(sessionID, content)
	if err != nil {
		return nil, err
	}
	if !accepted {
		return &chatv1.EnqueueUserMessageResponse{Accepted: false}, nil
	}
	if queued {
		if pendingID == "" {
			return nil, kerrors.BadRequest("CHAT", enqueueRejectMessage(rejectReason))
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
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
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
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	reply := strings.TrimSpace(req.GetReply())
	if reply == "" {
		return nil, kerrors.BadRequest("CHAT", "reply is required")
	}
	ch, ok := s.orch.LoadAwaitChannel(sessionID)
	if !ok {
		runID, canResume := s.orch.canResumeAwait(ctx, sessionID)
		if canResume {
			if req.RunId != nil && strings.TrimSpace(*req.RunId) != "" {
				runID = strings.TrimSpace(*req.RunId)
			}
			if err := s.orch.resumeAwaitAfterRestart(ctx, sessionID, reply, runID); err != nil {
				if errors.Is(err, errResumeInFlight) {
					return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
				}
				return nil, err
			}
			return &chatv1.AwaitUserReplyResponse{Accepted: true}, nil
		}
		return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
	}
	runID := ""
	if req.RunId != nil {
		runID = *req.RunId
	}
	select {
	case ch <- biz.AwaitReplyMsg{RunID: runID, Reply: reply}:
		return &chatv1.AwaitUserReplyResponse{Accepted: true}, nil
	default:
		return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
	}
}
