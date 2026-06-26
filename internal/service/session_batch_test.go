package service_test

import (
	"context"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/internal/service"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

type batchSessionRepo struct {
	sessions map[string]biz.Session
	messages []biz.ChatMessage
	tree     *biz.SessionTree
}

// ListBySession implements sessstatus.ActivityLister for batchSessionRepo.
// Phase 1c-3: message reads go through ActivityMessageReader → ActivityLister,
// not the messages table. Test repos convert stored ChatMessage fixtures to
// ActivityEntry shape so the production code path works in tests.
//
// Filters by sessionID to match the production contract: ActivityLister must
// only return activities belonging to the requested session.
func (m *batchSessionRepo) ListBySession(_ context.Context, sessionID string) ([]sessstatus.ActivityEntry, error) {
	if sessionID == "" {
		return nil, nil
	}
	var filtered []biz.ChatMessage
	for _, msg := range m.messages {
		if msg.SessionID == sessionID {
			filtered = append(filtered, msg)
		}
	}
	return chatMessagesToActivityEntries(filtered), nil
}

// ListBySessionTurn ignores turnID — matches production behavior where
// ActivityMessageReader.ListMessagesAfterTurn returns all messages (turn
// filtering is handled at the Activity layer).
func (m *batchSessionRepo) ListBySessionTurn(ctx context.Context, sessionID, _ string) ([]sessstatus.ActivityEntry, error) {
	return m.ListBySession(ctx, sessionID)
}

// chatMessagesToActivityEntries converts ChatMessage fixtures to ActivityEntry
// shape for test repos implementing ActivityLister.
func chatMessagesToActivityEntries(msgs []biz.ChatMessage) []sessstatus.ActivityEntry {
	acts := make([]sessstatus.ActivityEntry, 0, len(msgs))
	for _, m := range msgs {
		var kind string
		switch m.Role {
		case "user":
			kind = "task"
		case "assistant":
			kind = "reply"
		case "tool":
			kind = "action"
		case "system":
			kind = "notice"
		default:
			kind = "task"
		}
		a := sessstatus.ActivityEntry{
			ID:        m.ID,
			Kind:      kind,
			SessionID: m.SessionID,
			TurnID:    m.TurnID,
			Content:   m.ContentMarkdown,
		}
		if kind == "action" {
			a.ToolResult = m.ContentMarkdown
		}
		if m.CreatedAt != "" {
			if t, err := time.Parse(time.RFC3339Nano, m.CreatedAt); err == nil {
				a.Timestamp = t
			}
		}
		acts = append(acts, a)
	}
	return acts
}

// Compile-time assertion that batchSessionRepo satisfies ActivityLister.
var _ sessstatus.ActivityLister = (*batchSessionRepo)(nil)

func (m *batchSessionRepo) SearchSessions(context.Context, biz.SessionSearchQuery) (biz.SessionListResult, error) {
	return biz.SessionListResult{}, nil
}
func (m *batchSessionRepo) CreateSession(context.Context, biz.Session) (biz.Session, error) {
	return biz.Session{}, nil
}
func (m *batchSessionRepo) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return biz.Session{}, apierror.NotFound(apierror.DomainSession, "not found")
	}
	return s, nil
}
func (m *batchSessionRepo) UpdateSessionTitle(context.Context, string, string) (biz.Session, error) {
	return biz.Session{}, nil
}
func (m *batchSessionRepo) UpdateSession(context.Context, string, biz.SessionUpdateFields) (biz.Session, error) {
	return biz.Session{}, nil
}
func (m *batchSessionRepo) RestoreSession(context.Context, string) (biz.Session, error) {
	return biz.Session{}, nil
}
func (m *batchSessionRepo) ArchiveSession(context.Context, string) (int, error) { return 0, nil }
func (m *batchSessionRepo) DeleteSession(context.Context, string) (int, error)  { return 0, nil }
func (m *batchSessionRepo) DeleteSessionsByAgentID(context.Context, string) error {
	return nil
}
func (m *batchSessionRepo) CountMessagesBySession(context.Context, string) (int, error) {
	return 0, nil
}
func (m *batchSessionRepo) ListMessagesBySession(context.Context, string, int, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListMessagesAfterTurn(context.Context, string, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListMessagesByStatus(context.Context, string, string, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListMessagesRecent(context.Context, string, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListToolInvocationsBySession(context.Context, string, int) ([]biz.ToolInvocationView, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListSkillInvocationsBySession(context.Context, string, int) ([]biz.SkillInvocationView, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListTimelineEventRefsPaged(context.Context, string, biz.TimelineQuery) ([]biz.TimelineEventRef, int, error) {
	return nil, 0, nil
}
func (m *batchSessionRepo) ListMessagesByIDs(context.Context, string, []string) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListToolInvocationsByIDs(context.Context, string, []string) ([]biz.ToolInvocationView, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListSkillInvocationsByIDs(context.Context, string, []string) ([]biz.SkillInvocationView, error) {
	return nil, nil
}
func (m *batchSessionRepo) LookupAgentDisplayNames(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (m *batchSessionRepo) AppendChatTurn(context.Context, string, biz.ChatMessage, biz.ChatMessage) error {
	return nil
}
func (m *batchSessionRepo) SearchMessages(context.Context, biz.MessageSearchQuery) (biz.MessageSearchResult, error) {
	return biz.MessageSearchResult{}, nil
}

func (m *batchSessionRepo) AppendChatMessage(context.Context, string, biz.ChatMessage, bool) error {
	return nil
}
func (m *batchSessionRepo) UpdateMessageFeedbackJSON(context.Context, string, string, string, string) error {
	return nil
}
func (m *batchSessionRepo) UpsertChatActivityMessage(context.Context, string, biz.ChatMessage) (bool, error) {
	return false, nil
}
func (m *batchSessionRepo) UpdateRunnerSnapshotJSON(context.Context, string, string) error {
	return nil
}
func (m *batchSessionRepo) UpdateSessionContextFromLLMUsage(context.Context, string, int, int, int) error {
	return nil
}
func (m *batchSessionRepo) UpdateSessionContextAfterCompression(context.Context, string, int, int) error {
	return nil
}
func (m *batchSessionRepo) InsertSessionSummary(context.Context, biz.SessionSummary) error {
	return nil
}
func (m *batchSessionRepo) MaxSessionSummaryToTurn(context.Context, string) (int, error) {
	return 0, nil
}
func (m *batchSessionRepo) ListSessionSummaries(context.Context, string) ([]biz.SessionSummary, error) {
	return nil, nil
}
func (m *batchSessionRepo) LatestSessionSummaryTime(context.Context, string) (string, error) {
	return "", nil
}
func (m *batchSessionRepo) UpdateSessionListSummary(context.Context, string, string) error {
	return nil
}
func (m *batchSessionRepo) GetSessionState(context.Context, string) (map[string]string, error) {
	return nil, nil
}
func (m *batchSessionRepo) SaveSessionState(context.Context, string, map[string]string) error {
	return nil
}
func (m *batchSessionRepo) PatchSessionState(context.Context, string, map[string]string, []string) error {
	return nil
}
func (m *batchSessionRepo) CreateSessionTurn(context.Context, biz.SessionTurn) (biz.SessionTurn, error) {
	return biz.SessionTurn{}, nil
}
func (m *batchSessionRepo) UpdateSessionTurn(context.Context, string, biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	return biz.SessionTurn{}, nil
}
func (m *batchSessionRepo) ListSessionTurns(context.Context, string, int, int) (biz.SessionTurnListResult, error) {
	return biz.SessionTurnListResult{}, nil
}
func (m *batchSessionRepo) GetSessionTurn(context.Context, string) (biz.SessionTurn, error) {
	return biz.SessionTurn{}, nil
}
func (m *batchSessionRepo) IncrementInvocationCounts(context.Context, string, int, int, int) error {
	return nil
}
func (m *batchSessionRepo) ApplyMetricsDelta(context.Context, *sessstatus.SessionMetricsDelta) error {
	return nil
}
func (m *batchSessionRepo) ListSessionsByIDs(_ context.Context, ids []string) ([]biz.Session, error) {
	out := make([]biz.Session, 0, len(ids))
	for _, id := range ids {
		if s, ok := m.sessions[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}
func (m *batchSessionRepo) ListSessionsForBatch(_ context.Context, q biz.SessionSearchQuery) ([]biz.Session, error) {
	out := make([]biz.Session, 0, len(m.sessions))
	for _, s := range m.sessions {
		out = append(out, s)
	}
	if q.Limit > 0 && len(out) > q.Offset {
		start := q.Offset
		end := start + q.Limit
		if end > len(out) {
			end = len(out)
		}
		out = out[start:end]
	}
	return out, nil
}
func (m *batchSessionRepo) ArchiveSessionsByIDs(_ context.Context, ids []string) (int, []string, error) {
	return len(ids), nil, nil
}
func (m *batchSessionRepo) DeleteSessionsByIDs(_ context.Context, ids []string) (int, []string, error) {
	return len(ids), nil, nil
}
func (m *batchSessionRepo) PinSession(_ context.Context, id string) (biz.Session, error) {
	return m.GetSessionByID(context.Background(), id)
}
func (m *batchSessionRepo) UnpinSession(_ context.Context, id string) (biz.Session, error) {
	return m.GetSessionByID(context.Background(), id)
}
func (m *batchSessionRepo) BumpSessionRevision(context.Context, string) (int64, error) {
	return 1, nil
}
func (m *batchSessionRepo) GetSessionRevision(_ context.Context, sessionID string) (int64, error) {
	if s, ok := m.sessions[sessionID]; ok {
		return s.SessionRevision, nil
	}
	return 0, apierror.NotFound(apierror.DomainSession, "not found")
}
func (m *batchSessionRepo) ListMessagesAfterRevision(context.Context, string, int64) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *batchSessionRepo) TryIncrementCompressVersion(context.Context, string) (int64, error) {
	return 0, nil
}
func (m *batchSessionRepo) CompressSessionInTx(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
func (m *batchSessionRepo) SessionSummaryExists(context.Context, string, int, int) (bool, error) {
	return false, nil
}
func (m *batchSessionRepo) UpdateChatMessageStatus(context.Context, string, string, string, string) error {
	return nil
}
func (m *batchSessionRepo) ListByParentSessionID(_ context.Context, _ string) ([]biz.Session, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListActiveAgentUserKeys(_ context.Context, _ int) ([]sessstatus.AgentUserKey, error) {
	return nil, nil
}
func (m *batchSessionRepo) GetSessionTree(_ context.Context, _ string) (*biz.SessionTree, error) {
	return m.tree, nil
}
func (m *batchSessionRepo) ListChildSessions(_ context.Context, _ string) ([]biz.Session, error) {
	return nil, nil
}
func (m *batchSessionRepo) ListTeamAgentSessions(_ context.Context, _ string) ([]biz.Session, error) {
	return nil, nil
}

func TestSessionService_BatchPreviewSessions_validation(t *testing.T) {
	uc := biz.NewSessionUsecase(&batchSessionRepo{sessions: map[string]biz.Session{}}, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc := service.NewSessionService(uc, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	_, err := svc.BatchPreviewSessions(context.Background(), &v1.BatchPreviewSessionsRequest{})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected bad request, got %v", err)
	}

	_, err = svc.BatchArchiveSessions(context.Background(), &v1.BatchArchiveSessionsRequest{})
	if err == nil || !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected bad request for archive, got %v", err)
	}
}

func TestSessionService_BatchPreviewSessions_skippedNotFound(t *testing.T) {
	repo := &batchSessionRepo{sessions: map[string]biz.Session{
		"s1": {ID: "s1", Status: "completed", CreatedAt: "2020-01-01T00:00:00Z"},
	}}
	uc := biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	svc := service.NewSessionService(uc, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())

	resp, err := svc.BatchPreviewSessions(context.Background(), &v1.BatchPreviewSessionsRequest{
		Mode: "delete",
		Ids:  []string{"s1", "missing"},
	})
	if err != nil {
		t.Fatalf("preview: %v", err)
	}
	if resp.GetSkippedNotFound() != 1 {
		t.Fatalf("skipped_not_found: got %d want 1", resp.GetSkippedNotFound())
	}
	if resp.GetMatched() != 1 {
		t.Fatalf("matched: got %d want 1", resp.GetMatched())
	}
}
