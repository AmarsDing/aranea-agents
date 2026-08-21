package session

import (
	"aranea-agents/internal/biz/shared"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"testing"
)

type mockSessionRepo struct {
	searchSessionsFn func(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
	createSessionFn  func(ctx context.Context, s Session) (Session, error)
	getSessionByIDFn func(ctx context.Context, id string) (Session, error)
	updateSessionFn  func() (Session, error)
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

func (m *mockSessionRepo) ListActiveAgentUserKeys(_ context.Context, _ int) ([]AgentUserKey, error) {
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
	if m.updateSessionFn != nil {
		return m.updateSessionFn()
	}
	return Session{}, nil
}

func (m *mockSessionRepo) RestoreSession(_ context.Context, _ string) (Session, error) {
	return Session{}, nil
}

func (m *mockSessionRepo) UpdateSessionMetadataKey(_ context.Context, _, _, _ string) error {
	return nil
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

func (m *mockSessionRepo) SearchMessages(_ context.Context, _ MessageSearchQuery) (MessageSearchResult, error) {
	// Phase 1c-3: searchMessagesFn removed — production code reads via
	// ActivityMessageReader (backed by ActivityLister), not via this method.
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

func (m *mockSessionRepo) UpsertChatActivityMessage(_ context.Context, _ string, _ ChatMessage) (bool, error) {
	return false, nil
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

func (m *mockSessionRepo) DeleteSessionSummaries(_ context.Context, _ string) error {
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

func (m *mockSessionRepo) ApplyMetricsDelta(_ context.Context, _ *SessionMetricsDelta) error {
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

func (m *mockSessionRepo) GetSessionTree(_ context.Context, _ string) (*SessionTree, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListChildSessions(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}

func (m *mockSessionRepo) ListTeamAgentSessions(_ context.Context, _ string) ([]Session, error) {
	return nil, nil
}

func (m *mockSessionRepo) TransitionSessionStatus(_ context.Context, _ string, _ string, _ string, _ string) error {
	return nil
}

// ListBySession implements ActivityLister for mockSessionRepo (no-op default).
// Phase 1c-3: returns empty so ActivityMessageReader yields empty results.
func (m *mockSessionRepo) ListBySession(_ context.Context, _ string) ([]ActivityEntry, error) {
	return nil, nil
}

// ListBySessionTurn implements ActivityLister for mockSessionRepo (no-op default).
func (m *mockSessionRepo) ListBySessionTurn(_ context.Context, _, _ string) ([]ActivityEntry, error) {
	return nil, nil
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
	return NewSessionUsecase(repo, agents, teams, nil, &mockParticipantRepo{}, nil, NewSessionMetricsUsecase(repo, nil, nil), nil, repo, loggateway.NewNoop(), nil)
}

func assertBadRequest(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if e.Code != apierror.CodeBadRequest {
		t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, e.Code)
	}
	if e.Domain != "SESSION" {
		t.Fatalf("expected domain SESSION, got %s", e.Domain)
	}
	if wantMsg != "" && e.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, e.Message)
	}
}

func assertConflict(t *testing.T, err error, wantMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	e, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if e.Code != apierror.CodeConflict {
		t.Fatalf("expected code %s, got %s", apierror.CodeConflict, e.Code)
	}
	if e.Domain != "SESSION" {
		t.Fatalf("expected domain SESSION, got %s", e.Domain)
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
	e, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if e.Code != apierror.CodeNotFound {
		t.Fatalf("expected code %s, got %s", apierror.CodeNotFound, e.Code)
	}
	if e.Domain != "SESSION" {
		t.Fatalf("expected domain SESSION, got %s", e.Domain)
	}
	if wantMsg != "" && e.Message != wantMsg {
		t.Fatalf("expected message %q, got %q", wantMsg, e.Message)
	}
}

// 00:52 会话取证：running→running 等同状态转换产生大量 Conflict 告警噪音。
// 同状态转换语义上是幂等 no-op（目标状态已达成），不应报错、不应写库、
// 不应发布状态变更事件。
func TestTransitionStatus_SameStatusIdempotentNoop(t *testing.T) {
	writeCalled := false
	repo := &mockSessionRepo{
		getSessionByIDFn: func(_ context.Context, id string) (Session, error) {
			return Session{ID: id, Status: string(SessionStatusRunning)}, nil
		},
		updateSessionFn: func() (Session, error) {
			writeCalled = true
			return Session{}, nil
		},
	}
	uc := newTestUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{})

	if err := uc.TransitionStatus(context.Background(), "s1", SessionStatusRunning, ""); err != nil {
		t.Fatalf("same-status transition must be idempotent no-op, got error: %v", err)
	}
	if writeCalled {
		t.Fatal("same-status transition must not write to the repository")
	}
}

func TestCreate(t *testing.T) {
	tests := []struct {
		name        string
		input       Session
		agentFn     func(ctx context.Context, id string) (struct{}, error)
		teamFn      func(ctx context.Context, id string) (struct{}, error)
		createFn    func(ctx context.Context, s Session) (Session, error)
		wantErr     bool
		wantCode    apierror.Code
		wantMsg     string
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
			wantErr:  true,
			wantCode: apierror.CodeBadRequest,
			wantMsg:  "agent_id is required",
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
					return struct{}{}, shared.ErrNotFound
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
			wantErr:  true,
			wantCode: apierror.CodeBadRequest,
			wantMsg:  "owner_type must be agent or team",
		},
		{
			name: "agent not found returns not found error",
			input: Session{
				OwnerType: "agent",
				AgentID:   "ghost-agent",
			},
			agentFn: func(_ context.Context, _ string) (struct{}, error) {
				return struct{}{}, shared.ErrNotFound
			},
			wantErr:  true,
			wantCode: apierror.CodeNotFound,
			wantMsg:  "agent not found",
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
			wantErr:  true,
			wantCode: apierror.CodeBadRequest,
			wantMsg:  "team_id is required",
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
				e, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror, got %T", err)
				}
				if e.Code != tt.wantCode {
					t.Fatalf("expected code %s, got %s", tt.wantCode, e.Code)
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

// TestCreateBackfillsUserID verifies the ownership backfill: sessions created
// without an explicit UserID must inherit the authenticated principal from ctx
// (the confirm gate compares session.UserID against ctxuser.FromContext, so an
// empty owner makes every tool confirmation fail with 403).
func TestCreateBackfillsUserID(t *testing.T) {
	newUc := func() (*SessionUsecase, *mockSessionRepo) {
		repo := &mockSessionRepo{}
		agents := &mockAgentLookup{getAgentByIDFn: func(_ context.Context, _ string) (struct{}, error) {
			return struct{}{}, nil
		}}
		return newTestUsecase(repo, agents, &mockTeamLookup{}), repo
	}
	baseInput := Session{OwnerType: "agent", AgentID: "agent-1", Title: "t"}

	t.Run("empty UserID inherits auth principal", func(t *testing.T) {
		uc, _ := newUc()
		ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 1, Access: "admin"})
		got, err := uc.Create(ctx, baseInput)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.UserID != "1" {
			t.Fatalf("expected UserID backfilled to %q, got %q", "1", got.UserID)
		}
	})

	t.Run("empty UserID without auth stays empty", func(t *testing.T) {
		uc, _ := newUc()
		got, err := uc.Create(context.Background(), baseInput)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.UserID != "" {
			t.Fatalf("expected UserID to stay empty without auth, got %q", got.UserID)
		}
	})

	t.Run("explicit UserID is not overwritten", func(t *testing.T) {
		uc, _ := newUc()
		ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 1, Access: "admin"})
		in := baseInput
		in.UserID = "42"
		got, err := uc.Create(ctx, in)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.UserID != "42" {
			t.Fatalf("expected explicit UserID preserved, got %q", got.UserID)
		}
	})
}

func TestSearch(t *testing.T) {
	tests := []struct {
		name        string
		query       SessionSearchQuery
		searchFn    func(ctx context.Context, q SessionSearchQuery) (SessionListResult, error)
		wantErr     bool
		wantLimit   int
		wantOffset  int
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
			name: "limit within max preserved",
			query: SessionSearchQuery{
				Limit: 200,
			},
			searchFn: func(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Limit: q.Limit, Offset: q.Offset}, nil
			},
			wantErr:    false,
			wantLimit:  200,
			wantOffset: 0,
		},
		{
			name: "limit above max clamped to max",
			query: SessionSearchQuery{
				Limit: 9999,
			},
			searchFn: func(_ context.Context, q SessionSearchQuery) (SessionListResult, error) {
				return SessionListResult{Limit: q.Limit, Offset: q.Offset}, nil
			},
			wantErr:    false,
			wantLimit:  MaxSessionSearchLimit,
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
	// Phase 1c-3: SearchMessages now backs onto ActivityLister.ListBySession
	// (via ActivityMessageReader), so mockSessionRepo.searchMessagesFn is dead.
	// Mock listMessagesBySessionFn on testRepo instead.
	tests := []struct {
		name        string
		query       MessageSearchQuery
		listMsgFn   func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error)
		wantErr     bool
		wantMsg     string
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
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{
					{ID: "m1", Role: "user", ContentMarkdown: "hello world"},
					{ID: "m2", Role: "user", ContentMarkdown: "goodbye"},
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
			name:  "limit applied to results",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "test", Limit: 2},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{
					{ID: "m1", Role: "user", ContentMarkdown: "test 1"},
					{ID: "m2", Role: "user", ContentMarkdown: "test 2"},
					{ID: "m3", Role: "user", ContentMarkdown: "test 3"},
				}, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got MessageSearchResult) {
				if len(got.Items) != 2 {
					t.Fatalf("expected 2 items after limit, got %d", len(got.Items))
				}
			},
		},
		{
			name:  "repo error propagated",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "test"},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{}
			if tt.listMsgFn != nil {
				repo.listMessagesBySessionFn = tt.listMsgFn
			}
			uc := newTestUc(repo)

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
