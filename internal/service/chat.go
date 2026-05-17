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

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"
	trpcskill "trpc.group/trpc-go/trpc-agent-go/skill"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

// runStatusEntry holds the lifecycle state of a single agent run.
type runStatusEntry struct {
	RunID     string
	Status    string // idle | pending | running | awaiting_user | completed | failed | cancelled
	ErrMsg    string
	UpdatedAt time.Time
}

// awaitReplyCh is sent on channels keyed by sessionID when AwaitUserReply is called.
type awaitReplyCh struct {
	RunID string
	Reply string
}

type teamRunGuard struct {
	cancel context.CancelFunc
	runID  string
}

type ChatService struct {
	chatv1.UnimplementedChatServiceServer

	teams          biz.TeamRepository
	teamsNative    *team.Runner
	usage          *biz.UsageUsecase
	td             rt.TurnDeps
	pluginRT       *plugintrpc.Runtime
	skillDBRepo    trpcskill.Repository
	activeRuns     sync.Map
	pendingQueue   sync.Map
	pendingCancels sync.Map
	runStatuses    sync.Map
	awaitChans     sync.Map
}

type ChatServiceDeps struct {
	Teams        biz.TeamRepository
	TeamsNative  *team.Runner
	Usage        *biz.UsageUsecase
	Sessions     *biz.SessionUsecase
	Agents       biz.AgentRepository
	AgentsUC     *biz.AgentUsecase
	ToolsCatalog biz.ToolRepo
	ToolUC       *biz.ToolUsecase
	LLMCatalog   *biz.LlmProviderModelUsecase
	SkillUC      *biz.SkillUsecase
	Sys          biz.SystemSettingRepo
	Persist      rt.PersistenceSet
	Compress     biz.NativeTurnCompressor
	EventBus     event.Bus
	PluginRT     *plugintrpc.Runtime
	SkillDBRepo  trpcskill.Repository
}

func NewChatService(deps ChatServiceDeps) *ChatService {
	s := &ChatService{
		teams:       deps.Teams,
		teamsNative: deps.TeamsNative,
		usage:       deps.Usage,
		pluginRT:    deps.PluginRT,
		skillDBRepo: deps.SkillDBRepo,
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
			Persist:  deps.Persist,
			Pipeline: rt.EventPipeline{Bus: deps.EventBus},
			LLMHTTP:  &http.Client{Timeout: 300 * time.Second},
			Sessions: deps.Sessions,
			Compress: deps.Compress,
		},
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
	if cancelFn, ok := s.pendingCancels.LoadAndDelete(sessionID); ok {
		if c, ok := cancelFn.(context.CancelFunc); ok {
			c()
		}
	}
	val, ok := s.activeRuns.Load(sessionID)
	if !ok {
		return &chatv1.StopGenerationResponse{Stopped: false}, nil
	}
	if guard, ok := val.(*teamRunGuard); ok {
		guard.cancel()
		s.setRunStatus(sessionID, guard.runID, "cancelled", "")
		s.activeRuns.Delete(sessionID)
		return &chatv1.StopGenerationResponse{Stopped: true}, nil
	}
	r, ok := val.(trpcrunner.Runner)
	if !ok {
		return &chatv1.StopGenerationResponse{Stopped: false}, nil
	}
	if chatagent.CancelTRPCRun(r, sessionID) {
		return &chatv1.StopGenerationResponse{Stopped: true}, nil
	}
	_ = r.Close()
	s.activeRuns.Delete(sessionID)
	return &chatv1.StopGenerationResponse{Stopped: true}, nil
}

func (s *ChatService) CancelRun(ctx context.Context, sessionID string) bool {
	if cancelFn, ok := s.pendingCancels.LoadAndDelete(sessionID); ok {
		if c, ok := cancelFn.(context.CancelFunc); ok {
			c()
		}
	}
	val, ok := s.activeRuns.Load(sessionID)
	if !ok {
		return false
	}
	if guard, ok := val.(*teamRunGuard); ok {
		guard.cancel()
		s.setRunStatus(sessionID, guard.runID, "cancelled", "")
		s.activeRuns.Delete(sessionID)
		return true
	}
	r, ok := val.(trpcrunner.Runner)
	if !ok {
		return false
	}
	if chatagent.CancelTRPCRun(r, sessionID) {
		return true
	}
	_ = r.Close()
	s.activeRuns.Delete(sessionID)
	return true
}

type pendingEntry struct {
	ID        string
	Content   string
	Status    string
	CreatedAt string
}

