package service

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"time"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	chatagent "aranea-agents/internal/agent"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	plugintrpc "aranea-agents/internal/plugin/trpc"
	rt "aranea-agents/internal/runtime"
	"aranea-agents/internal/team"
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

	teams         biz.TeamRepository
	teamsNative   *team.Runner
	usage         *biz.UsageUsecase
	td            rt.TurnDeps
	pluginRT      *plugintrpc.Runtime
	pluginManager *plugintrpc.Manager
	skillDBRepo   trpcskill.Repository
	runs          *rt.RunRegistry
	pending       *PendingMessageQueue
	awaitChans    sync.Map
	sessionMu     sync.Map
	a2aUC         *biz.A2AUsecase
}

type ChatServiceDeps struct {
	Runs          *rt.RunRegistry
	Teams         biz.TeamRepository
	TeamsNative   *team.Runner
	Usage         *biz.UsageUsecase
	Sessions      *biz.SessionUsecase
	Agents        biz.AgentRepository
	AgentsUC      *biz.AgentUsecase
	ToolsCatalog  biz.ToolRepo
	ToolUC        *biz.ToolUsecase
	LLMCatalog    *biz.LlmProviderModelUsecase
	SkillUC       *biz.SkillUsecase
	Sys           biz.SystemSettingRepo
	Persist       rt.PersistenceSet
	Compress      biz.NativeTurnCompressor
	EventBus      event.Bus
	PluginRT      *plugintrpc.Runtime
	PluginManager *plugintrpc.Manager
	SkillDBRepo   trpcskill.Repository
	A2AUC         *biz.A2AUsecase
}

func coalesceRunRegistry(r *rt.RunRegistry) *rt.RunRegistry {
	if r != nil {
		return r
	}
	return rt.NewRunRegistry()
}

func NewChatService(deps ChatServiceDeps) *ChatService {
	s := &ChatService{
		teams:         deps.Teams,
		teamsNative:   deps.TeamsNative,
		usage:         deps.Usage,
		pluginRT:      deps.PluginRT,
		pluginManager: deps.PluginManager,
		skillDBRepo:   deps.SkillDBRepo,
		runs:          coalesceRunRegistry(deps.Runs),
		pending:       NewPendingMessageQueue(),
		a2aUC:         deps.A2AUC,
		td: rt.TurnDeps{
			Catalog: rt.Catalog{
				Agents:   deps.Agents,
				AgentsUC: deps.AgentsUC,
				Tools:    deps.ToolsCatalog,
				ToolUC:   deps.ToolUC,
				LLM:      deps.LLMCatalog,
				SkillUC:  deps.SkillUC,
				Settings: deps.Sys,
			},
			Persist:   deps.Persist,
			Pipeline:  rt.EventPipeline{Bus: deps.EventBus},
			LLMHTTP:   &http.Client{Timeout: 300 * time.Second},
			Sessions:  deps.Sessions,
			Compress:  deps.Compress,
			RunnerMgr: rt.NewRunnerManagerFromPersist(deps.Persist),
		},
	}
	if deps.TeamsNative != nil {
		deps.TeamsNative.SetAwaitHookProvider(func(runCtx context.Context, sessionID, runID string) tooltrpc.ReplyFunc {
			return s.makeAwaitReplyFunc(runCtx, sessionID, runID)
		})
		deps.TeamsNative.SetRunRegistry(s.runs)
	}
	return s
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
	return &chatv1.StopGenerationResponse{Stopped: s.runs.Cancel(sessionID)}, nil
}

func (s *ChatService) CancelRun(ctx context.Context, sessionID string) bool {
	return s.runs.Cancel(sessionID)
}

// RunGateway exposes the shared session run registry (Chat, Team, Cron, Channel, WS).
func (s *ChatService) RunGateway() rt.RunGateway {
	return s.runs
}

// HasActiveRun reports whether a session has an in-flight run on the shared gateway.
func (s *ChatService) HasActiveRun(sessionID string) bool {
	return s.runs.HasActive(sessionID)
}

