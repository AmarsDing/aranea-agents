package service

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	localexec "aranea-agents/internal/agent/codeexecutor"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/chatactivity"
	"aranea-agents/internal/event"
	graphadapter "aranea-agents/internal/graph/adapter"
	"aranea-agents/internal/knowledge"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/team"
	"aranea-agents/internal/tools/mcpobserve"
	serviceawaitreply "aranea-agents/internal/tools/serviceawaitreply"
	tooltrpc "aranea-agents/internal/tools/trpc"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// awaitReplyCh is sent on channels keyed by sessionID when AwaitUserReply is called.
type awaitReplyCh struct {
	RunID string
	Reply string
}

type ChatService struct {
	chatv1.UnimplementedChatServiceServer

	teams              biz.TeamRepository
	teamsNative        *team.Runner
	usage              *biz.UsageUsecase
	monitor            *biz.MonitorUsecase
	td                 rt.TurnDeps
	pluginRT           *plugintrpc.Runtime
	pluginManager      *plugintrpc.Manager
	skillDBRepo        trpcskill.Repository
	artifacts          *biz.ArtifactUsecase
	runs               *rt.RunRegistry
	chatUC             *biz.ChatUsecase
	turnJobs           *biz.ChannelTurnJobUsecase
	awaitMetaCache     sync.Map // sessionID -> biz.ChatAwaitMeta
	resumeInFlight     sync.Map // sessionID -> struct{}; guards cross-restart await resume
	a2aUC              *biz.A2AUsecase
	knowledgeRetriever *knowledge.Retriever
	codeExecFactory    *localexec.Factory
}

type ChatServiceDeps struct {
	rt.TurnDeps
	Runs               *rt.RunRegistry
	PendingQueue       *rt.PendingMessageQueue
	Teams              biz.TeamRepository
	TeamsNative        *team.Runner
	Usage              *biz.UsageUsecase
	Monitor            *biz.MonitorUsecase
	PluginRT           *plugintrpc.Runtime
	PluginManager      *plugintrpc.Manager
	SkillDBRepo        trpcskill.Repository
	Artifacts          *biz.ArtifactUsecase
	A2AUC              *biz.A2AUsecase
	KnowledgeRetriever *knowledge.Retriever
	CodeExecFactory    *localexec.Factory
	MCPServers         *biz.MCPServerUsecase
	GraphFactory       biz.GraphBuilderFactory
	Graphs             *biz.GraphUsecase
	Tasks              *biz.TaskUsecase
	TeamGraphCoord     *team.TeamGraphRunCoordinator
	TurnJobs           *biz.ChannelTurnJobUsecase
}

func coalesceRunRegistry(r *rt.RunRegistry) *rt.RunRegistry {
	if r != nil {
		return r
	}
	return rt.NewRunRegistry()
}

func coalescePendingQueue(q *rt.PendingMessageQueue) *rt.PendingMessageQueue {
	if q != nil {
		return q
	}
	return rt.NewPendingMessageQueue()
}

func NewChatService(deps ChatServiceDeps) *ChatService {
	runs := coalesceRunRegistry(deps.Runs)
	pending := coalescePendingQueue(deps.PendingQueue)
	sessionLocks := NewSessionLockManager()
	s := &ChatService{
		teams:              deps.Teams,
		teamsNative:        deps.TeamsNative,
		usage:              deps.Usage,
		monitor:            deps.Monitor,
		pluginRT:           deps.PluginRT,
		pluginManager:      deps.PluginManager,
		skillDBRepo:        deps.SkillDBRepo,
		artifacts:          deps.Artifacts,
		runs:               runs,
		chatUC:             NewChatUsecaseFromDeps(runs, pending, sessionLocks, deps.Sessions, deps.Pipeline.Bus),
		turnJobs:           deps.TurnJobs,
		a2aUC:              deps.A2AUC,
		knowledgeRetriever: deps.KnowledgeRetriever,
		codeExecFactory:    deps.CodeExecFactory,
		td:                 deps.TurnDeps,
	}
	if deps.TeamsNative != nil {
		deps.TeamsNative.SetKnowledgeRetriever(deps.KnowledgeRetriever)
		deps.TeamsNative.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc {
			return s.makeAwaitReplyFunc(runCtx, sessionID, runID)
		})
		deps.TeamsNative.SetRunRegistry(s.runs)
		if deps.Graphs != nil {
			deps.TeamsNative.SetGraphBuildConfigLoader(graphadapter.NewLinkedGraphBuildConfigLoader(deps.Graphs))
		}
		if deps.GraphFactory != nil {
			if builder, ok := deps.GraphFactory.(graphadapter.TeamGraphRootBuilder); ok {
				deps.TeamsNative.SetGraphRootBuilder(builder)
			}
		}
		if deps.Tasks != nil {
			deps.TeamsNative.SetTeamGraphTaskCreator(team.NewTaskUsecaseGraphTaskCreator(deps.Tasks))
		}
		if deps.TeamGraphCoord != nil {
			deps.TeamsNative.SetTeamGraphRunCoordinator(deps.TeamGraphCoord)
			deps.TeamGraphCoord.SetFinisher(deps.TeamsNative)
		}
	}
	configureMCPObserve(deps.TurnDeps.Pipeline.Bus, deps.MCPServers)
	return s
}

