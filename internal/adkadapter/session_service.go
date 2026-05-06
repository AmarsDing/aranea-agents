package adkadapter

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"strings"
	"time"

	"aranea-agents/internal/biz"

	"google.golang.org/adk/session"
)

// Default identifiers for Aranea admin sessions (single-tenant); uniqueness is sessions.id.
const (
	DefaultAppName = "aranea"
	DefaultUserID  = "local"
)

// SessionRepositorySubset is the persistence surface for [session.Service].
type SessionRepositorySubset interface {
	GetSessionByID(ctx context.Context, id string) (biz.Session, error)
	UpdateAdkSnapshotJSON(ctx context.Context, sessionID string, snapshotJSON string) error
	ListMessagesBySession(ctx context.Context, sessionID string) ([]biz.ChatMessage, error)
}

// BizSessionService persists ADK session state in sessions.adk_snapshot_json.
type BizSessionService struct {
	Repo    SessionRepositorySubset
	AppName string
	// ResolveAssistantAuthor maps session.AgentID to the string used as event Author for legacy-hydrated assistant rows (typically AgentKey).
	ResolveAssistantAuthor func(ctx context.Context, agentID string) (string, error)
}

var _ session.Service = (*BizSessionService)(nil)

// NewSessionService builds a SQLite-backed session.Service (Runner-compatible).
func NewSessionService(repo SessionRepositorySubset) *BizSessionService {
	if repo == nil {
		return nil
	}
	return &BizSessionService{Repo: repo, AppName: DefaultAppName}
}

// NewSessionService returns nil if repo is nil.
func (s *BizSessionService) app() string {
	if s != nil && strings.TrimSpace(s.AppName) != "" {
		return strings.TrimSpace(s.AppName)
	}
	return DefaultAppName
}

// Create initializes ADK snapshot for an existing biz session row (Runner auto-create off).
func (s *BizSessionService) Create(ctx context.Context, req *session.CreateRequest) (*session.CreateResponse, error) {
	if req == nil || strings.TrimSpace(req.AppName) == "" || strings.TrimSpace(req.UserID) == "" {
		return nil, fmt.Errorf("adkadapter: invalid create request")
	}
	sid := strings.TrimSpace(req.SessionID)
	if sid == "" {
		return nil, fmt.Errorf("adkadapter: session_id is required")
	}
	if _, err := s.Repo.GetSessionByID(ctx, sid); err != nil {
		return nil, err
	}
	ms := newMutableSession(req.AppName, req.UserID, sid)
	if req.State != nil {
		maps.Copy(ms.state, req.State)
	}
	payload, err := marshalBundle(bundleFromSession(ms))
	if err != nil {
		return nil, err
	}
	if err := s.Repo.UpdateAdkSnapshotJSON(ctx, sid, payload); err != nil {
		return nil, err
	}
	return &session.CreateResponse{Session: ms}, nil
}

// Get loads snapshot for Runner.
func (s *BizSessionService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("adkadapter: nil get request")
	}
	sid := strings.TrimSpace(req.SessionID)
	if sid == "" {
		return nil, fmt.Errorf("adkadapter: session_id required")
	}
	bizSess, err := s.Repo.GetSessionByID(ctx, sid)
	if err != nil {
		return nil, err
	}
	bundle, err := unmarshalBundle(bizSess.AdkSnapshotJSON)
	if err != nil {
		return nil, fmt.Errorf("adkadapter: corrupt adk snapshot: %w", err)
	}
	if err := s.maybeHydrateFromLegacyMessages(ctx, sid, bizSess, bundle); err != nil {
		return nil, err
	}
	ms := bundle.toMutableSession(sid)
	if ms.appName == DefaultAppName && req.AppName != "" {
		ms.appName = req.AppName
	}
	if ms.userID == DefaultUserID && req.UserID != "" {
		ms.userID = req.UserID
	}

	ev := ms.events
	if req.NumRecentEvents > 0 && len(ev) > 0 {
		start := max(len(ev)-req.NumRecentEvents, 0)
		ev = ev[start:]
	}
	if !req.After.IsZero() && len(ev) > 0 {
		cut := 0
		for i := range ev {
			if !ev[i].Timestamp.Before(req.After) {
				cut = i
				break
			}
		}
		ev = ev[cut:]
	}
	ms.events = ev
	return &session.GetResponse{Session: ms}, nil
}