func (s *ChatService) GetPendingMessages(ctx context.Context, req *chatv1.GetPendingMessagesRequest) (*chatv1.GetPendingMessagesResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	entries := s.pending.List(sessionID)
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
	cancelled := s.pending.Remove(sessionID, pendingID)
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
	updated := s.pending.Update(sessionID, pendingID, content)
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

	unlock := s.lockSession(sessionID)
	defer unlock()
	if !s.runs.HasActive(sessionID) {
		return &chatv1.EnqueueUserMessageResponse{Accepted: false}, nil
	}
	enqueued, err := s.runs.EnqueueUserMessage(sessionID, content)
	if err != nil {
		return nil, err
	}
	if enqueued {
		return &chatv1.EnqueueUserMessageResponse{Accepted: true}, nil
	}
	pendingID := s.pending.Enqueue(sessionID, content)
	if pendingID == "" {
		return nil, kerrors.BadRequest("CHAT", "pending queue is full for this session")
	}
	return &chatv1.EnqueueUserMessageResponse{
		Accepted:  true,
		Queued:    true,
		PendingId: pendingID,
	}, nil
}

// setRunStatus atomically updates the run status for a session and publishes a WS envelope.
func (s *ChatService) setRunStatus(sessionID, runID, status, errMsg string) {
	s.runs.SetStatus(sessionID, runID, status, errMsg)
	s.publishRunStatus(sessionID, runID, status, errMsg)
}

func (s *ChatService) publishRunStatus(sessionID, runID, status, errMsg string) {
	bus := s.td.Pipeline.Bus
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "chat-service", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"run_id":        runID,
		"status":        status,
		"error_message": errMsg,
	}
	bus.Publish(context.Background(), env)
}

// publishMessageQueued notifies WS clients that a user message was accepted into the
// active run (steerable enqueue or pending queue) without starting a new turn.
func (s *ChatService) publishMessageQueued(sessionID string) {
	bus := s.td.Pipeline.Bus
	if bus == nil || strings.TrimSpace(sessionID) == "" {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeRunStatus, "chat-service", sessionID)
	env.Channel = event.RouteChannel(env)
	env.Metadata = map[string]any{
		"status": "queued",
		"hint":   "message_queued",
	}
	bus.Publish(context.Background(), env)
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
	}
	if runner, _, active := s.runs.ActiveRunner(sessionID); active {
		mergeFrameworkRunStatus(resp, runner, sessionID)
	}
	return resp, nil
}

func mergeFrameworkRunStatus(resp *chatv1.RunStatus, runner trpcrunner.Runner, requestID string) {
	if resp == nil || runner == nil {
		return
	}
	st, ok := chatagent.TRPCRunStatus(runner, requestID)
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
	val, _ := s.sessionMu.LoadOrStore(sessionID, &sync.Mutex{})
	mu := val.(*sync.Mutex)
	mu.Lock()
	return mu.Unlock
}

// makeAwaitReplyFunc returns a ReplyFunc closure that the ServiceTool calls to
// pause the current agent turn (EP-RT-02).  The closure:
//  1. Creates a buffered channel keyed by sessionID in awaitChans.
//  2. Sets the run status to "awaiting_user" so the frontend can react.
//  3. Blocks until AwaitUserReply delivers a reply or ctx is cancelled.
func (s *ChatService) makeAwaitReplyFunc(runCtx context.Context, sessionID, runID string) func(context.Context) (string, error) {
	return func(toolCtx context.Context) (string, error) {
		ch := make(chan awaitReplyCh, 1)
		s.awaitChans.Store(sessionID, ch)
		s.setRunStatus(sessionID, runID, "awaiting_user", "")
		defer func() {
			s.awaitChans.Delete(sessionID)
			// Restore "running" so downstream setRunStatus calls see a clean state.
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
	val, ok := s.awaitChans.Load(sessionID)
	if !ok {
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
