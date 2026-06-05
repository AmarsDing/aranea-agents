package service

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"

	"github.com/google/uuid"
)

type ingressPeerSessionRepo struct {
	byKey map[string]biz.ChannelPeerSession
}

func peerMapKey(channelID, peerKey string) string {
	return channelID + "\x00" + peerKey
}

func (r *ingressPeerSessionRepo) GetByChannelAndPeer(_ context.Context, channelID, peerKey string) (biz.ChannelPeerSession, error) {
	if r.byKey == nil {
		return biz.ChannelPeerSession{}, sql.ErrNoRows
	}
	row, ok := r.byKey[peerMapKey(channelID, peerKey)]
	if !ok {
		return biz.ChannelPeerSession{}, sql.ErrNoRows
	}
	return row, nil
}

func (r *ingressPeerSessionRepo) Create(_ context.Context, row biz.ChannelPeerSession) (biz.ChannelPeerSession, error) {
	if r.byKey == nil {
		r.byKey = map[string]biz.ChannelPeerSession{}
	}
	r.byKey[peerMapKey(row.ChannelID, row.PeerKey)] = row
	return row, nil
}

func (r *ingressPeerSessionRepo) UpdateSessionID(_ context.Context, channelID, peerKey, sessionID string) (biz.ChannelPeerSession, error) {
	key := peerMapKey(channelID, peerKey)
	row, ok := r.byKey[key]
	if !ok {
		return biz.ChannelPeerSession{}, sql.ErrNoRows
	}
	row.SessionID = sessionID
	r.byKey[key] = row
	return row, nil
}

func (r *ingressPeerSessionRepo) DeleteByChannelID(_ context.Context, channelID string) (int, error) {
	n := 0
	for k, row := range r.byKey {
		if row.ChannelID == channelID {
			delete(r.byKey, k)
			n++
		}
	}
	return n, nil
}

func (r *ingressPeerSessionRepo) DeleteBySessionID(_ context.Context, sessionID string) (int, error) {
	n := 0
	for k, row := range r.byKey {
		if row.SessionID == sessionID {
			delete(r.byKey, k)
			n++
		}
	}
	return n, nil
}

type ingressSessionRepo struct {
	sessions map[string]biz.Session
	created  int
}

