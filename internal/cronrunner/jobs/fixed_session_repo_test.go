package jobs

import (
	"context"
	"database/sql"
	"errors"

	sessionsess "aranea-agents/internal/biz/session"
)

// fixedSessionRepo implements sessionsess.SessionRepo for auto-memory integration tests.
type fixedSessionRepo struct {
	sess sessionsess.Session
	msgs []sessionsess.ChatMessage
}

func (r fixedSessionRepo) GetSessionByID(_ context.Context, id string) (sessionsess.Session, error) {
	if id != r.sess.ID {
		return sessionsess.Session{}, sql.ErrNoRows
	}
	return r.sess, nil
}

func (r fixedSessionRepo) ListMessagesRecent(_ context.Context, sessionID string, limit int) ([]sessionsess.ChatMessage, error) {
	if sessionID != r.sess.ID {
		return nil, nil
	}
	if limit <= 0 || limit >= len(r.msgs) {
		return append([]sessionsess.ChatMessage(nil), r.msgs...), nil
	}
	return append([]sessionsess.ChatMessage(nil), r.msgs[len(r.msgs)-limit:]...), nil
}

func (fixedSessionRepo) SearchSessions(context.Context, sessionsess.SessionSearchQuery) (sessionsess.SessionListResult, error) {
	return sessionsess.SessionListResult{}, nil
}
func (fixedSessionRepo) CreateSession(context.Context, sessionsess.Session) (sessionsess.Session, error) {
	return sessionsess.Session{}, errors.New("not implemented")
}
func (fixedSessionRepo) UpdateSessionTitle(context.Context, string, string) (sessionsess.Session, error) {
	return sessionsess.Session{}, errors.New("not implemented")
}
func (fixedSessionRepo) UpdateSession(context.Context, string, sessionsess.SessionUpdateFields) (sessionsess.Session, error) {
	return sessionsess.Session{}, errors.New("not implemented")
}
func (fixedSessionRepo) RestoreSession(context.Context, string) (sessionsess.Session, error) {
	return sessionsess.Session{}, errors.New("not implemented")
}
func (fixedSessionRepo) ArchiveSession(context.Context, string) (int, error) { return 0, nil }
func (fixedSessionRepo) DeleteSession(context.Context, string) (int, error)  { return 0, nil }
func (fixedSessionRepo) DeleteSessionsByAgentID(context.Context, string) error {
	return nil
}
func (fixedSessionRepo) CountMessagesBySession(context.Context, string) (int, error) { return 0, nil }
func (fixedSessionRepo) ListMessagesBySession(context.Context, string, int, int) ([]sessionsess.ChatMessage, error) {
	return nil, nil
}
func (fixedSessionRepo) ListMessagesAfterTurn(context.Context, string, int) ([]sessionsess.ChatMessage, error) {
	return nil, nil
}
func (fixedSessionRepo) ListMessagesByStatus(context.Context, string, string, int) ([]sessionsess.ChatMessage, error) {
	return nil, nil
}
func (fixedSessionRepo) SearchMessages(context.Context, sessionsess.MessageSearchQuery) (sessionsess.MessageSearchResult, error) {
	return sessionsess.MessageSearchResult{}, nil
}
func (fixedSessionRepo) ListToolInvocationsBySession(context.Context, string, int) ([]sessionsess.ToolInvocationView, error) {
	return nil, nil
}
func (fixedSessionRepo) ListSkillInvocationsBySession(context.Context, string, int) ([]sessionsess.SkillInvocationView, error) {
	return nil, nil
}
func (fixedSessionRepo) ListTimelineEventRefsPaged(context.Context, string, sessionsess.TimelineQuery) ([]sessionsess.TimelineEventRef, int, error) {
	return nil, 0, nil
}
func (fixedSessionRepo) ListMessagesByIDs(context.Context, string, []string) ([]sessionsess.ChatMessage, error) {
	return nil, nil
}
func (fixedSessionRepo) ListToolInvocationsByIDs(context.Context, string, []string) ([]sessionsess.ToolInvocationView, error) {
	return nil, nil
}
func (fixedSessionRepo) ListSkillInvocationsByIDs(context.Context, string, []string) ([]sessionsess.SkillInvocationView, error) {
	return nil, nil
}
func (fixedSessionRepo) LookupAgentDisplayNames(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (fixedSessionRepo) AppendChatTurn(context.Context, string, sessionsess.ChatMessage, sessionsess.ChatMessage) error {
	return nil
}
func (fixedSessionRepo) AppendChatMessage(context.Context, string, sessionsess.ChatMessage, bool) error {
	return nil
}
func (fixedSessionRepo) UpdateMessageFeedbackJSON(context.Context, string, string, string, string) error {
	return nil
}
func (fixedSessionRepo) UpsertChatActivityMessage(context.Context, string, sessionsess.ChatMessage) (bool, error) {
	return false, nil
}
func (fixedSessionRepo) UpdateRunnerSnapshotJSON(context.Context, string, string) error { return nil }
func (fixedSessionRepo) UpdateSessionContextFromLLMUsage(context.Context, string, int, int, int) error {
	return nil
}
func (fixedSessionRepo) UpdateSessionContextAfterCompression(context.Context, string, int, int) error {
	return nil
}
func (fixedSessionRepo) InsertSessionSummary(context.Context, sessionsess.SessionSummary) error {
	return nil
}
func (fixedSessionRepo) MaxSessionSummaryToTurn(context.Context, string) (int, error) { return 0, nil }
func (fixedSessionRepo) ListSessionSummaries(context.Context, string) ([]sessionsess.SessionSummary, error) {
	return nil, nil
}
func (fixedSessionRepo) LatestSessionSummaryTime(context.Context, string) (string, error) {
	return "", nil
}
func (fixedSessionRepo) UpdateSessionListSummary(context.Context, string, string) error { return nil }
func (fixedSessionRepo) GetSessionState(context.Context, string) (map[string]string, error) {
	return nil, nil
}
func (fixedSessionRepo) SaveSessionState(context.Context, string, map[string]string) error {
	return nil
}
func (fixedSessionRepo) CreateSessionTurn(context.Context, sessionsess.SessionTurn) (sessionsess.SessionTurn, error) {
	return sessionsess.SessionTurn{}, errors.New("not implemented")
}
func (fixedSessionRepo) UpdateSessionTurn(context.Context, string, sessionsess.SessionTurnUpdateFields) (sessionsess.SessionTurn, error) {
	return sessionsess.SessionTurn{}, errors.New("not implemented")
}
func (fixedSessionRepo) ListSessionTurns(context.Context, string, int, int) (sessionsess.SessionTurnListResult, error) {
	return sessionsess.SessionTurnListResult{}, nil
}
func (fixedSessionRepo) GetSessionTurn(context.Context, string) (sessionsess.SessionTurn, error) {
	return sessionsess.SessionTurn{}, sql.ErrNoRows
}
func (fixedSessionRepo) IncrementInvocationCounts(context.Context, string, int, int, int) error {
	return nil
}
func (fixedSessionRepo) ApplyMetricsDelta(context.Context, *sessionsess.SessionMetricsDelta) error {
	return nil
}
func (fixedSessionRepo) ListSessionsForBatch(context.Context, sessionsess.SessionSearchQuery) ([]sessionsess.Session, error) {
	return nil, nil
}
func (fixedSessionRepo) ListSessionsByIDs(context.Context, []string) ([]sessionsess.Session, error) {
	return nil, nil
}
func (fixedSessionRepo) ArchiveSessionsByIDs(context.Context, []string) (int, []string, error) {
	return 0, nil, nil
}
func (fixedSessionRepo) DeleteSessionsByIDs(context.Context, []string) (int, []string, error) {
	return 0, nil, nil
}
func (fixedSessionRepo) PinSession(context.Context, string) (sessionsess.Session, error) {
	return sessionsess.Session{}, errors.New("not implemented")
}
func (fixedSessionRepo) UnpinSession(context.Context, string) (sessionsess.Session, error) {
	return sessionsess.Session{}, errors.New("not implemented")
}
func (fixedSessionRepo) BumpSessionRevision(context.Context, string) (int64, error) { return 0, nil }
func (fixedSessionRepo) GetSessionRevision(context.Context, string) (int64, error)  { return 0, nil }
func (fixedSessionRepo) ListMessagesAfterRevision(context.Context, string, int64) ([]sessionsess.ChatMessage, error) {
	return nil, nil
}

func (fixedSessionRepo) SessionSummaryExists(context.Context, string, int, int) (bool, error) {
	return false, nil
}
func (fixedSessionRepo) UpdateChatMessageStatus(context.Context, string, string, string, string) error {
	return nil
}
func (fixedSessionRepo) TryIncrementCompressVersion(context.Context, string) (int64, error) {
	return 0, nil
}
func (fixedSessionRepo) CompressSessionInTx(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
func (fixedSessionRepo) PatchSessionState(_ context.Context, _ string, _ map[string]string, _ []string) error {
	return nil
}
func (fixedSessionRepo) ListByParentSessionID(_ context.Context, _ string) ([]sessionsess.Session, error) {
	return nil, nil
}
func (fixedSessionRepo) ListActiveAgentUserKeys(_ context.Context, _ int) ([]sessionsess.AgentUserKey, error) {
	return nil, nil
}

var _ sessionsess.SessionRepo = fixedSessionRepo{}

var (
	_ sessionsess.SessionReader       = fixedSessionRepo{}
	_ sessionsess.SessionWriter       = fixedSessionRepo{}
	_ sessionsess.SessionMutator      = fixedSessionRepo{}
	_ sessionsess.SessionBatchMutator = fixedSessionRepo{}
	_ sessionsess.MessageReader       = fixedSessionRepo{}
	_ sessionsess.MessageSearchReader = fixedSessionRepo{}
	_ sessionsess.MessageWriter       = fixedSessionRepo{}
	_ sessionsess.MessageStatusWriter = fixedSessionRepo{}
	_ sessionsess.TimelineReader      = fixedSessionRepo{}
	_ sessionsess.InvocationReader    = fixedSessionRepo{}
	_ sessionsess.SummaryReader       = fixedSessionRepo{}
	_ sessionsess.SummaryWriter       = fixedSessionRepo{}
	_ sessionsess.StateRepo           = fixedSessionRepo{}
	_ sessionsess.TurnRepo            = fixedSessionRepo{}
	_ sessionsess.ContextUpdater      = fixedSessionRepo{}
	_ sessionsess.CompressRepo        = fixedSessionRepo{}
)
