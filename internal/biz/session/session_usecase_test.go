package session

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type mockSessionRepo struct {
	searchSessionsFn   func(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
	createSessionFn    func(ctx context.Context, s Session) (Session, error)
	searchMessagesFn   func(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error)
	getSessionByIDFn   func(ctx context.Context, id string) (Session, error)
}

func (m *mockSessionRepo) SearchSessions(ctx context.Context, q SessionSearchQuery) (SessionListResult, error) {
	if m.searchSessionsFn != nil {
		return m.searchSessionsFn(ctx, q)
	}
	return SessionListResult{}, nil
}

func (m *mockSessionRepo) GetSessionByID(ctx context.Context, id string) (Session, error) {
	if m.getSessionByIDFn != nil {
		return m.getSessionByIDFn(ctx, id)
	}
	return Session{}, nil
}

func (m *mockSessionRepo) GetSessionRevision(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockSessionRepo) ListSessionsForBatch(_ context.Context, _ SessionSearchQuery) ([]Session, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListSessionsByIDs(_ context.Context, _ []string) ([]Session, error) {
	return nil, nil
}

func (m *mockSessionRepo) CreateSession(ctx context.Context, s Session) (Session, error) {
	if m.createSessionFn != nil {
		return m.createSessionFn(ctx, s)
	}
	return s, nil
}

func (m *mockSessionRepo) UpdateSessionTitle(_ context.Context, _ string, _ string) (Session, error) {
	return Session{}, nil
}

func (m *mockSessionRepo) UpdateSession(_ context.Context, _ string, _ SessionUpdateFields) (Session, error) {
	return Session{}, nil
}

func (m *mockSessionRepo) RestoreSession(_ context.Context, _ string) (Session, error) {
	return Session{}, nil
}

func (m *mockSessionRepo) BumpSessionRevision(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockSessionRepo) ArchiveSession(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockSessionRepo) DeleteSession(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockSessionRepo) DeleteSessionsByAgentID(_ context.Context, _ string) error {
	return nil
}

func (m *mockSessionRepo) PinSession(_ context.Context, _ string) (Session, error) {
	return Session{}, nil
}

func (m *mockSessionRepo) UnpinSession(_ context.Context, _ string) (Session, error) {
	return Session{}, nil
}

func (m *mockSessionRepo) ArchiveSessionsByIDs(_ context.Context, _ []string) (int, []string, error) {
	return 0, nil, nil
}

func (m *mockSessionRepo) DeleteSessionsByIDs(_ context.Context, _ []string) (int, []string, error) {
	return 0, nil, nil
}

func (m *mockSessionRepo) CountMessagesBySession(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockSessionRepo) ListMessagesBySession(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListMessagesAfterTurn(_ context.Context, _ string, _ int) ([]ChatMessage, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListMessagesRecent(_ context.Context, _ string, _ int) ([]ChatMessage, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListMessagesByIDs(_ context.Context, _ string, _ []string) ([]ChatMessage, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListMessagesByStatus(_ context.Context, _, _ string, _ int) ([]ChatMessage, error) {
	return nil, nil
}

func (m *mockSessionRepo) SearchMessages(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error) {
	if m.searchMessagesFn != nil {
		return m.searchMessagesFn(ctx, q)
	}
	return MessageSearchResult{}, nil
}

func (m *mockSessionRepo) ListMessagesAfterRevision(_ context.Context, _ string, _ int64) ([]ChatMessage, error) {
	return nil, nil
}

func (m *mockSessionRepo) AppendChatTurn(_ context.Context, _ string, _, _ ChatMessage) error {
	return nil
}

func (m *mockSessionRepo) AppendChatMessage(_ context.Context, _ string, _ ChatMessage, _ bool) error {
	return nil
}

func (m *mockSessionRepo) UpdateMessageFeedbackJSON(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (m *mockSessionRepo) UpsertChatActivityMessage(_ context.Context, _ string, _ ChatMessage) error {
	return nil
}

func (m *mockSessionRepo) UpdateChatMessageStatus(_ context.Context, _, _, _, _ string) error {
	return nil
}

func (m *mockSessionRepo) ListTimelineEventRefsPaged(_ context.Context, _ string, _ TimelineQuery) ([]TimelineEventRef, int, error) {
	return nil, 0, nil
}

func (m *mockSessionRepo) ListToolInvocationsByIDs(_ context.Context, _ string, _ []string) ([]ToolInvocationView, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListSkillInvocationsByIDs(_ context.Context, _ string, _ []string) ([]SkillInvocationView, error) {
	return nil, nil
}

func (m *mockSessionRepo) LookupAgentDisplayNames(_ context.Context, _ []string) (map[string]string, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListToolInvocationsBySession(_ context.Context, _ string, _ int) ([]ToolInvocationView, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListSkillInvocationsBySession(_ context.Context, _ string, _ int) ([]SkillInvocationView, error) {
	return nil, nil
}

func (m *mockSessionRepo) MaxSessionSummaryToTurn(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func (m *mockSessionRepo) ListSessionSummaries(_ context.Context, _ string) ([]SessionSummary, error) {
	return nil, nil
}

func (m *mockSessionRepo) LatestSessionSummaryTime(_ context.Context, _ string) (string, error) {
	return "", nil
}

func (m *mockSessionRepo) InsertSessionSummary(_ context.Context, _ SessionSummary) error {
	return nil
}

func (m *mockSessionRepo) UpdateSessionListSummary(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockSessionRepo) SessionSummaryExists(_ context.Context, _ string, _, _ int) (bool, error) {
	return false, nil
}

func (m *mockSessionRepo) GetSessionState(_ context.Context, _ string) (map[string]string, error) {
	return nil, nil
}

func (m *mockSessionRepo) SaveSessionState(_ context.Context, _ string, _ map[string]string) error {
	return nil
}

func (m *mockSessionRepo) PatchSessionState(_ context.Context, _ string, _ map[string]string, _ []string) error {
	return nil
}

func (m *mockSessionRepo) CreateSessionTurn(_ context.Context, _ SessionTurn) (SessionTurn, error) {
	return SessionTurn{}, nil
}

func (m *mockSessionRepo) UpdateSessionTurn(_ context.Context, _ string, _ SessionTurnUpdateFields) (SessionTurn, error) {
	return SessionTurn{}, nil
}

func (m *mockSessionRepo) ListSessionTurns(_ context.Context, _ string, _, _ int) (SessionTurnListResult, error) {
	return SessionTurnListResult{}, nil
}

func (m *mockSessionRepo) GetSessionTurn(_ context.Context, _ string) (SessionTurn, error) {
	return SessionTurn{}, nil
}

func (m *mockSessionRepo) UpdateRunnerSnapshotJSON(_ context.Context, _, _ string) error {
	return nil
}

func (m *mockSessionRepo) UpdateSessionContextFromLLMUsage(_ context.Context, _ string, _, _, _ int) error {
	return nil
}

func (m *mockSessionRepo) UpdateSessionContextAfterCompression(_ context.Context, _ string, _, _ int) error {
	return nil
}

func (m *mockSessionRepo) IncrementInvocationCounts(_ context.Context, _ string, _, _, _ int) error {
	return nil
}

func (m *mockSessionRepo) TryIncrementCompressVersion(_ context.Context, _ string) (int64, error) {
	return 0, nil
}

func (m *mockSessionRepo) CompressSessionInTx(_ context.Context, _ string, _ func(ctx context.Context) error) error {
	return nil
}

func (m *mockSessionRepo) ListByParentSessionID(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}

func (m *mockSessionRepo) TransitionSessionStatus(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

type mockAgentLookup struct {
	getAgentByIDFn func(ctx context.Context, id string) (struct{}, error)
}

func (m *mockAgentLookup) GetAgentByID(ctx context.Context, id string) (struct{}, error) {
	if m.getAgentByIDFn != nil {
		return m.getAgentByIDFn(ctx, id)
	}
	return struct{}{}, nil
}

type mockTeamLookup struct {
	getTeamByIDFn func(ctx context.Context, id string) (struct{}, error)
}

func (m *mockTeamLookup) GetTeamByID(ctx context.Context, id string) (struct{}, error) {
	if m.getTeamByIDFn != nil {
		return m.getTeamByIDFn(ctx, id)
	}
	return struct{}{}, nil
}

type mockParticipantRepo struct{}

func (m *mockParticipantRepo) SyncFromSession(_ context.Context, _ Session, _ []ChatMessage) error {
	return nil
}

func (m *mockParticipantRepo) ListBySession(_ context.Context, _ string) ([]SessionParticipant, error) {
	return nil, nil
}

func newTestUsecase(repo *mockSessionRepo, agents *mockAgentLookup, teams *mockTeamLookup) *SessionUsecase {
	return NewSessionUsecase(repo, agents, teams, nil, &mockParticipantRepo{})
}

func assertBadRequest(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e := kerrors.FromError(err)
	if e.Code != 400 {
		t.Fatalf("expected code 400, got %d", e.Code)
	}
	if e.Reason != "SESSION" {
		t.Fatalf("expected reason SESSION, got %s", e.Reason)
	}
	if wantMsg != "" && e.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, e.Message)
	}
}

func assertNotFound(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e := kerrors.FromError(err)
	if e.Code != 404 {
		t.Fatalf("expected code 404, got %d", e.Code)
	}
	if e.Reason != "SESSION" {
		t.Fatalf("expected reason SESSION, got %s", e.Reason)
	}
	if wantMsg != "" && e.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, e.Message)
	}
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name       string
		input      Session
		agentFn    func(ctx context.Context, id string) (struct{}, error)
		teamFn     func(ctx context.Context, id string) (struct{}, error)
		createFn   func(ctx context.Context, s Session) (Session, error)
		wantErr    bool
		wantStatus int32
		wantMsg    string
		checkResult func(t *testing.T, got Session)
	}{
		{
			name: "valid agent session creation",
			input: Session{
				OwnerType:  "agent",
				AgentID:    "agent-1",
				Title:      "Test Session",
				Status:     "active",
				Visibility: "private",
				DialogMode: "single",
			},
			agentFn: func(_ context.Context, _ string) (struct{}, error) {
				return struct{}{}, nil
			},
			createFn: func(_ context.Context, s Session) (Session, error) {
				return s, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got Session) {
				if got.ID == "" {
					t.Fatal("expected ID to be set")
				}
				if got.OwnerType != "agent" {
					t.Fatalf("expected owner_type agent, got %s", got.OwnerType)
				}
				if got.AgentID != "agent-1" {
					t.Fatalf("expected agent_id agent-1, got %s", got.AgentID)
				}
				if got.Status != "active" {
					t.Fatalf("expected status active, got %s", got.Status)
				}
				if got.Visibility != "private" {
					t.Fatalf("expected visibility private, got %s", got.Visibility)
				}
				if got.DialogMode != "single" {
					t.Fatalf("expected dialog_mode single, got %s", got.DialogMode)
				}
			},
		},
		{
			name: "empty agent_id returns error",
			input: Session{
				OwnerType: "agent",
				AgentID:   "",
			},
			wantErr:    true,
			wantStatus: 400,
			wantMsg:    "agent_id is required",
		},
		{
			name: "empty owner_type defaults to agent",
			input: Session{
				OwnerType:  "",
				AgentID:    "agent-2",
				Status:     "active",
				Visibility: "private",
				DialogMode: "single",
			},
			agentFn: func(_ context.Context, id string) (struct{}, error) {
				if id != "agent-2" {
					return struct{}{}, sql.ErrNoRows
				}
				return struct{}{}, nil
			},
			createFn: func(_ context.Context, s Session) (Session, error) {
				return s, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got Session) {
				if got.OwnerType != "agent" {
					t.Fatalf("expected owner_type to default to agent, got %s", got.OwnerType)
				}
				if got.ID == "" {
					t.Fatal("expected ID to be set")
				}
				if got.Status != "active" {
					t.Fatalf("expected status active, got %s", got.Status)
				}
				if got.Visibility != "private" {
					t.Fatalf("expected visibility private, got %s", got.Visibility)
				}
				if got.DialogMode != "single" {
					t.Fatalf("expected dialog_mode single, got %s", got.DialogMode)
				}
			},
		},
		{
			name: "invalid owner_type returns error",
			input: Session{
				OwnerType: "invalid",
			},
			wantErr:    true,
			wantStatus: 400,
			wantMsg:    "owner_type must be agent or team",
		},
		{
			name: "agent not found returns not found error",
			input: Session{
				OwnerType: "agent",
				AgentID:   "ghost-agent",
			},
			agentFn: func(_ context.Context, _ string) (struct{}, error) {
				return struct{}{}, sql.ErrNoRows
			},
			wantErr:    true,
			wantStatus: 404,
			wantMsg:    "agent not found",
		},
		{
			name: "team session creation",
			input: Session{
				OwnerType: "team",
				TeamID:    "team-1",
				Status:    "active",
			},
			teamFn: func(_ context.Context, _ string) (struct{}, error) {
				return struct{}{}, nil
			},
			createFn: func(_ context.Context, s Session) (Session, error) {
				return s, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got Session) {
				if got.ID == "" {
					t.Fatal("expected ID to be set")
				}
				if got.OwnerType != "team" {
					t.Fatalf("expected owner_type team, got %s", got.OwnerType)
				}
				if got.TeamID != "team-1" {
					t.Fatalf("expected team_id team-1, got %s", got.TeamID)
				}
			},
		},
		{
			name: "empty team_id returns error",
			input: Session{
				OwnerType: "team",
				TeamID:    "",
			},
			wantErr:    true,
			wantStatus: 400,
			wantMsg:    "team_id is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepo{createSessionFn: tt.createFn}
			agents := &mockAgentLookup{getAgentByIDFn: tt.agentFn}
			teams := &mockTeamLookup{getTeamByIDFn: tt.teamFn}
			uc := newTestUsecase(repo, agents, teams)

			got, err := uc.Create(context.Background(), tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				e := kerrors.FromError(err)
				if e.Code != tt.wantStatus {
					t.Fatalf("expected status %d, got %d", tt.wantStatus, e.Code)
				}
				if e.Message != tt.wantMsg {
					t.Fatalf("expected message %q, got %q", tt.wantMsg, e.Message)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkResult != nil {
				tt.checkResult(t, got)
			}
		})
	}
}

func TestSearch(t *testing.T) {
	tests := []struct {
		name      string
		query     SessionSearchQuery
		searchFn  func(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
		wantErr   bool
		wantLimit int
		wantOffset int
		checkResult func(t *testing.T, got SessionListResult, capturedQ SessionSearchQuery)
	}{
		{
			name: "pagination from page and page_size",
			query: SessionSearchQuery{
				Page:     3,
				PageSize: 10,
			},
			searchFn: func(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Limit: q.Limit, Offset: q.Offset}, nil
			},
			wantErr:    false,
			wantLimit:  10,
			wantOffset: 20,
		},
		{
			name: "page zero resets offset",
			query: SessionSearchQuery{
				Page:     0,
				PageSize: 10,
			},
			searchFn: func(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Limit: q.Limit, Offset: q.Offset}, nil
			},
			wantErr:    false,
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name: "default limit when zero",
			query: SessionSearchQuery{
				Limit: 0,
			},
			searchFn: func(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Limit: q.Limit, Offset: q.Offset}, nil
			},
			wantErr:    false,
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name: "limit capped at 100",
			query: SessionSearchQuery{
				Limit: 200,
			},
			searchFn: func(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Limit: q.Limit, Offset: q.Offset}, nil
			},
			wantErr:    false,
			wantLimit:  20,
			wantOffset: 0,
		},
		{
			name: "negative offset reset to zero",
			query: SessionSearchQuery{
				Limit:  10,
				Offset: -5,
			},
			searchFn: func(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Limit: q.Limit, Offset: q.Offset}, nil
			},
			wantErr:    false,
			wantLimit:  10,
			wantOffset: 0,
		},
		{
			name:  "empty results",
			query: SessionSearchQuery{Limit: 20},
			searchFn: func(_ context.Context, _ SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Items: []Session{}, Total: 0, Limit: 20, Offset: 0}, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got SessionListResult, _ SessionSearchQuery) {
				if got.Total != 0 {
					t.Fatalf("expected total 0, got %d", got.Total)
				}
				if len(got.Items) != 0 {
					t.Fatalf("expected 0 items, got %d", len(got.Items))
				}
			},
		},
		{
			name:  "results with data",
			query: SessionSearchQuery{Limit: 20},
			searchFn: func(_ context.Context, _ SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{
					Items: []Session{
						{ID: "s1", Title: "Session 1"},
						{ID: "s2", Title: "Session 2"},
					},
					Total:  2,
					Limit:  20,
					Offset: 0,
				}, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got SessionListResult, _ SessionSearchQuery) {
				if got.Total != 2 {
					t.Fatalf("expected total 2, got %d", got.Total)
				}
				if len(got.Items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(got.Items))
				}
				if got.Items[0].ID != "s1" {
					t.Fatalf("expected first item id s1, got %s", got.Items[0].ID)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepo{searchSessionsFn: tt.searchFn}
			uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})

			got, err := uc.Search(context.Background(), tt.query)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tt.checkResult != nil {
				tt.checkResult(t, got, SessionSearchQuery{})
			} else {
				if got.Limit != tt.wantLimit {
					t.Fatalf("expected limit %d, got %d", tt.wantLimit, got.Limit)
				}
				if got.Offset != tt.wantOffset {
					t.Fatalf("expected offset %d, got %d", tt.wantOffset, got.Offset)
				}
			}
		})
	}
}

func TestSearchMessages(t *testing.T) {
	tests := []struct {
		name     string
		query    MessageSearchQuery
		searchFn func(ctx context.Context, q MessageSearchQuery) (MessageSearchResult, error)
		wantErr  bool
		wantMsg  string
		checkResult func(t *testing.T, got MessageSearchResult)
	}{
		{
			name:    "empty session_id returns error",
			query:   MessageSearchQuery{SessionID: "", Keyword: "test"},
			wantErr: true,
			wantMsg: "session_id is required",
		},
		{
			name:    "whitespace session_id returns error",
			query:   MessageSearchQuery{SessionID: "  ", Keyword: "test"},
			wantErr: true,
			wantMsg: "session_id is required",
		},
		{
			name:    "empty keyword returns error",
			query:   MessageSearchQuery{SessionID: "s1", Keyword: ""},
			wantErr: true,
			wantMsg: "keyword is required",
		},
		{
			name:    "whitespace keyword returns error",
			query:   MessageSearchQuery{SessionID: "s1", Keyword: "   "},
			wantErr: true,
			wantMsg: "keyword is required",
		},
		{
			name:  "valid search returns results",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "hello", Limit: 10},
			searchFn: func(_ context.Context, q MessageSearchQuery) (MessageSearchResult, error) {
				return MessageSearchResult{
					Items: []MessageSearchHit{
						{ID: "m1", SessionID: "s1", ContentMarkdown: "hello world", Highlight: "<b>hello</b> world"},
					},
					Total: 1,
				}, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got MessageSearchResult) {
				if got.Total != 1 {
					t.Fatalf("expected total 1, got %d", got.Total)
				}
				if len(got.Items) != 1 {
					t.Fatalf("expected 1 item, got %d", len(got.Items))
				}
				if got.Items[0].ID != "m1" {
					t.Fatalf("expected item id m1, got %s", got.Items[0].ID)
				}
			},
		},
		{
			name:  "limit passed through to repo",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "test", Limit: 50, Offset: 10},
			searchFn: func(_ context.Context, q MessageSearchQuery) (MessageSearchResult, error) {
				if q.Limit != 50 {
					t.Fatalf("expected limit 50 passed through, got %d", q.Limit)
				}
				if q.Offset != 10 {
					t.Fatalf("expected offset 10 passed through, got %d", q.Offset)
				}
				return MessageSearchResult{Total: 0}, nil
			},
			wantErr: false,
		},
		{
			name:  "repo error propagated",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "test"},
			searchFn: func(_ context.Context, _ MessageSearchQuery) (MessageSearchResult, error) {
				return MessageSearchResult{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &mockSessionRepo{searchMessagesFn: tt.searchFn}
			uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})

			got, err := uc.SearchMessages(context.Background(), tt.query)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					assertBadRequest(t, err, tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkResult != nil {
				tt.checkResult(t, got)
			}
		})
	}
}
