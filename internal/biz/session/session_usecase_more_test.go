package session

import (
	"context"
	"errors"
	"strings"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

type testRepo struct {
	mockSessionRepo
	updateSessionFn             func(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
	restoreSessionFn            func(ctx context.Context, id string) (Session, error)
	archiveSessionFn            func(ctx context.Context, id string) (int, error)
	deleteSessionFn             func(ctx context.Context, id string) (int, error)
	pinSessionFn                func(ctx context.Context, id string) (Session, error)
	unpinSessionFn              func(ctx context.Context, id string) (Session, error)
	updateSessionTitleFn        func(ctx context.Context, id, title string) (Session, error)
	appendChatMessageFn         func(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
	updateChatMessageStatusFn   func(ctx context.Context, sessionID, messageID, status, errorMessage string) error
	updateMessageFeedbackJSONFn func(ctx context.Context, sessionID, messageID, rating, comment string) error
	listTimelineEventRefsPagedFn func(ctx context.Context, sessionID string, q TimelineQuery) ([]TimelineEventRef, int, error)
	countMessagesBySessionFn     func(ctx context.Context, sessionID string) (int, error)
	listMessagesBySessionFn      func(ctx context.Context, sessionID string, limit, offset int) ([]ChatMessage, error)
	listMessagesByIDsFn          func(ctx context.Context, sessionID string, ids []string) ([]ChatMessage, error)
	listToolInvocationsByIDsFn   func(ctx context.Context, sessionID string, ids []string) ([]ToolInvocationView, error)
	listSkillInvocationsByIDsFn  func(ctx context.Context, sessionID string, ids []string) ([]SkillInvocationView, error)
	lookupAgentDisplayNamesFn    func(ctx context.Context, agentIDs []string) (map[string]string, error)
	createSessionTurnFn          func(ctx context.Context, turn SessionTurn) (SessionTurn, error)
	listSessionTurnsFn           func(ctx context.Context, sessionID string, limit, offset int) (SessionTurnListResult, error)
	incrementInvocationCountsFn  func(ctx context.Context, sessionID string, toolDelta, mcpDelta, skillDelta int) error
	insertSessionSummaryFn       func(ctx context.Context, row SessionSummary) error
	listSessionSummariesFn       func(ctx context.Context, sessionID string) ([]SessionSummary, error)
	getSessionStateFn            func(ctx context.Context, sessionID string) (map[string]string, error)
	saveSessionStateFn           func(ctx context.Context, sessionID string, state map[string]string) error
	patchSessionStateFn          func(ctx context.Context, sessionID string, sets map[string]string, deletes []string) error
	listSessionsByIDsFn          func(ctx context.Context, ids []string) ([]Session, error)
	listSessionsForBatchFn       func(ctx context.Context, q SessionSearchQuery) ([]Session, error)
	archiveSessionsByIDsFn       func(ctx context.Context, ids []string) (int, []string, error)
	deleteSessionsByIDsFn        func(ctx context.Context, ids []string) (int, []string, error)
	compressSessionInTxFn        func(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error
	tryIncrementCompressVersionFn func(ctx context.Context, sessionID string) (int64, error)
	sessionSummaryExistsFn       func(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error)
	deleteSessionsByAgentIDFn    func(ctx context.Context, agentID string) error
	bumpSessionRevisionFn        func(ctx context.Context, sessionID string) (int64, error)
	getSessionRevisionFn         func(ctx context.Context, sessionID string) (int64, error)
	updateSessionTurnFn          func(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error)
	upsertChatActivityMessageFn  func(ctx context.Context, sessionID string, msg ChatMessage) (bool, error)
	listMessagesAfterTurnFn      func(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error)
	listMessagesRecentFn         func(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error)
	listMessagesAfterRevisionFn  func(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error)
}

var _ SessionRepo = (*testRepo)(nil)

func (r *testRepo) UpdateSession(ctx context.Context, id string, fields SessionUpdateFields) (Session, error) {
	if r.updateSessionFn != nil {
		return r.updateSessionFn(ctx, id, fields)
	}
	return r.mockSessionRepo.UpdateSession(ctx, id, fields)
}

func (r *testRepo) RestoreSession(ctx context.Context, id string) (Session, error) {
	if r.restoreSessionFn != nil {
		return r.restoreSessionFn(ctx, id)
	}
	return r.mockSessionRepo.RestoreSession(ctx, id)
}

func (r *testRepo) ArchiveSession(ctx context.Context, id string) (int, error) {
	if r.archiveSessionFn != nil {
		return r.archiveSessionFn(ctx, id)
	}
	return r.mockSessionRepo.ArchiveSession(ctx, id)
}

func (r *testRepo) DeleteSession(ctx context.Context, id string) (int, error) {
	if r.deleteSessionFn != nil {
		return r.deleteSessionFn(ctx, id)
	}
	return r.mockSessionRepo.DeleteSession(ctx, id)
}

func (r *testRepo) PinSession(ctx context.Context, id string) (Session, error) {
	if r.pinSessionFn != nil {
		return r.pinSessionFn(ctx, id)
	}
	return r.mockSessionRepo.PinSession(ctx, id)
}

func (r *testRepo) UnpinSession(ctx context.Context, id string) (Session, error) {
	if r.unpinSessionFn != nil {
		return r.unpinSessionFn(ctx, id)
	}
	return r.mockSessionRepo.UnpinSession(ctx, id)
}

func (r *testRepo) UpdateSessionTitle(ctx context.Context, id, title string) (Session, error) {
	if r.updateSessionTitleFn != nil {
		return r.updateSessionTitleFn(ctx, id, title)
	}
	return r.mockSessionRepo.UpdateSessionTitle(ctx, id, title)
}

func (r *testRepo) AppendChatMessage(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error {
	if r.appendChatMessageFn != nil {
		return r.appendChatMessageFn(ctx, sessionID, msg, bumpModelCall)
	}
	return r.mockSessionRepo.AppendChatMessage(ctx, sessionID, msg, bumpModelCall)
}

func (r *testRepo) UpdateChatMessageStatus(ctx context.Context, sessionID, messageID, status, errorMessage string) error {
	if r.updateChatMessageStatusFn != nil {
		return r.updateChatMessageStatusFn(ctx, sessionID, messageID, status, errorMessage)
	}
	return r.mockSessionRepo.UpdateChatMessageStatus(ctx, sessionID, messageID, status, errorMessage)
}

func (r *testRepo) UpdateMessageFeedbackJSON(ctx context.Context, sessionID, messageID, rating, comment string) error {
	if r.updateMessageFeedbackJSONFn != nil {
		return r.updateMessageFeedbackJSONFn(ctx, sessionID, messageID, rating, comment)
	}
	return r.mockSessionRepo.UpdateMessageFeedbackJSON(ctx, sessionID, messageID, rating, comment)
}

func (r *testRepo) ListSessionsByIDs(ctx context.Context, ids []string) ([]Session, error) {
	if r.listSessionsByIDsFn != nil {
		return r.listSessionsByIDsFn(ctx, ids)
	}
	return r.mockSessionRepo.ListSessionsByIDs(ctx, ids)
}

func (r *testRepo) ListSessionsForBatch(ctx context.Context, q SessionSearchQuery) ([]Session, error) {
	if r.listSessionsForBatchFn != nil {
		return r.listSessionsForBatchFn(ctx, q)
	}
	return r.mockSessionRepo.ListSessionsForBatch(ctx, q)
}

func (r *testRepo) ArchiveSessionsByIDs(ctx context.Context, ids []string) (int, []string, error) {
	if r.archiveSessionsByIDsFn != nil {
		return r.archiveSessionsByIDsFn(ctx, ids)
	}
	return r.mockSessionRepo.ArchiveSessionsByIDs(ctx, ids)
}

func (r *testRepo) DeleteSessionsByIDs(ctx context.Context, ids []string) (int, []string, error) {
	if r.deleteSessionsByIDsFn != nil {
		return r.deleteSessionsByIDsFn(ctx, ids)
	}
	return r.mockSessionRepo.DeleteSessionsByIDs(ctx, ids)
}

func newTestUc(repo *testRepo) *SessionUsecase {
	return NewSessionUsecase(repo, &mockAgentLookup{}, &mockTeamLookup{}, nil, &mockParticipantRepo{})
}

func strPtr(s string) *string {
	return &s
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		fields      SessionUpdateFields
		updateFn    func(ctx context.Context, id string, fields SessionUpdateFields) (Session, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got Session)
	}{
		{
			name:   "valid update with fields",
			id:     "sess-1",
			fields: SessionUpdateFields{Title: strPtr("New Title"), Visibility: strPtr("public")},
			updateFn: func(_ context.Context, id string, fields SessionUpdateFields) (Session, error) {
				return Session{ID: id, Title: *fields.Title, Visibility: *fields.Visibility}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.ID != "sess-1" {
					t.Fatalf("expected id sess-1, got %s", got.ID)
				}
				if got.Title != "New Title" {
					t.Fatalf("expected title New Title, got %s", got.Title)
				}
				if got.Visibility != "public" {
					t.Fatalf("expected visibility public, got %s", got.Visibility)
				}
			},
		},
		{
			name:    "empty session_id returns error",
			id:      "",
			fields:  SessionUpdateFields{Title: strPtr("Title")},
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:    "whitespace session_id returns error",
			id:      "   ",
			fields:  SessionUpdateFields{Title: strPtr("Title")},
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:   "title is trimmed before update",
			id:     "sess-1",
			fields: SessionUpdateFields{Title: strPtr("  Hello World  ")},
			updateFn: func(_ context.Context, id string, fields SessionUpdateFields) (Session, error) {
				if fields.Title == nil {
					t.Fatal("expected title to be set")
				}
				if *fields.Title != "Hello World" {
					t.Fatalf("expected trimmed title 'Hello World', got %q", *fields.Title)
				}
				return Session{ID: id, Title: *fields.Title}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.Title != "Hello World" {
					t.Fatalf("expected title Hello World, got %s", got.Title)
				}
			},
		},
		{
			name:   "nil title passes through",
			id:     "sess-1",
			fields: SessionUpdateFields{Visibility: strPtr("private")},
			updateFn: func(_ context.Context, id string, fields SessionUpdateFields) (Session, error) {
				if fields.Title != nil {
					t.Fatal("expected title to be nil")
				}
				return Session{ID: id, Visibility: *fields.Visibility}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.Visibility != "private" {
					t.Fatalf("expected visibility private, got %s", got.Visibility)
				}
			},
		},
		{
			name:   "repo error propagated",
			id:     "sess-1",
			fields: SessionUpdateFields{Title: strPtr("Title")},
			updateFn: func(_ context.Context, _ string, _ SessionUpdateFields) (Session, error) {
				return Session{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{updateSessionFn: tt.updateFn}
			uc := newTestUc(repo)
			got, err := uc.Update(context.Background(), tt.id, tt.fields)
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

func TestArchive(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		archiveFn func(ctx context.Context, id string) (int, error)
		getFn     func(ctx context.Context, id string) (Session, error)
		wantErr   bool
		wantMsg   string
	}{
		{
			name: "valid archive",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "idle"}, nil
			},
			archiveFn: func(_ context.Context, _ string) (int, error) {
				return 1, nil
			},
		},
		{
			name: "already archived session returns not found",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "idle", ArchivedAt: "2025-01-01T00:00:00Z"}, nil
			},
			archiveFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			wantErr: true,
		},
		{
			name: "running session cannot be archived",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "running"}, nil
			},
			wantErr: true,
			wantMsg: "session is running, cannot archive",
		},
		{
			name: "awaiting_confirmation session cannot be archived",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "awaiting_confirmation"}, nil
			},
			wantErr: true,
			wantMsg: "session is awaiting_confirmation, cannot archive",
		},
		{
			name: "get session error propagated",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, kerrors.NotFound("SESSION", "session not found")
			},
			wantErr: true,
		},
		{
			name: "archive repo error propagated",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "idle"}, nil
			},
			archiveFn: func(_ context.Context, _ string) (int, error) {
				return 0, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{archiveSessionFn: tt.archiveFn}
			if tt.getFn != nil {
				repo.getSessionByIDFn = tt.getFn
			}
			uc := newTestUc(repo)
			err := uc.Archive(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					if strings.Contains(tt.wantMsg, "cannot archive") && (strings.Contains(tt.wantMsg, "running") || strings.Contains(tt.wantMsg, "awaiting_confirmation")) {
						assertConflict(t, err, tt.wantMsg)
					} else {
						assertBadRequest(t, err, tt.wantMsg)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRestore(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		restoreFn   func(ctx context.Context, id string) (Session, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got Session)
	}{
		{
			name: "valid restore clears archived_at",
			id:   "sess-1",
			restoreFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Status: "active", ArchivedAt: ""}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.ID != "sess-1" {
					t.Fatalf("expected id sess-1, got %s", got.ID)
				}
				if got.ArchivedAt != "" {
					t.Fatalf("expected archived_at to be empty, got %s", got.ArchivedAt)
				}
				if got.Status != "active" {
					t.Fatalf("expected status active, got %s", got.Status)
				}
			},
		},
		{
			name:    "empty session_id returns error",
			id:      "",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:    "whitespace session_id returns error",
			id:      "   ",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name: "repo error propagated",
			id:   "sess-1",
			restoreFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{restoreSessionFn: tt.restoreFn}
			uc := newTestUc(repo)
			got, err := uc.Restore(context.Background(), tt.id)
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

func TestDelete(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		deleteFn  func(ctx context.Context, id string) (int, error)
		getFn     func(ctx context.Context, id string) (Session, error)
		wantErr   bool
		wantMsg   string
	}{
		{
			name: "valid delete",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "idle"}, nil
			},
			deleteFn: func(_ context.Context, _ string) (int, error) {
				return 1, nil
			},
		},
		{
			name: "already deleted session returns not found",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "idle", DeletedAt: "2025-01-01T00:00:00Z"}, nil
			},
			deleteFn: func(_ context.Context, _ string) (int, error) {
				return 0, nil
			},
			wantErr: true,
		},
		{
			name: "running session cannot be deleted",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "running"}, nil
			},
			wantErr: true,
			wantMsg: "session is running, cannot delete",
		},
		{
			name: "awaiting_confirmation session cannot be deleted",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "awaiting_confirmation"}, nil
			},
			wantErr: true,
			wantMsg: "session is awaiting_confirmation, cannot delete",
		},
		{
			name: "get session error propagated",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, kerrors.NotFound("SESSION", "session not found")
			},
			wantErr: true,
		},
		{
			name: "delete repo error propagated",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{ID: "sess-1", Status: "idle"}, nil
			},
			deleteFn: func(_ context.Context, _ string) (int, error) {
				return 0, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{deleteSessionFn: tt.deleteFn}
			if tt.getFn != nil {
				repo.getSessionByIDFn = tt.getFn
			}
			uc := newTestUc(repo)
			err := uc.Delete(context.Background(), tt.id)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					if strings.Contains(tt.wantMsg, "cannot delete") && (strings.Contains(tt.wantMsg, "running") || strings.Contains(tt.wantMsg, "awaiting_confirmation")) {
						assertConflict(t, err, tt.wantMsg)
					} else {
						assertBadRequest(t, err, tt.wantMsg)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestRename(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		title       string
		renameFn    func(ctx context.Context, id, title string) (Session, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got Session)
	}{
		{
			name:  "valid rename",
			id:    "sess-1",
			title: "My Session",
			renameFn: func(_ context.Context, id, title string) (Session, error) {
				return Session{ID: id, Title: title}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.ID != "sess-1" {
					t.Fatalf("expected id sess-1, got %s", got.ID)
				}
				if got.Title != "My Session" {
					t.Fatalf("expected title My Session, got %s", got.Title)
				}
			},
		},
		{
			name:    "empty title returns error",
			id:      "sess-1",
			title:   "",
			wantErr: true,
			wantMsg: "title is required",
		},
		{
			name:    "whitespace title returns error",
			id:      "sess-1",
			title:   "   ",
			wantErr: true,
			wantMsg: "title is required",
		},
		{
			name:  "title is trimmed before rename",
			id:    "sess-1",
			title: "  Hello  ",
			renameFn: func(_ context.Context, id, title string) (Session, error) {
				if title != "Hello" {
					t.Fatalf("expected trimmed title 'Hello', got %q", title)
				}
				return Session{ID: id, Title: title}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.Title != "Hello" {
					t.Fatalf("expected title Hello, got %s", got.Title)
				}
			},
		},
		{
			name:  "repo error propagated",
			id:    "sess-1",
			title: "Title",
			renameFn: func(_ context.Context, _, _ string) (Session, error) {
				return Session{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{updateSessionTitleFn: tt.renameFn}
			uc := newTestUc(repo)
			got, err := uc.Rename(context.Background(), tt.id, tt.title)
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

func TestGet(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		getFn       func(ctx context.Context, id string) (Session, error)
		wantErr     bool
		checkResult func(t *testing.T, got Session)
	}{
		{
			name: "returns session from repo",
			id:   "sess-1",
			getFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, Title: "Test", Status: "active"}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.ID != "sess-1" {
					t.Fatalf("expected id sess-1, got %s", got.ID)
				}
				if got.Title != "Test" {
					t.Fatalf("expected title Test, got %s", got.Title)
				}
				if got.Status != "active" {
					t.Fatalf("expected status active, got %s", got.Status)
				}
			},
		},
		{
			name: "repo error propagated",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, kerrors.NotFound("SESSION", "session not found")
			},
			wantErr: true,
		},
		{
			name: "repo internal error propagated",
			id:   "sess-1",
			getFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, errors.New("db connection lost")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{}
			repo.getSessionByIDFn = tt.getFn
			uc := newTestUc(repo)
			got, err := uc.Get(context.Background(), tt.id)
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

func TestPin(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		pinFn       func(ctx context.Context, id string) (Session, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got Session)
	}{
		{
			name: "valid pin sets pinned_at",
			id:   "sess-1",
			pinFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, PinnedAt: "2025-06-01T12:00:00Z"}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.ID != "sess-1" {
					t.Fatalf("expected id sess-1, got %s", got.ID)
				}
				if got.PinnedAt == "" {
					t.Fatal("expected pinned_at to be set")
				}
			},
		},
		{
			name:    "empty id returns error",
			id:      "",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:    "whitespace id returns error",
			id:      "   ",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name: "repo error propagated",
			id:   "sess-1",
			pinFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{pinSessionFn: tt.pinFn}
			uc := newTestUc(repo)
			got, err := uc.Pin(context.Background(), tt.id)
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

func TestUnpin(t *testing.T) {
	tests := []struct {
		name        string
		id          string
		unpinFn     func(ctx context.Context, id string) (Session, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got Session)
	}{
		{
			name: "valid unpin clears pinned_at",
			id:   "sess-1",
			unpinFn: func(_ context.Context, id string) (Session, error) {
				return Session{ID: id, PinnedAt: ""}, nil
			},
			checkResult: func(t *testing.T, got Session) {
				if got.ID != "sess-1" {
					t.Fatalf("expected id sess-1, got %s", got.ID)
				}
				if got.PinnedAt != "" {
					t.Fatalf("expected pinned_at to be empty, got %s", got.PinnedAt)
				}
			},
		},
		{
			name:    "empty id returns error",
			id:      "",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name:    "whitespace id returns error",
			id:      "   ",
			wantErr: true,
			wantMsg: "session id is required",
		},
		{
			name: "repo error propagated",
			id:   "sess-1",
			unpinFn: func(_ context.Context, _ string) (Session, error) {
				return Session{}, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{unpinSessionFn: tt.unpinFn}
			uc := newTestUc(repo)
			got, err := uc.Unpin(context.Background(), tt.id)
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

func TestAppendChatMessage(t *testing.T) {
	tests := []struct {
		name    string
		sid     string
		msg     ChatMessage
		bump    bool
		appendFn func(ctx context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error
		wantErr bool
	}{
		{
			name: "valid append assistant message",
			sid:  "sess-1",
			msg: ChatMessage{
				ID: "msg-1", Role: "assistant", ContentMarkdown: "Hello!",
			},
			bump: false,
			appendFn: func(_ context.Context, sessionID string, msg ChatMessage, bumpModelCall bool) error {
				if sessionID != "sess-1" {
					t.Fatalf("expected sessionID sess-1, got %s", sessionID)
				}
				if msg.ID != "msg-1" {
					t.Fatalf("expected msg ID msg-1, got %s", msg.ID)
				}
				if bumpModelCall {
					t.Fatal("expected bumpModelCall false")
				}
				return nil
			},
		},
		{
			name: "valid append with bump model call",
			sid:  "sess-1",
			msg: ChatMessage{
				ID: "msg-2", Role: "assistant", ContentMarkdown: "Response",
			},
			bump: true,
			appendFn: func(_ context.Context, _ string, _ ChatMessage, bumpModelCall bool) error {
				if !bumpModelCall {
					t.Fatal("expected bumpModelCall true")
				}
				return nil
			},
		},
		{
			name: "repo error propagated",
			sid:  "sess-1",
			msg:  ChatMessage{ID: "msg-3", Role: "assistant", ContentMarkdown: "Hi"},
			appendFn: func(_ context.Context, _ string, _ ChatMessage, _ bool) error {
				return errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{appendChatMessageFn: tt.appendFn}
			uc := newTestUc(repo)
			err := uc.AppendChatMessage(context.Background(), tt.sid, tt.msg, tt.bump)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

func TestUpdateChatMessageStatus(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		messageID  string
		status     string
		errMsg     string
		statusFn   func(ctx context.Context, sessionID, messageID, status, errorMessage string) error
		wantErr    bool
		wantMsg    string
	}{
		{
			name:      "valid status update",
			sessionID: "sess-1",
			messageID: "msg-1",
			status:    "completed",
			errMsg:    "",
			statusFn: func(_ context.Context, sessionID, messageID, status, errorMessage string) error {
				if sessionID != "sess-1" {
					t.Fatalf("expected sessionID sess-1, got %s", sessionID)
				}
				if messageID != "msg-1" {
					t.Fatalf("expected messageID msg-1, got %s", messageID)
				}
				if status != "completed" {
					t.Fatalf("expected status completed, got %s", status)
				}
				return nil
			},
		},
		{
			name:      "empty session_id returns error",
			sessionID: "",
			messageID: "msg-1",
			status:    "completed",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "empty message_id returns error",
			sessionID: "sess-1",
			messageID: "",
			status:    "completed",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "whitespace session_id returns error",
			sessionID: "   ",
			messageID: "msg-1",
			status:    "completed",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "whitespace message_id returns error",
			sessionID: "sess-1",
			messageID: "   ",
			status:    "completed",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "empty status returns error",
			sessionID: "sess-1",
			messageID: "msg-1",
			status:    "",
			wantErr:   true,
			wantMsg:   "status is required",
		},
		{
			name:      "whitespace status returns error",
			sessionID: "sess-1",
			messageID: "msg-1",
			status:    "   ",
			wantErr:   true,
			wantMsg:   "status is required",
		},
		{
			name:      "repo error propagated",
			sessionID: "sess-1",
			messageID: "msg-1",
			status:    "failed",
			errMsg:    "timeout",
			statusFn: func(_ context.Context, _, _, _, _ string) error {
				return errors.New("db error")
			},
			wantErr: true,
		},
		{
			name:      "error message trimmed and passed through",
			sessionID: "sess-1",
			messageID: "msg-1",
			status:    "failed",
			errMsg:    "  connection lost  ",
			statusFn: func(_ context.Context, _, _, status, errorMessage string) error {
				if errorMessage != "connection lost" {
					t.Fatalf("expected trimmed error message 'connection lost', got %q", errorMessage)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{updateChatMessageStatusFn: tt.statusFn}
			uc := newTestUc(repo)
			err := uc.UpdateChatMessageStatus(context.Background(), tt.sessionID, tt.messageID, tt.status, tt.errMsg)
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

func TestUpdateMessageFeedback(t *testing.T) {
	tests := []struct {
		name       string
		sessionID  string
		messageID  string
		rating     string
		comment    string
		feedbackFn func(ctx context.Context, sessionID, messageID, rating, comment string) error
		wantErr    bool
		wantMsg    string
	}{
		{
			name:      "valid positive feedback",
			sessionID: "sess-1",
			messageID: "msg-1",
			rating:    "positive",
			comment:   "Great answer",
			feedbackFn: func(_ context.Context, _, _, rating, comment string) error {
				if rating != "positive" {
					t.Fatalf("expected rating positive, got %s", rating)
				}
				if comment != "Great answer" {
					t.Fatalf("expected comment 'Great Answer', got %q", comment)
				}
				return nil
			},
		},
		{
			name:      "valid negative feedback",
			sessionID: "sess-1",
			messageID: "msg-1",
			rating:    "negative",
			comment:   "Wrong info",
			feedbackFn: func(_ context.Context, _, _, rating, comment string) error {
				if rating != "negative" {
					t.Fatalf("expected rating negative, got %s", rating)
				}
				return nil
			},
		},
		{
			name:      "rating is lowercased",
			sessionID: "sess-1",
			messageID: "msg-1",
			rating:    "POSITIVE",
			feedbackFn: func(_ context.Context, _, _, rating, _ string) error {
				if rating != "positive" {
					t.Fatalf("expected rating lowercased to 'positive', got %q", rating)
				}
				return nil
			},
		},
		{
			name:      "invalid rating returns error",
			sessionID: "sess-1",
			messageID: "msg-1",
			rating:    "neutral",
			wantErr:   true,
			wantMsg:   "rating must be positive or negative",
		},
		{
			name:      "empty rating returns error",
			sessionID: "sess-1",
			messageID: "msg-1",
			rating:    "",
			wantErr:   true,
			wantMsg:   "rating must be positive or negative",
		},
		{
			name:      "empty session_id returns error",
			sessionID: "",
			messageID: "msg-1",
			rating:    "positive",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "empty message_id returns error",
			sessionID: "sess-1",
			messageID: "",
			rating:    "positive",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "whitespace session_id returns error",
			sessionID: "   ",
			messageID: "msg-1",
			rating:    "positive",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "whitespace message_id returns error",
			sessionID: "sess-1",
			messageID: "   ",
			rating:    "positive",
			wantErr:   true,
			wantMsg:   "session_id and message_id are required",
		},
		{
			name:      "repo error propagated",
			sessionID: "sess-1",
			messageID: "msg-1",
			rating:    "positive",
			feedbackFn: func(_ context.Context, _, _, _, _ string) error {
				return errors.New("db error")
			},
			wantErr: true,
		},
		{
			name:      "comment is trimmed",
			sessionID: "sess-1",
			messageID: "msg-1",
			rating:    "negative",
			comment:   "  too slow  ",
			feedbackFn: func(_ context.Context, _, _, _, comment string) error {
				if comment != "too slow" {
					t.Fatalf("expected trimmed comment 'too slow', got %q", comment)
				}
				return nil
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{updateMessageFeedbackJSONFn: tt.feedbackFn}
			uc := newTestUc(repo)
			err := uc.UpdateMessageFeedback(context.Background(), tt.sessionID, tt.messageID, tt.rating, tt.comment)
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
