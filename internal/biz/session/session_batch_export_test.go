package session

import (
	"context"
	"errors"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type testParticipantRepo struct {
	syncFn  func(ctx context.Context, sess Session, messages []ChatMessage) error
	listFn  func(ctx context.Context, sessionID string) ([]SessionParticipant, error)
}

func (r *testParticipantRepo) SyncFromSession(ctx context.Context, sess Session, messages []ChatMessage) error {
	if r.syncFn != nil {
		return r.syncFn(ctx, sess, messages)
	}
	return nil
}

func (r *testParticipantRepo) ListBySession(ctx context.Context, sessionID string) ([]SessionParticipant, error) {
	if r.listFn != nil {
		return r.listFn(ctx, sessionID)
	}
	return nil, nil
}

func newTestUcWithParticipants(repo *testRepo, participants *testParticipantRepo) *SessionUsecase {
	return NewSessionUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{}, nil, participants)
}

func TestPreviewBatch(t *testing.T) {
	tests := []struct {
		name           string
		mode           string
		ids            []string
		olderThanDays  int
		scope          SessionBatchScope
		includeArchived bool
		listByIDsFn    func(ctx context.Context, ids []string) ([]Session, error)
		wantErr        bool
		wantMsg        string
		checkResult    func(t *testing.T, got SessionBatchPreview)
	}{
		{
			name:          "valid preview with sessions",
			mode:          "archive",
			ids:           []string{"s1", "s2"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, ids []string) ([]Session, error) {
				return []Session{
					{ID: "s1", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
					{ID: "s2", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
				}, nil
			},
			checkResult: func(t *testing.T, got SessionBatchPreview) {
				if got.Matched != 2 {
					t.Fatalf("expected matched 2, got %d", got.Matched)
				}
				if got.SkippedRunning != 0 {
					t.Fatalf("expected skipped_running 0, got %d", got.SkippedRunning)
				}
				if len(got.SampleIDs) != 2 {
					t.Fatalf("expected 2 sample IDs, got %d", len(got.SampleIDs))
				}
			},
		},
		{
			name:          "no ids and no older_than_days returns error",
			mode:          "archive",
			ids:           nil,
			olderThanDays: 0,
			wantErr:       true,
			wantMsg:       "ids or older_than_days >= 1 is required",
		},
		{
			name:          "repo error propagated",
			mode:          "archive",
			ids:           []string{"s1"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
		{
			name:          "invalid mode returns error",
			mode:          "invalid",
			ids:           []string{"s1"},
			olderThanDays: 0,
			wantErr:       true,
			wantMsg:       "mode must be archive or delete",
		},
		{
			name:          "skips running sessions",
			mode:          "archive",
			ids:           []string{"s1", "s2"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return []Session{
					{ID: "s1", Status: "running", UpdatedAt: "2025-01-01T00:00:00Z"},
					{ID: "s2", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
				}, nil
			},
			checkResult: func(t *testing.T, got SessionBatchPreview) {
				if got.Matched != 1 {
					t.Fatalf("expected matched 1, got %d", got.Matched)
				}
				if got.SkippedRunning != 1 {
					t.Fatalf("expected skipped_running 1, got %d", got.SkippedRunning)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{listSessionsByIDsFn: tt.listByIDsFn}
			uc := newTestUc(repo)
			got, err := uc.PreviewBatch(context.Background(), tt.mode, tt.ids, tt.olderThanDays, tt.scope, tt.includeArchived)
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

func TestBatchArchive(t *testing.T) {
	tests := []struct {
		name            string
		ids             []string
		olderThanDays   int
		scope           SessionBatchScope
		listByIDsFn     func(ctx context.Context, ids []string) ([]Session, error)
		archiveByIDsFn  func(ctx context.Context, ids []string) (int, []string, error)
		wantErr         bool
		wantMsg         string
		checkResult     func(t *testing.T, got SessionBatchResult)
	}{
		{
			name:          "valid archive",
			ids:           []string{"s1", "s2"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return []Session{
					{ID: "s1", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
					{ID: "s2", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
				}, nil
			},
			archiveByIDsFn: func(_ context.Context, ids []string) (int, []string, error) {
				return len(ids), nil, nil
			},
			checkResult: func(t *testing.T, got SessionBatchResult) {
				if got.Matched != 2 {
					t.Fatalf("expected matched 2, got %d", got.Matched)
				}
				if got.Processed != 2 {
					t.Fatalf("expected processed 2, got %d", got.Processed)
				}
			},
		},
		{
			name:          "no ids and no older_than_days returns error",
			ids:           nil,
			olderThanDays: 0,
			wantErr:       true,
			wantMsg:       "ids or older_than_days >= 1 is required",
		},
		{
			name:          "repo error on load",
			ids:           []string{"s1"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
		{
			name:          "repo error on archive",
			ids:           []string{"s1"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return []Session{{ID: "s1", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"}}, nil
			},
			archiveByIDsFn: func(_ context.Context, _ []string) (int, []string, error) {
				return 0, nil, errors.New("archive error")
			},
			wantErr: true,
		},
		{
			name:          "zero matched returns result without calling mutator",
			ids:           []string{"s1"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return []Session{{ID: "s1", Status: "running", UpdatedAt: "2025-01-01T00:00:00Z"}}, nil
			},
			checkResult: func(t *testing.T, got SessionBatchResult) {
				if got.Matched != 0 {
					t.Fatalf("expected matched 0, got %d", got.Matched)
				}
				if got.SkippedRunning != 1 {
					t.Fatalf("expected skipped_running 1, got %d", got.SkippedRunning)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{
				listSessionsByIDsFn:    tt.listByIDsFn,
				archiveSessionsByIDsFn: tt.archiveByIDsFn,
			}
			uc := newTestUc(repo)
			got, err := uc.BatchArchive(context.Background(), tt.ids, tt.olderThanDays, tt.scope)
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

func TestBatchDelete(t *testing.T) {
	tests := []struct {
		name            string
		ids             []string
		olderThanDays   int
		scope           SessionBatchScope
		includeArchived bool
		listByIDsFn     func(ctx context.Context, ids []string) ([]Session, error)
		deleteByIDsFn   func(ctx context.Context, ids []string) (int, []string, error)
		wantErr         bool
		wantMsg         string
		checkResult     func(t *testing.T, got SessionBatchResult)
	}{
		{
			name:          "valid delete",
			ids:           []string{"s1", "s2"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return []Session{
					{ID: "s1", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
					{ID: "s2", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
				}, nil
			},
			deleteByIDsFn: func(_ context.Context, ids []string) (int, []string, error) {
				return len(ids), nil, nil
			},
			checkResult: func(t *testing.T, got SessionBatchResult) {
				if got.Matched != 2 {
					t.Fatalf("expected matched 2, got %d", got.Matched)
				}
				if got.Processed != 2 {
					t.Fatalf("expected processed 2, got %d", got.Processed)
				}
			},
		},
		{
			name:          "no ids and no older_than_days returns error",
			ids:           nil,
			olderThanDays: 0,
			wantErr:       true,
			wantMsg:       "ids or older_than_days >= 1 is required",
		},
		{
			name:          "repo error on load",
			ids:           []string{"s1"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
		{
			name:          "repo error on delete",
			ids:           []string{"s1"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return []Session{{ID: "s1", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"}}, nil
			},
			deleteByIDsFn: func(_ context.Context, _ []string) (int, []string, error) {
				return 0, nil, errors.New("delete error")
			},
			wantErr: true,
		},
		{
			name:          "delete with failed IDs",
			ids:           []string{"s1", "s2"},
			olderThanDays: 0,
			listByIDsFn: func(_ context.Context, _ []string) ([]Session, error) {
				return []Session{
					{ID: "s1", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
					{ID: "s2", Status: "active", UpdatedAt: "2025-01-01T00:00:00Z"},
				}, nil
			},
			deleteByIDsFn: func(_ context.Context, ids []string) (int, []string, error) {
				return 1, []string{"s2"}, nil
			},
			checkResult: func(t *testing.T, got SessionBatchResult) {
				if got.Processed != 1 {
					t.Fatalf("expected processed 1, got %d", got.Processed)
				}
				if len(got.FailedIDs) != 1 || got.FailedIDs[0] != "s2" {
					t.Fatalf("expected failed IDs [s2], got %v", got.FailedIDs)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{
				listSessionsByIDsFn:   tt.listByIDsFn,
				deleteSessionsByIDsFn: tt.deleteByIDsFn,
			}
			uc := newTestUc(repo)
			got, err := uc.BatchDelete(context.Background(), tt.ids, tt.olderThanDays, tt.scope, tt.includeArchived)
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

func TestExport(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		format      string
		getFn       func(ctx context.Context, id string) (Session, error)
		countFn     func(ctx context.Context, sessionID string) (int, error)
		listMsgFn   func(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
		timelineFn  func(ctx context.Context, sessionID string, q TimelineQuery) ([]TimelineEventRef, int, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, content, filename, contentType string)
	}{
		{
			name:   "valid markdown export",
			id:     "sess-1",
			format: "markdown",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Title: "Test Session", OwnerType: "agent", AgentID: "a1", MessageCount: 1, TotalTokens: 100}, nil
			},
			countFn: func(_ context.Context, _ string) (int, error) {
				return 1, nil
			},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{{ID: "m1", Role: "user", ContentMarkdown: "Hello", TurnNumber: 1}}, nil
			},
			timelineFn: func(_ context.Context, _ string, _ TimelineQuery) ([]TimelineEventRef, int, error) {
				return nil, 0, nil
			},
			checkResult: func(t *testing.T, content, filename, contentType string) {
				if filename != "Test-Session.md" {
					t.Fatalf("expected filename Test-Session.md, got %s", filename)
				}
				if contentType != "text/markdown; charset=utf-8" {
					t.Fatalf("expected text/markdown content type, got %s", contentType)
				}
			},
		},
		{
			name:   "valid json export",
			id:     "sess-1",
			format: "json",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Title: "JSON Session", OwnerType: "agent", AgentID: "a1", MessageCount: 0, TotalTokens: 0}, nil
			},
			countFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			timelineFn: func(_ context.Context, _ string, _ TimelineQuery) ([]TimelineEventRef, int, error) {
				return nil, 0, nil
			},
			checkResult: func(t *testing.T, content, filename, contentType string) {
				if filename != "JSON-Session.json" {
					t.Fatalf("expected filename JSON-Session.json, got %s", filename)
				}
				if contentType != "application/json; charset=utf-8" {
					t.Fatalf("expected application/json content type, got %s", contentType)
				}
			},
		},
		{
			name:    "empty session_id returns error",
			id:      "",
			format:  "markdown",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:    "whitespace session_id returns error",
			id:      "   ",
			format:  "markdown",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:   "repo error on get session",
			id:     "sess-1",
			format: "markdown",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, kerrors.NotFound("SESSION", "session not found")
			},
			wantErr: true,
		},
		{
			name:   "repo error on count messages",
			id:     "sess-1",
			format: "markdown",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Title: "Test", OwnerType: "agent"}, nil
			},
			countFn: func(_ context.Context, _ string) (int, error) {
				return 0, errors.New("db error")
			},
			wantErr: true,
		},
		{
			name:   "invalid format returns error",
			id:     "sess-1",
			format: "csv",
			wantErr: true,
			wantMsg: "format must be markdown or json",
		},
		{
			name:   "default format is markdown",
			id:     "sess-1",
			format: "",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Title: "Default", OwnerType: "agent"}, nil
			},
			countFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			timelineFn: func(_ context.Context, _ string, _ TimelineQuery) ([]TimelineEventRef, int, error) {
				return nil, 0, nil
			},
			checkResult: func(t *testing.T, _, filename, contentType string) {
				if contentType != "text/markdown; charset=utf-8" {
					t.Fatalf("expected markdown content type, got %s", contentType)
				}
				if filename != "Default.md" {
					t.Fatalf("expected filename Default.md, got %s", filename)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{
				listTimelineEventRefsPagedFn: tt.timelineFn,
				countMessagesBySessionFn:     tt.countFn,
				listMessagesBySessionFn:      tt.listMsgFn,
			}
			if tt.getFn != nil {
				repo.getSessionByIDFn = tt.getFn
			}
			uc := newTestUc(repo)
			content, filename, contentType, err := uc.Export(context.Background(), tt.id, tt.format)
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
				tt.checkResult(t, content, filename, contentType)
			}
		})
	}
}

func TestListParticipants(t *testing.T) {
	tests := []struct {
		name        string
		sessionID   string
		getFn       func(ctx context.Context, id string) (Session, error)
		countFn     func(ctx context.Context, sessionID string) (int, error)
		listMsgFn   func(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
		syncFn      func(ctx context.Context, sess Session, messages []ChatMessage) error
		listPartFn  func(ctx context.Context, sessionID string) ([]SessionParticipant, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got []SessionParticipant)
	}{
		{
			name:      "returns participants",
			sessionID: "sess-1",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Status: "active"}, nil
			},
			countFn: func(_ context.Context, _ string) (int, error) {
				return 1, nil
			},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{{ID: "m1", Role: "user"}}, nil
			},
			syncFn: func(_ context.Context, _ Session, _ []ChatMessage) error {
				return nil
			},
			listPartFn: func(_ context.Context, sessionID string) ([]SessionParticipant, error) {
				return []SessionParticipant{
					{ID: "p1", SessionID: sessionID, ParticipantType: "agent", DisplayName: "Agent1"},
				}, nil
			},
			checkResult: func(t *testing.T, got []SessionParticipant) {
				if len(got) != 1 {
					t.Fatalf("expected 1 participant, got %d", len(got))
				}
				if got[0].DisplayName != "Agent1" {
					t.Fatalf("expected display name Agent1, got %s", got[0].DisplayName)
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
			name:      "repo error on get session",
			sessionID: "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, kerrors.NotFound("SESSION", "session not found")
			},
			wantErr: true,
		},
		{
			name:      "repo error on sync",
			sessionID: "sess-1",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Status: "active"}, nil
			},
			countFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			syncFn: func(_ context.Context, _ Session, _ []ChatMessage) error {
				return errors.New("sync error")
			},
			wantErr: true,
		},
		{
			name:      "repo error on list participants",
			sessionID: "sess-1",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Status: "active"}, nil
			},
			countFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			syncFn: func(_ context.Context, _ Session, _ []ChatMessage) error {
				return nil
			},
			listPartFn: func(_ context.Context, _ string) ([]SessionParticipant, error) {
				return nil, errors.New("list error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{
				countMessagesBySessionFn: tt.countFn,
				listMessagesBySessionFn:  tt.listMsgFn,
			}
			if tt.getFn != nil {
				repo.getSessionByIDFn = tt.getFn
			}
			participants := &testParticipantRepo{
				syncFn: tt.syncFn,
				listFn: tt.listPartFn,
			}
			uc := newTestUcWithParticipants(repo, participants)
			got, err := uc.ListParticipants(context.Background(), tt.sessionID)
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
