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
	"aranea-agents/internal/runtimedeps"
	"aranea-agents/internal/team"

	trpcrunner "trpc.group/trpc-go/trpc-agent-go/runner"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/google/uuid"
)

type ChatService struct {
	chatv1.UnimplementedChatServiceServer

	teams          biz.TeamRepository
	teamsNative    *team.Runner
	usage          *biz.UsageUsecase
	td             runtimedeps.TurnDeps
	activeRuns     sync.Map
	pendingQueue   sync.Map
	pendingCancels sync.Map
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
	RT           *runtimedeps.Runtime
	Compress     biz.NativeTurnCompressor
	EventBus     event.Bus
}

func NewChatService(deps ChatServiceDeps) *ChatService {
	s := &ChatService{
		teams:       deps.Teams,
		teamsNative: deps.TeamsNative,
		usage:       deps.Usage,
		td: runtimedeps.TurnDeps{
			Agents:       deps.Agents,
			AgentsUC:     deps.AgentsUC,
			ToolsCatalog: deps.ToolsCatalog,
			ToolUC:       deps.ToolUC,
			LLMCatalog:   deps.LLMCatalog,
			SkillUC:      deps.SkillUC,
			Sys:          deps.Sys,
			RT:           deps.RT,
			LLMHTTP:      &http.Client{Timeout: 300 * time.Second},
			Sessions:     deps.Sessions,
			Compress:     deps.Compress,
			EventBus:     deps.EventBus,
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