func (m *ingressSessionRepo) SearchSessions(context.Context, biz.SessionSearchQuery) (biz.SessionListResult, error) {
	return biz.SessionListResult{}, nil
}
func (m *ingressSessionRepo) CreateSession(_ context.Context, in biz.Session) (biz.Session, error) {
	if m.sessions == nil {
		m.sessions = map[string]biz.Session{}
	}
	if strings.TrimSpace(in.ID) == "" {
		in.ID = uuid.NewString()
	}
	m.sessions[in.ID] = in
	m.created++
	return in, nil
}
func (m *ingressSessionRepo) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	s, ok := m.sessions[id]
	if !ok {
		return biz.Session{}, sql.ErrNoRows
	}
	return s, nil
}
func (m *ingressSessionRepo) UpdateSessionTitle(context.Context, string, string) (biz.Session, error) {
	return biz.Session{}, nil
}
func (m *ingressSessionRepo) UpdateSession(context.Context, string, biz.SessionUpdateFields) (biz.Session, error) {
	return biz.Session{}, nil
}
func (m *ingressSessionRepo) RestoreSession(context.Context, string) (biz.Session, error) {
	return biz.Session{}, nil
}
func (m *ingressSessionRepo) ArchiveSession(context.Context, string) (int, error) { return 0, nil }
func (m *ingressSessionRepo) DeleteSession(context.Context, string) (int, error)  { return 0, nil }
func (m *ingressSessionRepo) DeleteSessionsByAgentID(context.Context, string) error {
	return nil
}
func (m *ingressSessionRepo) CountMessagesBySession(context.Context, string) (int, error) {
	return 0, nil
}
func (m *ingressSessionRepo) ListMessagesBySession(context.Context, string, int, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListMessagesAfterTurn(context.Context, string, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListMessagesByStatus(context.Context, string, string, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListMessagesRecent(context.Context, string, int) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListToolInvocationsBySession(context.Context, string, int) ([]biz.ToolInvocationView, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListSkillInvocationsBySession(context.Context, string, int) ([]biz.SkillInvocationView, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListTimelineEventRefsPaged(context.Context, string, biz.TimelineQuery) ([]biz.TimelineEventRef, int, error) {
	return nil, 0, nil
}
func (m *ingressSessionRepo) ListMessagesByIDs(context.Context, string, []string) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListToolInvocationsByIDs(context.Context, string, []string) ([]biz.ToolInvocationView, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ListSkillInvocationsByIDs(context.Context, string, []string) ([]biz.SkillInvocationView, error) {
	return nil, nil
}
func (m *ingressSessionRepo) LookupAgentDisplayNames(context.Context, []string) (map[string]string, error) {
	return map[string]string{}, nil
}
func (m *ingressSessionRepo) AppendChatTurn(context.Context, string, biz.ChatMessage, biz.ChatMessage) error {
	return nil
}
func (m *ingressSessionRepo) SearchMessages(context.Context, biz.MessageSearchQuery) (biz.MessageSearchResult, error) {
	return biz.MessageSearchResult{}, nil
}
func (m *ingressSessionRepo) AppendChatMessage(context.Context, string, biz.ChatMessage, bool) error {
	return nil
}
func (m *ingressSessionRepo) UpdateChatMessageStatus(context.Context, string, string, string, string) error {
	return nil
}
func (m *ingressSessionRepo) UpdateMessageFeedbackJSON(context.Context, string, string, string, string) error {
	return nil
}
func (m *ingressSessionRepo) UpsertChatActivityMessage(context.Context, string, biz.ChatMessage) (bool, error) {
	return false, nil
}
func (m *ingressSessionRepo) UpdateRunnerSnapshotJSON(context.Context, string, string) error {
	return nil
}
func (m *ingressSessionRepo) UpdateSessionContextFromLLMUsage(context.Context, string, int, int, int) error {
	return nil
}
func (m *ingressSessionRepo) UpdateSessionContextAfterCompression(context.Context, string, int, int) error {
	return nil
}
func (m *ingressSessionRepo) InsertSessionSummary(context.Context, biz.SessionSummary) error {
	return nil
}
func (m *ingressSessionRepo) MaxSessionSummaryToTurn(context.Context, string) (int, error) {
	return 0, nil
}
func (m *ingressSessionRepo) ListSessionSummaries(context.Context, string) ([]biz.SessionSummary, error) {
	return nil, nil
}
func (m *ingressSessionRepo) LatestSessionSummaryTime(context.Context, string) (string, error) {
	return "", nil
}
func (m *ingressSessionRepo) UpdateSessionListSummary(context.Context, string, string) error {
	return nil
}
func (m *ingressSessionRepo) GetSessionState(context.Context, string) (map[string]string, error) {
	return nil, nil
}
func (m *ingressSessionRepo) SaveSessionState(context.Context, string, map[string]string) error {
	return nil
}
func (m *ingressSessionRepo) PatchSessionState(context.Context, string, map[string]string, []string) error {
	return nil
}
func (m *ingressSessionRepo) CreateSessionTurn(context.Context, biz.SessionTurn) (biz.SessionTurn, error) {
	return biz.SessionTurn{}, nil
}
func (m *ingressSessionRepo) UpdateSessionTurn(context.Context, string, biz.SessionTurnUpdateFields) (biz.SessionTurn, error) {
	return biz.SessionTurn{}, nil
}
func (m *ingressSessionRepo) ListSessionTurns(context.Context, string, int, int) (biz.SessionTurnListResult, error) {
	return biz.SessionTurnListResult{}, nil
}
func (m *ingressSessionRepo) GetSessionTurn(context.Context, string) (biz.SessionTurn, error) {
	return biz.SessionTurn{}, nil
}
func (m *ingressSessionRepo) IncrementInvocationCounts(context.Context, string, int, int, int) error {
	return nil
}
func (m *ingressSessionRepo) ApplyMetricsDelta(context.Context, *sessstatus.SessionMetricsDelta) error {
	return nil
}
func (m *ingressSessionRepo) ListSessionsByIDs(_ context.Context, ids []string) ([]biz.Session, error) {
	out := make([]biz.Session, 0, len(ids))
	for _, id := range ids {
		if s, ok := m.sessions[id]; ok {
			out = append(out, s)
		}
	}
	return out, nil
}
func (m *ingressSessionRepo) ListSessionsForBatch(context.Context, biz.SessionSearchQuery) ([]biz.Session, error) {
	return nil, nil
}
func (m *ingressSessionRepo) ArchiveSessionsByIDs(context.Context, []string) (int, []string, error) {
	return 0, nil, nil
}
func (m *ingressSessionRepo) DeleteSessionsByIDs(context.Context, []string) (int, []string, error) {
	return 0, nil, nil
}
func (m *ingressSessionRepo) PinSession(_ context.Context, id string) (biz.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return biz.Session{}, sql.ErrNoRows
}
func (m *ingressSessionRepo) UnpinSession(_ context.Context, id string) (biz.Session, error) {
	if s, ok := m.sessions[id]; ok {
		return s, nil
	}
	return biz.Session{}, sql.ErrNoRows
}
func (m *ingressSessionRepo) BumpSessionRevision(context.Context, string) (int64, error) {
	return 1, nil
}
func (m *ingressSessionRepo) GetSessionRevision(context.Context, string) (int64, error) {
	return 0, nil
}
func (m *ingressSessionRepo) ListMessagesAfterRevision(context.Context, string, int64) ([]biz.ChatMessage, error) {
	return nil, nil
}
func (m *ingressSessionRepo) TryIncrementCompressVersion(context.Context, string) (int64, error) {
	return 0, nil
}
func (m *ingressSessionRepo) CompressSessionInTx(ctx context.Context, _ string, fn func(context.Context) error) error {
	return fn(ctx)
}
func (m *ingressSessionRepo) SessionSummaryExists(context.Context, string, int, int) (bool, error) {
	return false, nil
}
func (m *ingressSessionRepo) ListByParentSessionID(_ context.Context, _ string) ([]biz.Session, error) {
	return nil, nil
}

type ingressAgentRepo struct {
	id string
}

func (s ingressAgentRepo) SearchAgents(context.Context, biz.AgentListQuery) (biz.AgentListResult, error) {
	return biz.AgentListResult{}, nil
}
func (s ingressAgentRepo) GetAgentByID(_ context.Context, id string) (biz.Agent, error) {
	return biz.Agent{ID: s.id, AgentKey: "agent-key"}, nil
}
func (s ingressAgentRepo) GetAgentByAgentKey(context.Context, string) (biz.Agent, error) {
	return biz.Agent{}, sql.ErrNoRows
}
func (s ingressAgentRepo) CreateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (s ingressAgentRepo) UpdateAgent(context.Context, biz.Agent) (biz.Agent, error) {
	return biz.Agent{}, nil
}
func (s ingressAgentRepo) DeleteAgent(context.Context, string) error { return nil }
func (s ingressAgentRepo) GetAgentRuntimeSettings(context.Context, string) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}
func (s ingressAgentRepo) UpsertAgentRuntimeSettings(context.Context, biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return biz.AgentRuntimeSettings{}, nil
}
func (s ingressAgentRepo) ListAgentPromptFiles(context.Context, string) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (s ingressAgentRepo) ReplaceAgentPromptFiles(context.Context, string, []biz.AgentPromptFile) ([]biz.AgentPromptFile, error) {
	return nil, nil
}
func (s ingressAgentRepo) CreateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (s ingressAgentRepo) UpdateAgentPromptFile(context.Context, biz.AgentPromptFile) (biz.AgentPromptFile, error) {
	return biz.AgentPromptFile{}, nil
}
func (s ingressAgentRepo) DeleteAgentPromptFile(context.Context, string, string) error { return nil }
func (s ingressAgentRepo) ListExtrasForAgents(context.Context, []string) (map[string]biz.AgentListExtras, error) {
	return map[string]biz.AgentListExtras{}, nil
}
func (s ingressAgentRepo) ListAgentCreators(context.Context) ([]biz.AgentCreator, error) {
	return nil, nil
}
func (s ingressAgentRepo) ReorderAgents(context.Context, []string) error { return nil }
func (s ingressAgentRepo) ExecInTx(ctx context.Context, fn func(context.Context) error) error {
	return fn(ctx)
}

func TestEnsureChannelSessionRebindsStalePeerBind(t *testing.T) {
	const (
		channelID = "ch-feishu"
		peerKey   = "ou_test"
		agentID   = "agent-1"
		staleID   = "sess-deleted"
	)
	peerRepo := &ingressPeerSessionRepo{
		byKey: map[string]biz.ChannelPeerSession{
			peerMapKey(channelID, peerKey): {
				ID:        "bind-1",
				ChannelID: channelID,
				PeerKey:   peerKey,
				SessionID: staleID,
			},
		},
	}
	sessRepo := &ingressSessionRepo{sessions: map[string]biz.Session{}}
	agents := ingressAgentRepo{id: agentID}
	sessions := biz.NewSessionUsecase(sessRepo, biz.NewSessionAgentLookup(agents), nil, nil, nil, nil, nil, nil)
	h := &ChannelIngress{
		channels: biz.NewChannelUsecase(nil, peerRepo, nil, agents, nil, nil, nil),
		sessions: sessions,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{
		ID:         channelID,
		Key:        "feishu_main",
		ConfigJSON: `{"type":"feishu","routing":{"default_agent_id":"` + agentID + `"}}`,
	}
	input, err := h.prepareChannelChatRequest(context.Background(), ch, "feishu", peerKey, peerKey, "hello", false)
	if err != nil {
		t.Fatal(err)
	}
	got := strings.TrimSpace(input.SessionID)
	if got == "" || got == staleID {
		t.Fatalf("session id = %q, want new session (not stale %q)", got, staleID)
	}
	if sessRepo.created != 1 {
		t.Fatalf("created sessions = %d, want 1", sessRepo.created)
	}
	bind, err := peerRepo.GetByChannelAndPeer(context.Background(), channelID, peerKey)
	if err != nil {
		t.Fatal(err)
	}
	if bind.SessionID != got {
		t.Fatalf("peer bind session = %q, want %q", bind.SessionID, got)
	}
}

func TestEnsureChannelSessionReusesLivePeerBind(t *testing.T) {
	const (
		channelID = "ch-feishu"
		peerKey   = "ou_live"
		agentID   = "agent-1"
		liveID    = "sess-live"
	)
	peerRepo := &ingressPeerSessionRepo{
		byKey: map[string]biz.ChannelPeerSession{
			peerMapKey(channelID, peerKey): {
				ID:        "bind-1",
				ChannelID: channelID,
				PeerKey:   peerKey,
				SessionID: liveID,
			},
		},
	}
	sessRepo := &ingressSessionRepo{
		sessions: map[string]biz.Session{
			liveID: {ID: liveID, AgentID: agentID, OwnerType: "agent"},
		},
	}
	agents := ingressAgentRepo{id: agentID}
	sessions := biz.NewSessionUsecase(sessRepo, biz.NewSessionAgentLookup(agents), nil, nil, nil, nil, nil, nil)
	h := &ChannelIngress{
		channels: biz.NewChannelUsecase(nil, peerRepo, nil, agents, nil, nil, nil),
		sessions: sessions,
		lg:       loggateway.NewNoop(),
	}
	ch := biz.Channel{
		ID:         channelID,
		Key:        "feishu_main",
		ConfigJSON: `{"type":"feishu","routing":{"default_agent_id":"` + agentID + `"}}`,
	}
	input, err := h.prepareChannelChatRequest(context.Background(), ch, "feishu", peerKey, peerKey, "hello", false)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(input.SessionID); got != liveID {
		t.Fatalf("session id = %q, want %q", got, liveID)
	}
	if sessRepo.created != 0 {
		t.Fatalf("created sessions = %d, want 0", sessRepo.created)
	}
}
