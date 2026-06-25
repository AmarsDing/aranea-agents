package session

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/apierror"
)

func (r *testRepo) ListTimelineEventRefsPaged(ctx context.Context, sessionID string, q TimelineQuery) ([]TimelineEventRef, int, error) {
	if r.listTimelineEventRefsPagedFn != nil {
		return r.listTimelineEventRefsPagedFn(ctx, sessionID, q)
	}
	return r.mockSessionRepo.ListTimelineEventRefsPaged(ctx, sessionID, q)
}

func (r *testRepo) ListMessagesBySession(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error) {
	if r.listMessagesBySessionFn != nil {
		return r.listMessagesBySessionFn(ctx, sessionID, limit, offset)
	}
	return r.mockSessionRepo.ListMessagesBySession(ctx, sessionID, limit, offset)
}

func (r *testRepo) ListToolInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]ToolInvocationView, error) {
	if r.listToolInvocationsByIDsFn != nil {
		return r.listToolInvocationsByIDsFn(ctx, sessionID, ids)
	}
	return r.mockSessionRepo.ListToolInvocationsByIDs(ctx, sessionID, ids)
}

func (r *testRepo) ListSkillInvocationsByIDs(ctx context.Context, sessionID string, ids []string) ([]SkillInvocationView, error) {
	if r.listSkillInvocationsByIDsFn != nil {
		return r.listSkillInvocationsByIDsFn(ctx, sessionID, ids)
	}
	return r.mockSessionRepo.ListSkillInvocationsByIDs(ctx, sessionID, ids)
}

func (r *testRepo) LookupAgentDisplayNames(ctx context.Context, agentIDs []string) (map[string]string, error) {
	if r.lookupAgentDisplayNamesFn != nil {
		return r.lookupAgentDisplayNamesFn(ctx, agentIDs)
	}
	return r.mockSessionRepo.LookupAgentDisplayNames(ctx, agentIDs)
}

func (r *testRepo) CreateSessionTurn(ctx context.Context, turn SessionTurn) (SessionTurn, error) {
	if r.createSessionTurnFn != nil {
		return r.createSessionTurnFn(ctx, turn)
	}
	return r.mockSessionRepo.CreateSessionTurn(ctx, turn)
}

func (r *testRepo) ListSessionTurns(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error) {
	if r.listSessionTurnsFn != nil {
		return r.listSessionTurnsFn(ctx, sessionID, limit, offset)
	}
	return r.mockSessionRepo.ListSessionTurns(ctx, sessionID, limit, offset)
}