// List is not used by Runner for chat; return empty.
func (s *BizSessionService) List(ctx context.Context, req *session.ListRequest) (*session.ListResponse, error) {
	_ = ctx
	_ = s
	return &session.ListResponse{Sessions: nil}, nil
}

// Delete clears ADK blob for the session id.
func (s *BizSessionService) Delete(ctx context.Context, req *session.DeleteRequest) error {
	sid := strings.TrimSpace(req.SessionID)
	if sid == "" {
		return fmt.Errorf("adkadapter: session_id required")
	}
	return s.Repo.UpdateAdkSnapshotJSON(ctx, sid, "{}")
}

// AppendEvent merges event into snapshot JSON.
func (s *BizSessionService) AppendEvent(ctx context.Context, curSession session.Session, event *session.Event) error {
	if curSession == nil || event == nil {
		return fmt.Errorf("adkadapter: nil session or event")
	}
	if event.Partial {
		return nil
	}
	sid := strings.TrimSpace(curSession.ID())
	bizSess, err := s.Repo.GetSessionByID(ctx, sid)
	if err != nil {
		return err
	}
	bundle, err := unmarshalBundle(bizSess.AdkSnapshotJSON)
	if err != nil {
		return err
	}
	ms := bundle.toMutableSession(sid)

	if err := appendEventToMutable(ms, event); err != nil {
		return err
	}
	out := bundleFromSession(ms)
	raw, err := marshalBundle(out)
	if err != nil {
		return err
	}
	return s.Repo.UpdateAdkSnapshotJSON(ctx, sid, raw)
}

func appendEventToMutable(ms *mutableSession, event *session.Event) error {
	ms.mu.Lock()
	defer ms.mu.Unlock()
	evt := trimTempDeltaState(cloneEvent(event))
	if evt.Actions.StateDelta != nil {
		if ms.state == nil {
			ms.state = map[string]any{}
		}
		maps.Copy(ms.state, evt.Actions.StateDelta)
	}
	ms.events = append(ms.events, evt)
	ms.updatedAt = evt.Timestamp
	if ms.updatedAt.IsZero() {
		ms.updatedAt = time.Now()
	}
	return nil
}

func cloneEvent(event *session.Event) *session.Event {
	if event == nil {
		return nil
	}
	cp := *event
	cp.Actions = session.EventActions{
		StateDelta:                 maps.Clone(event.Actions.StateDelta),
		ArtifactDelta:              maps.Clone(event.Actions.ArtifactDelta),
		RequestedToolConfirmations: maps.Clone(event.Actions.RequestedToolConfirmations),
		TransferToAgent:            event.Actions.TransferToAgent,
		Escalate:                   event.Actions.Escalate,
		SkipSummarization:          event.Actions.SkipSummarization,
	}
	if event.LongRunningToolIDs != nil {
		cp.LongRunningToolIDs = slices.Clone(event.LongRunningToolIDs)
	}
	cp.LLMResponse = event.LLMResponse
	return &cp
}

func (s *BizSessionService) maybeHydrateFromLegacyMessages(ctx context.Context, sid string, bizSess biz.Session, bundle *persistedBundle) error {
	if bundle == nil || len(bundle.Events) > 0 {
		return nil
	}
	msgs, err := s.Repo.ListMessagesBySession(ctx, sid)
	if err != nil {
		return err
	}
	if len(msgs) == 0 {
		return nil
	}
	author := "agent"
	if s.ResolveAssistantAuthor != nil && strings.TrimSpace(bizSess.AgentID) != "" {
		if k, err := s.ResolveAssistantAuthor(ctx, strings.TrimSpace(bizSess.AgentID)); err == nil && strings.TrimSpace(k) != "" {
			author = strings.TrimSpace(k)
		}
	}
	bundle.Events = messagesToADKEvents(msgs, author)
	bundle.RootAgentName = author
	if bundle.State == nil {
		bundle.State = map[string]any{}
	}
	bundle.UpdatedAt = time.Now()
	raw, err := marshalBundle(bundle)
	if err != nil {
		return err
	}
	return s.Repo.UpdateAdkSnapshotJSON(ctx, sid, raw)
}

func trimTempDeltaState(event *session.Event) *session.Event {
	if len(event.Actions.StateDelta) == 0 {
		return event
	}
	filtered := make(map[string]any)
	for k, v := range event.Actions.StateDelta {
		if strings.HasPrefix(k, session.KeyPrefixTemp) {
			continue
		}
		filtered[k] = v
	}
	event.Actions.StateDelta = filtered
	return event
}