func (s *ChatService) GetPendingMessages(ctx context.Context, req *chatv1.GetPendingMessagesRequest) (*chatv1.GetPendingMessagesResponse, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	val, ok := s.pendingQueue.Load(sessionID)
	if !ok {
		return &chatv1.GetPendingMessagesResponse{}, nil
	}
	entries, ok := val.([]pendingEntry)
	if !ok {
		return &chatv1.GetPendingMessagesResponse{}, nil
	}
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

const maxPendingPerSession = 32

func (s *ChatService) enqueuePending(sessionID, content string) string {
	id := uuid.NewString()
	entry := pendingEntry{
		ID:        id,
		Content:   content,
		Status:    "pending",
		CreatedAt: time.Now().Format(time.RFC3339),
	}
	for {
		existing, loaded := s.pendingQueue.LoadOrStore(sessionID, []pendingEntry{entry})
		if !loaded {
			return id
		}
		queue := existing.([]pendingEntry)
		if len(queue) >= maxPendingPerSession {
			return ""
		}
		queue = append(queue, entry)
		if s.pendingQueue.CompareAndSwap(sessionID, existing, queue) {
			return id
		}
	}
}

func (s *ChatService) dequeuePending(sessionID string) (pendingEntry, bool) {
	for {
		val, ok := s.pendingQueue.Load(sessionID)
		if !ok {
			return pendingEntry{}, false
		}
		queue := val.([]pendingEntry)
		if len(queue) == 0 {
			s.pendingQueue.Delete(sessionID)
			return pendingEntry{}, false
		}
		head := queue[0]
		remaining := queue[1:]
		if len(remaining) == 0 {
			if s.pendingQueue.CompareAndDelete(sessionID, val) {
				return head, true
			}
			continue
		}
		if s.pendingQueue.CompareAndSwap(sessionID, val, remaining) {
			return head, true
		}
	}
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
	cancelled := s.removePending(sessionID, pendingID)
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
	updated := s.updatePending(sessionID, pendingID, content)
	return &chatv1.UpdatePendingMessageResponse{Updated: updated}, nil
}

func (s *ChatService) removePending(sessionID, entryID string) bool {
	for {
		val, ok := s.pendingQueue.Load(sessionID)
		if !ok {
			return false
		}
		queue := val.([]pendingEntry)
		found := false
		filtered := make([]pendingEntry, 0, len(queue))
		for _, e := range queue {
			if e.ID == entryID && !found {
				found = true
				continue
			}
			filtered = append(filtered, e)
		}
		if !found {
			return false
		}
		if len(filtered) == 0 {
			if s.pendingQueue.CompareAndDelete(sessionID, val) {
				return true
			}
			continue
		}
		if s.pendingQueue.CompareAndSwap(sessionID, val, filtered) {
			return true
		}
	}
}

func (s *ChatService) updatePending(sessionID, entryID, newContent string) bool {
	for {
		val, ok := s.pendingQueue.Load(sessionID)
		if !ok {
			return false
		}
		queue := val.([]pendingEntry)
		found := false
		updated := make([]pendingEntry, len(queue))
		for i, e := range queue {
			if e.ID == entryID && !found {
				found = true
				updated[i] = pendingEntry{
					ID:        e.ID,
					Content:   newContent,
					Status:    e.Status,
					CreatedAt: e.CreatedAt,
				}
				continue
			}
			updated[i] = e
		}
		if !found {
			return false
		}
		if s.pendingQueue.CompareAndSwap(sessionID, val, updated) {
			return true
		}
	}
}

// setRunStatus atomically updates the run status for a session.
func (s *ChatService) setRunStatus(sessionID, runID, status, errMsg string) {
	s.runStatuses.Store(sessionID, &runStatusEntry{
		RunID:     runID,
		Status:    status,
		ErrMsg:    errMsg,
		UpdatedAt: time.Now(),
	})
}

// GetRunStatus returns the current run lifecycle state for a session.
func (s *ChatService) GetRunStatus(ctx context.Context, req *chatv1.GetRunStatusRequest) (*chatv1.RunStatus, error) {
	sessionID := strings.TrimSpace(req.GetSessionId())
	if sessionID == "" {
		return nil, kerrors.BadRequest("CHAT", "session_id is required")
	}
	val, ok := s.runStatuses.Load(sessionID)
	if !ok {
		return &chatv1.RunStatus{Status: "idle"}, nil
	}
	entry := val.(*runStatusEntry)
	return &chatv1.RunStatus{
		RunId:        entry.RunID,
		Status:       entry.Status,
		ErrorMessage: entry.ErrMsg,
		UpdatedAt:    entry.UpdatedAt.Format(time.RFC3339),
	}, nil
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