func configureMCPObserve(bus event.Bus, mcp *biz.MCPServerUsecase) {
	if bus != nil {
		mcpobserve.SetBus(bus)
	}
	if mcp == nil {
		return
	}
	mcpobserve.SetMetadataRecorder(func(ctx context.Context, serverKey string, at time.Time) {
		_ = mcp.RecordReconnectMetadata(ctx, serverKey, at)
	})
}

func (s *ChatService) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	return s.nativeSendChatMessage(ctx, req)
}

func (s *ChatService) GetChatOptions(ctx context.Context, req *chatv1.GetChatOptionsRequest) (*chatv1.GetChatOptionsResponse, error) {
	return s.nativeGetChatOptions(ctx, req)
}

func (s *ChatService) StopGeneration(ctx context.Context, req *chatv1.StopGenerationRequest) (*chatv1.StopGenerationResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	stopped := s.cancelActiveRun(ctx, sessionID)
	return &chatv1.StopGenerationResponse{Stopped: stopped}, nil
}

// CancelRun stops the active run for a session (WS cancel and HTTP stop share this path).
func (s *ChatService) CancelRun(ctx context.Context, sessionID string) bool {
	return s.cancelActiveRun(ctx, strings.TrimSpace(sessionID))
}

func (s *ChatService) cancelActiveRun(ctx context.Context, sessionID string) bool {
	if s == nil || sessionID == "" {
		return false
	}
	stopped, runID := s.runs.Cancel(sessionID)
	if !stopped {
		return false
	}
	s.setRunStatus(sessionID, runID, "cancelled", "")
	if _, err := chatactivity.CancelRunningActivityMessages(ctx, s.td.Sessions, sessionID); err != nil {
		event.CtxFlowLogWarn(ctx, "chat.activity.cancel", "取消执行卡片查询失败",
			event.P("session_id", sessionID),
			event.P("error", err.Error()),
		)
	}
	return true
}

// RunGateway exposes the shared session run registry (Chat, Team, Cron, Channel, WS).
func (s *ChatService) RunGateway() rt.RunGateway {
	return s.runs
}

// ChannelFlowBuffer exposes only the FlowLogger buffer Channel ingress needs.
func (s *ChatService) ChannelFlowBuffer() *event.Buffer {
	if s == nil {
		return nil
	}
	return s.td.Pipeline.Buffer
}

// HasActiveRun reports whether a session has an in-flight run on the shared gateway.
func (s *ChatService) HasActiveRun(sessionID string) bool {
	return s.runs.HasActive(sessionID)
}

// LastPendingMessageID returns the most recently enqueued pending message id for a session, if any.
func (s *ChatService) LastPendingMessageID(sessionID string) string {
	if s == nil || s.chatUC == nil {
		return ""
	}
	entries := s.chatUC.GetPendingMessages(sessionID)
	if len(entries) == 0 {
		return ""
	}
	return entries[len(entries)-1].ID
}