func (r *testRepo) IncrementInvocationCounts(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error {
	if r.incrementInvocationCountsFn != nil {
		return r.incrementInvocationCountsFn(ctx, sessionID, toolDelta, mcpDelta, skillDelta)
	}
	return r.mockSessionRepo.IncrementInvocationCounts(ctx, sessionID, toolDelta, mcpDelta, skillDelta)
}

func (r *testRepo) InsertSessionSummary(ctx context.Context, row SessionSummary) error {
	if r.insertSessionSummaryFn != nil {
		return r.insertSessionSummaryFn(ctx, row)
	}
	return r.mockSessionRepo.InsertSessionSummary(ctx, row)
}

func (r *testRepo) ListSessionSummaries(ctx context.Context, sessionID string) ([]SessionSummary, error) {
	if r.listSessionSummariesFn != nil {
		return r.listSessionSummariesFn(ctx, sessionID)
	}
	return r.mockSessionRepo.ListSessionSummaries(ctx, sessionID)
}

func (r *testRepo) GetSessionState(ctx context.Context, sessionID string) (map[string]string, error) {
	if r.getSessionStateFn != nil {
		return r.getSessionStateFn(ctx, sessionID)
	}
	return r.mockSessionRepo.GetSessionState(ctx, sessionID)
}

func (r *testRepo) SaveSessionState(ctx context.Context, sessionID string, state map[string]string) error {
	if r.saveSessionStateFn != nil {
		return r.saveSessionStateFn(ctx, sessionID, state)
	}
	return r.mockSessionRepo.SaveSessionState(ctx, sessionID, state)
}

func (r *testRepo) PatchSessionState(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error {
	if r.patchSessionStateFn != nil {
		return r.patchSessionStateFn(ctx, sessionID, sets, deletes)
	}
	return r.mockSessionRepo.PatchSessionState(ctx, sessionID, sets, deletes)
}

func TestTimeline(t *testing.T) {
	// Phase 1c-3: countFn and listMsgIDsFn are dead (count derived from list;
	// ListMessagesByIDs filters from ListBySession). Mock listMsgFn instead.
	tests := []struct {
		name        string
		id          string
		query       TimelineQuery
		getFn       func(ctx context.Context, id string) (Session, error)
		listMsgFn   func(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
		listRefsFn  func(ctx context.Context, sessionID string, q TimelineQuery) ([]TimelineEventRef, int, error)
		listToolFn  func(ctx context.Context, sessionID string, ids []string) ([]ToolInvocationView, error)
		listSkillFn func(ctx context.Context, sessionID string, ids []string) ([]SkillInvocationView, error)
		lookupFn    func(ctx context.Context, agentIDs []string) (map[string]string, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got SessionTimeline)
	}{
		{
			name:    "empty session_id returns error",
			id:      "",
			query:   TimelineQuery{KindFilter: "message"},
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:    "whitespace session_id returns error",
			id:      "   ",
			query:   TimelineQuery{KindFilter: "message"},
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:  "valid timeline with messages only filter",
			id:    "sess-1",
			query: TimelineQuery{KindFilter: "message", Limit: 10},
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, MessageCount: 2}, nil
			},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{
					{ID: "m1", Role: "user", ContentMarkdown: "hello", CreatedAt: "2026-01-01T10:00:00Z"},
					{ID: "m2", Role: "assistant", ContentMarkdown: "hi", CreatedAt: "2026-01-01T10:01:00Z"},
				}, nil
			},
			checkResult: func(t *testing.T, got SessionTimeline) {
				if got.SessionID != "sess-1" {
					t.Fatalf("expected session_id sess-1, got %s", got.SessionID)
				}
				if len(got.Items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(got.Items))
				}
				if got.Items[0].Kind != "message" {
					t.Fatalf("expected kind message, got %s", got.Items[0].Kind)
				}
				if got.Summary.MessageCount != 2 {
					t.Fatalf("expected message_count 2, got %d", got.Summary.MessageCount)
				}
			},
		},
		{
			name:  "valid timeline with messages and invocations union",
			id:    "sess-1",
			query: TimelineQuery{Limit: 10},
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id}, nil
			},
			listRefsFn: func(_ context.Context, _ string, _ TimelineQuery) ([]TimelineEventRef, int, error) {
				return []TimelineEventRef{
					{Kind: "message", ID: "m1", OccurredAt: "2026-01-01T10:00:00Z"},
					{Kind: "tool", ID: "t1", OccurredAt: "2026-01-01T10:01:00Z"},
					{Kind: "skill", ID: "s1", OccurredAt: "2026-01-01T10:02:00Z"},
				}, 3, nil
			},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				// ActivityMessageReader.ListMessagesByIDs filters by ID from ListBySession result.
				return []ChatMessage{{ID: "m1", Role: "user", ContentMarkdown: "hello", CreatedAt: "2026-01-01T10:00:00Z"}}, nil
			},
			listToolFn: func(_ context.Context, _ string, ids []string) ([]ToolInvocationView, error) {
				return []ToolInvocationView{{ID: "t1", ToolKey: "search", ToolDisplayName: "Search", Status: "success", StartedAt: "2026-01-01T10:01:00Z"}}, nil
			},
			listSkillFn: func(_ context.Context, _ string, ids []string) ([]SkillInvocationView, error) {
				return []SkillInvocationView{{ID: "s1", SkillName: "Analyze", Status: "ok", StartedAt: "2026-01-01T10:02:00Z"}}, nil
			},
			lookupFn: func(_ context.Context, _ []string) (map[string]string, error) {
				return map[string]string{}, nil
			},
			checkResult: func(t *testing.T, got SessionTimeline) {
				if got.SessionID != "sess-1" {
					t.Fatalf("expected session_id sess-1, got %s", got.SessionID)
				}
				if len(got.Items) != 3 {
					t.Fatalf("expected 3 items, got %d", len(got.Items))
				}
				kinds := make(map[string]int)
				for _, item := range got.Items {
					kinds[item.Kind]++
				}
				if kinds["message"] != 1 {
					t.Fatalf("expected 1 message item, got %d", kinds["message"])
				}
				if kinds["tool"] != 1 {
					t.Fatalf("expected 1 tool item, got %d", kinds["tool"])
				}
				if kinds["skill"] != 1 {
					t.Fatalf("expected 1 skill item, got %d", kinds["skill"])
				}
				if got.Summary.Total != 3 {
					t.Fatalf("expected total 3, got %d", got.Summary.Total)
				}
			},
		},
		{
			name:  "get session error propagated",
			id:    "sess-1",
			query: TimelineQuery{KindFilter: "message"},
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, apierror.NotFound("SESSION", "session not found")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{}
			if tt.getFn != nil {
				repo.getSessionByIDFn = tt.getFn
			}
			if tt.listMsgFn != nil {
				repo.listMessagesBySessionFn = tt.listMsgFn
			}
			if tt.listRefsFn != nil {
				repo.listTimelineEventRefsPagedFn = tt.listRefsFn
			}
			if tt.listToolFn != nil {
				repo.listToolInvocationsByIDsFn = tt.listToolFn
			}
			if tt.listSkillFn != nil {
				repo.listSkillInvocationsByIDsFn = tt.listSkillFn
			}
			if tt.lookupFn != nil {
				repo.lookupAgentDisplayNamesFn = tt.lookupFn
			}
			uc := newTestUc(repo)
			got, err := uc.Timeline(context.Background(), tt.id, tt.query)
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

func TestCreateTurn(t *testing.T) {
	tests := []struct {
		name        string
		turn        SessionTurn
		createFn    func(ctx context.Context, turn SessionTurn) (SessionTurn, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got SessionTurn)
	}{
		{
			name: "valid turn creation with defaults",
			turn: SessionTurn{SessionID: "sess-1", Status: "running"},
			createFn: func(_ context.Context, turn SessionTurn) (SessionTurn, error) {
				if turn.ID == "" {
					t.Fatal("expected ID to be generated")
				}
				if turn.CreatedAt == "" {
					t.Fatal("expected CreatedAt to be set")
				}
				if turn.UpdatedAt == "" {
					t.Fatal("expected UpdatedAt to be set")
				}
				if turn.SessionID != "sess-1" {
					t.Fatalf("expected session_id sess-1, got %s", turn.SessionID)
				}
				return turn, nil
			},
			checkResult: func(t *testing.T, got SessionTurn) {
				if got.ID == "" {
					t.Fatal("expected ID to be set")
				}
				if got.SessionID != "sess-1" {
					t.Fatalf("expected session_id sess-1, got %s", got.SessionID)
				}
			},
		},
		{
			name:    "empty session_id returns error",
			turn:    SessionTurn{SessionID: ""},
			wantErr: true,
			wantMsg: "session_id is required",
		},
		{
			name:    "whitespace session_id returns error",
			turn:    SessionTurn{SessionID: "   "},
			wantErr: true,
			wantMsg: "session_id is required",
		},
		{
			name: "preserves existing ID and timestamps",
			turn: SessionTurn{ID: "turn-1", SessionID: "sess-1", CreatedAt: "2026-01-01T00:00:00Z", UpdatedAt: "2026-01-01T00:00:00Z"},
			createFn: func(_ context.Context, turn SessionTurn) (SessionTurn, error) {
				if turn.ID != "turn-1" {
					t.Fatalf("expected ID turn-1, got %s", turn.ID)
				}
				if turn.CreatedAt != "2026-01-01T00:00:00Z" {
					t.Fatalf("expected CreatedAt to be preserved, got %s", turn.CreatedAt)
				}
				return turn, nil
			},
			checkResult: func(t *testing.T, got SessionTurn) {
				if got.ID != "turn-1" {
					t.Fatalf("expected ID turn-1, got %s", got.ID)
				}
			},
		},
		{
			name: "repo error propagated",
			turn: SessionTurn{SessionID: "sess-1"},
			createFn: func(_ context.Context, _ SessionTurn) (SessionTurn, error) {
				return SessionTurn{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{createSessionTurnFn: tt.createFn}
			uc := newTestUc(repo)
			got, err := uc.CreateTurn(context.Background(), tt.turn)
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

func TestListTurns(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		limit       int
		offset      int
		listFn      func(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got SessionTurnListResult)
	}{
		{
			name:      "returns turns from repo",
			sessionID: "sess-1",
			limit:     20,
			offset:    0,
			listFn: func(_ context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error) {
				return SessionTurnListResult{
					Items: []SessionTurn{
						{ID: "t1", SessionID: sessionID, TurnNumber: 1},
						{ID: "t2", SessionID: sessionID, TurnNumber: 2},
					},
					Total: 2,
				}, nil
			},
			checkResult: func(t *testing.T, got SessionTurnListResult) {
				if got.Total != 2 {
					t.Fatalf("expected total 2, got %d", got.Total)
				}
				if len(got.Items) != 2 {
					t.Fatalf("expected 2 items, got %d", len(got.Items))
				}
				if got.Items[0].ID != "t1" {
					t.Fatalf("expected first item id t1, got %s", got.Items[0].ID)
				}
			},
		},
		{
			name:      "empty result",
			sessionID: "sess-1",
			limit:     20,
			offset:    0,
			listFn: func(_ context.Context, _ string, _, _ int) (SessionTurnListResult, error) {
				return SessionTurnListResult{Items: []SessionTurn{}, Total: 0}, nil
			},
			checkResult: func(t *testing.T, got SessionTurnListResult) {
				if got.Total != 0 {
					t.Fatalf("expected total 0, got %d", got.Total)
				}
				if len(got.Items) != 0 {
					t.Fatalf("expected 0 items, got %d", len(got.Items))
				}
			},
		},
		{
			name:      "empty session_id returns error",
			sessionID: "",
			wantErr:   true,
			wantMsg:   "session id is required",
		},
		{
			name:      "whitespace session_id returns error",
			sessionID: "   ",
			wantErr:   true,
			wantMsg:   "session id is required",
		},
		{
			name:      "default limit when zero",
			sessionID: "sess-1",
			limit:     0,
			listFn: func(_ context.Context, _ string, limit, _ int) (SessionTurnListResult, error) {
				if limit != 20 {
					t.Fatalf("expected default limit 20, got %d", limit)
				}
				return SessionTurnListResult{}, nil
			},
		},
		{
			name:      "limit capped at 100",
			sessionID: "sess-1",
			limit:     200,
			listFn: func(_ context.Context, _ string, limit, _ int) (SessionTurnListResult, error) {
				if limit != 20 {
					t.Fatalf("expected default limit 20 for over-100 input, got %d", limit)
				}
				return SessionTurnListResult{}, nil
			},
		},
		{
			name:      "repo error propagated",
			sessionID: "sess-1",
			listFn: func(_ context.Context, _ string, _, _ int) (SessionTurnListResult, error) {
				return SessionTurnListResult{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{listSessionTurnsFn: tt.listFn}
			uc := newTestUc(repo)
			got, err := uc.ListTurns(context.Background(), tt.sessionID, tt.limit, tt.offset)
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

func TestIncrementInvocationCounts(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		toolDelta  int
		mcpDelta   int
		skillDelta int
		incrFn     func(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error
		wantErr    bool
		wantMsg    string
	}{
		{
			name:       "valid increment",
			sessionID:  "sess-1",
			toolDelta:  1,
			mcpDelta:   0,
			skillDelta: 0,
			incrFn: func(_ context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error {
				if sessionID != "sess-1" {
					t.Fatalf("expected session_id sess-1, got %s", sessionID)
				}
				if toolDelta != 1 {
					t.Fatalf("expected tool_delta 1, got %d", toolDelta)
				}
				return nil
			},
		},
		{
			name:       "empty session_id returns error with non-zero deltas",
			sessionID:  "",
			toolDelta:  1,
			mcpDelta:   0,
			skillDelta: 0,
			wantErr:    true,
			wantMsg:    "session id is required",
		},
		{
			name:       "whitespace session_id returns error with non-zero deltas",
			sessionID:  "   ",
			toolDelta:  0,
			mcpDelta:   1,
			skillDelta: 0,
			wantErr:    true,
			wantMsg:    "session id is required",
		},
		{
			name:       "all zero deltas returns nil",
			sessionID:  "sess-1",
			toolDelta:  0,
			mcpDelta:   0,
			skillDelta: 0,
		},
		{
			name:       "non-zero deltas accumulated without error",
			sessionID:  "sess-1",
			toolDelta:  1,
			mcpDelta:   2,
			skillDelta: 3,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{incrementInvocationCountsFn: tt.incrFn}
			uc := newTestUc(repo)
			err := uc.IncrementInvocationCounts(context.Background(), tt.sessionID, tt.toolDelta, tt.mcpDelta, tt.skillDelta)
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
		})
	}
}

func TestInsertSessionSummary(t *testing.T) {
	tests := []struct {
		name     string
		row      SessionSummary
		insertFn func(ctx context.Context, row SessionSummary) error
		wantErr  bool
		wantMsg  string
	}{
		{
			name: "valid insert",
			row:  SessionSummary{SessionID: "sess-1", SummaryMarkdown: "summary text", FromTurn: 1, ToTurn: 5},
			insertFn: func(_ context.Context, row SessionSummary) error {
				if row.SessionID != "sess-1" {
					t.Fatalf("expected session_id sess-1, got %s", row.SessionID)
				}
				return nil
			},
		},
		{
			name:    "empty session_id returns error",
			row:     SessionSummary{SessionID: "", SummaryMarkdown: "text"},
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:    "whitespace session_id returns error",
			row:     SessionSummary{SessionID: "   ", SummaryMarkdown: "text"},
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name: "repo error propagated",
			row:  SessionSummary{SessionID: "sess-1", SummaryMarkdown: "text"},
			insertFn: func(_ context.Context, _ SessionSummary) error {
				return errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{insertSessionSummaryFn: tt.insertFn}
			uc := newTestUc(repo)
			err := uc.InsertSessionSummary(context.Background(), tt.row)
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
		})
	}
}

func TestListSessionSummaries(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		listFn      func(ctx context.Context, sessionID string) ([]SessionSummary, error)
		wantErr     bool
		checkResult func(t *testing.T, got []SessionSummary)
	}{
		{
			name:      "returns summaries from repo",
			sessionID: "sess-1",
			listFn: func(_ context.Context, _ string) ([]SessionSummary, error) {
				return []SessionSummary{
					{ID: "sum-1", SessionID: "sess-1", FromTurn: 1, ToTurn: 5},
					{ID: "sum-2", SessionID: "sess-1", FromTurn: 6, ToTurn: 10},
				}, nil
			},
			checkResult: func(t *testing.T, got []SessionSummary) {
				if len(got) != 2 {
					t.Fatalf("expected 2 summaries, got %d", len(got))
				}
				if got[0].ID != "sum-1" {
					t.Fatalf("expected first summary id sum-1, got %s", got[0].ID)
				}
			},
		},
		{
			name:      "empty result",
			sessionID: "sess-1",
			listFn: func(_ context.Context, _ string) ([]SessionSummary, error) {
				return nil, nil
			},
			checkResult: func(t *testing.T, got []SessionSummary) {
				if len(got) != 0 {
					t.Fatalf("expected 0 summaries, got %d", len(got))
				}
			},
		},
		{
			name:      "repo error propagated",
			sessionID: "sess-1",
			listFn: func(_ context.Context, _ string) ([]SessionSummary, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{listSessionSummariesFn: tt.listFn}
			uc := newTestUc(repo)
			got, err := uc.ListSessionSummaries(context.Background(), tt.sessionID)
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
				tt.checkResult(t, got)
			}
		})
	}
}

func TestGetSessionState(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		getFn       func(ctx context.Context, sessionID string) (map[string]string, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got map[string]string)
	}{
		{
			name:      "returns state from repo",
			sessionID: "sess-1",
			getFn: func(_ context.Context, _ string) (map[string]string, error) {
				return map[string]string{"key1": "val1", "key2": "val2"}, nil
			},
			checkResult: func(t *testing.T, got map[string]string) {
				if len(got) != 2 {
					t.Fatalf("expected 2 keys, got %d", len(got))
				}
				if got["key1"] != "val1" {
					t.Fatalf("expected key1=val1, got key1=%s", got["key1"])
				}
			},
		},
		{
			name:      "empty session_id returns error",
			sessionID: "",
			wantErr:   true,
			wantMsg:   "session id is required",
		},
		{
			name:      "whitespace session_id returns error",
			sessionID: "   ",
			wantErr:   true,
			wantMsg:   "session id is required",
		},
		{
			name:      "repo error propagated",
			sessionID: "sess-1",
			getFn: func(_ context.Context, _ string) (map[string]string, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{getSessionStateFn: tt.getFn}
			uc := newTestUc(repo)
			got, err := uc.GetSessionState(context.Background(), tt.sessionID)
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

func TestSaveSessionState(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		state     map[string]string
		saveFn    func(ctx context.Context, sessionID string, state map[string]string) error
		wantErr   bool
		wantMsg   string
	}{
		{
			name:      "valid save",
			sessionID: "sess-1",
			state:     map[string]string{"key1": "val1"},
			saveFn: func(_ context.Context, sessionID string, state map[string]string) error {
				if sessionID != "sess-1" {
					t.Fatalf("expected session_id sess-1, got %s", sessionID)
				}
				if state["key1"] != "val1" {
					t.Fatalf("expected key1=val1, got key1=%s", state["key1"])
				}
				return nil
			},
		},
		{
			name:      "empty session_id returns error",
			sessionID: "",
			state:     map[string]string{"key1": "val1"},
			wantErr:   true,
			wantMsg:   "session id is required",
		},
		{
			name:      "whitespace session_id returns error",
			sessionID: "   ",
			state:     map[string]string{"key1": "val1"},
			wantErr:   true,
			wantMsg:   "session id is required",
		},
		{
			name:      "repo error propagated",
			sessionID: "sess-1",
			state:     map[string]string{},
			saveFn: func(_ context.Context, _ string, _ map[string]string) error {
				return errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{saveSessionStateFn: tt.saveFn}
			uc := newTestUc(repo)
			err := uc.SaveSessionState(context.Background(), tt.sessionID, tt.state)
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
		})
	}
}

func TestApplyStateDelta(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		delta       StateDelta
		wantSets    map[string]string
		wantDeletes []string
		wantErr     bool
	}{
		{
			name:        "set operation",
			sessionID:   "sess-1",
			delta:       StateDelta{Operation: "set", Path: "key1", ValueJSON: `"value1"`},
			wantSets:    map[string]string{"key1": `"value1"`},
			wantDeletes: nil,
		},
		{
			name:        "delete operation",
			sessionID:   "sess-1",
			delta:       StateDelta{Operation: "delete", Path: "key1"},
			wantSets:    nil,
			wantDeletes: []string{"key1"},
		},
		{
			name:        "default operation sets value",
			sessionID:   "sess-1",
			delta:       StateDelta{Operation: "unknown", Path: "key1", ValueJSON: `"value1"`},
			wantSets:    map[string]string{"key1": `"value1"`},
			wantDeletes: nil,
		},
		{
			name:      "empty path returns nil",
			sessionID: "sess-1",
			delta:     StateDelta{Path: ""},
			wantErr:   false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var patchSets map[string]string
			var patchDeletes []string
			repo := &testRepo{
				patchSessionStateFn: func(_ context.Context, _ string, sets map[string]string, deletes []string) error {
					patchSets = sets
					patchDeletes = deletes
					return nil
				},
			}
			uc := newTestUc(repo)
			err := uc.ApplyStateDelta(context.Background(), tt.sessionID, tt.delta)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.delta.Path == "" {
				return
			}
			if patchSets == nil && tt.wantSets != nil {
				t.Fatal("expected sets to be patched")
			}
			if tt.wantSets != nil {
				if len(patchSets) != len(tt.wantSets) {
					t.Fatalf("expected %d set keys, got %d", len(tt.wantSets), len(patchSets))
				}
				for k, v := range tt.wantSets {
					if patchSets[k] != v {
						t.Fatalf("expected set key %s=%s, got %s=%s", k, v, k, patchSets[k])
					}
				}
			}
			if tt.wantDeletes != nil {
				if len(patchDeletes) != len(tt.wantDeletes) {
					t.Fatalf("expected %d delete keys, got %d", len(tt.wantDeletes), len(patchDeletes))
				}
				for i, k := range tt.wantDeletes {
					if patchDeletes[i] != k {
						t.Fatalf("expected delete key[%d]=%s, got %s", i, k, patchDeletes[i])
					}
				}
			}
		})
	}
}