func (s *ChatService) GetPendingMessages(ctx context.Context, req *chatv1.GetPendingMessagesRequest) (*chatv1.GetPendingMessagesResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	entries := s.chatUC.GetPendingMessages(sessionID)
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
	cancelled := s.chatUC.CancelPendingMessage(sessionID, pendingID)
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
	updated := s.chatUC.UpdatePendingMessage(sessionID, pendingID, content)
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

	accepted, queued, pendingID, rejectReason, err := s.chatUC.EnqueueUserMessage(sessionID, content)
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

// setRunStatus atomically updates the run status for a session and publishes a WS envelope.
func (s *ChatService) setRunStatus(sessionID, runID, status, errMsg string) {
	s.setRunStatusWithAwait(sessionID, runID, status, errMsg, nil)
}

func (s *ChatService) setRunStatusWithAwait(sessionID, runID, status, errMsg string, await *AwaitStatusMeta) {
	s.runs.SetStatus(sessionID, runID, status, errMsg)
	if await != nil {
		PublishRunStatusMeta(s.td.Pipeline.Bus, sessionID, runID, status, errMsg, await)
	} else {
		PublishRunStatus(s.td.Pipeline.Bus, sessionID, runID, status, errMsg)
	}
	s.persistRunStatus(context.Background(), sessionID, runID, status, errMsg)
}

func (s *ChatService) publishRunStatus(sessionID, runID, status, errMsg string) {
	PublishRunStatus(s.td.Pipeline.Bus, sessionID, runID, status, errMsg)
}

// GetRunStatus returns the current run lifecycle state for a session.
// Service-layer status comes from RunRegistry; when a ManagedRunner is active,
// framework fields are merged from ManagedRunner.RunStatus (request_id = session_id).
func (s *ChatService) GetRunStatus(ctx context.Context, req *chatv1.GetRunStatusRequest) (*chatv1.RunStatus, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	resp := &chatv1.RunStatus{Status: "idle"}
	if entry, ok := s.runs.GetStatus(sessionID); ok {
		resp.RunId = entry.RunID
		resp.Status = entry.Status
		resp.ErrorMessage = entry.ErrMsg
		if !entry.UpdatedAt.IsZero() {
			resp.UpdatedAt = entry.UpdatedAt.Format(time.RFC3339)
		}
	} else if snap, ok := s.hydrateRunStatusFromSession(ctx, sessionID); ok {
		resp.RunId = snap.RunID
		resp.Status = snap.Status
		resp.ErrorMessage = snap.ErrorMessage
		resp.UpdatedAt = snap.UpdatedAt
	}
	if runner, _, active := s.runs.ActiveRunner(sessionID); active {
		applyFrameworkRunStatus(resp, runner, sessionID)
	}
	if meta := s.resolveAwaitMeta(ctx, sessionID, resp.Status); strings.TrimSpace(resp.Status) == "awaiting_user" {
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

func (s *ChatService) lockSession(sessionID string) func() {
	return s.chatUC.LockSession(sessionID)
}

// makeAwaitReplyFunc returns a ReplyFunc closure that the ServiceTool calls to
// pause the current agent turn (EP-RT-02).  The closure:
//  1. Creates a buffered channel keyed by sessionID in awaitChans.
//  2. Sets the run status to "awaiting_user" so the frontend can react.
//  3. Blocks until AwaitUserReply delivers a reply or ctx is cancelled.
func (s *ChatService) makeAwaitReplyFunc(runCtx context.Context, sessionID, runID string) func(context.Context) (string, error) {
	return func(toolCtx context.Context) (string, error) {
		ch := make(chan awaitReplyCh, 1)
		s.chatUC.RegisterAwaitChannel(sessionID, ch)
		awaitMeta := AwaitStatusMeta{Kind: biz.ChatAwaitKindReply}
		if req, ok := serviceawaitreply.ToolConfirmRequestFromContext(toolCtx); ok {
			awaitMeta = AwaitStatusMeta{
				Kind:       biz.ChatAwaitKindToolConfirm,
				ToolKey:    req.ToolKey,
				ToolCallID: req.ToolCallID,
			}
		}
		s.setRunStatusWithAwait(sessionID, runID, "awaiting_user", "", &awaitMeta)
		s.persistAwaitMarkers(toolCtx, sessionID, runID, awaitMeta, true)
		defer func() {
			s.chatUC.DeleteAwaitChannel(sessionID)
			s.clearAwaitMetaCache(sessionID)
			s.setRunStatus(sessionID, runID, "running", "")
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
	val, ok := s.chatUC.LoadAwaitChannel(sessionID)
	if !ok {
		runID, canResume := s.canResumeAwait(ctx, sessionID)
		if canResume {
			if req.RunId != nil && strings.TrimSpace(*req.RunId) != "" {
				runID = strings.TrimSpace(*req.RunId)
			}
			if err := s.resumeAwaitAfterRestart(ctx, sessionID, reply, runID); err != nil {
				if errors.Is(err, errResumeInFlight) {
					return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
				}
				return nil, err
			}
			return &chatv1.AwaitUserReplyResponse{Accepted: true}, nil
		}
		return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
	}
	ch, ok := val.(chan awaitReplyCh)
	if !ok {
		return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
	}
	runID := ""
	if req.RunId != nil {
		runID = *req.RunId
	}
	select {
	case ch <- awaitReplyCh{RunID: runID, Reply: reply}:
		return &chatv1.AwaitUserReplyResponse{Accepted: true}, nil
	default:
		return &chatv1.AwaitUserReplyResponse{Accepted: false}, nil
	}
}
