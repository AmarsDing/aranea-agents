package session

import (
	"context"
	"errors"
	"testing"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (r *testRepo) CompressSessionInTx(ctx context.Context, sessionID string, fn func(ctx context.Context) error) error {
	if r.compressSessionInTxFn != nil {
		return r.compressSessionInTxFn(ctx, sessionID, fn)
	}
	return r.mockSessionRepo.CompressSessionInTx(ctx, sessionID, fn)
}

func (r *testRepo) TryIncrementCompressVersion(ctx context.Context, sessionID string) (int64, error) {
	if r.tryIncrementCompressVersionFn != nil {
		return r.tryIncrementCompressVersionFn(ctx, sessionID)
	}
	return r.mockSessionRepo.TryIncrementCompressVersion(ctx, sessionID)
}

func (r *testRepo) SessionSummaryExists(ctx context.Context, sessionID string, fromTurn, toTurn int) (bool, error) {
	if r.sessionSummaryExistsFn != nil {
		return r.sessionSummaryExistsFn(ctx, sessionID, fromTurn, toTurn)
	}
	return r.mockSessionRepo.SessionSummaryExists(ctx, sessionID, fromTurn, toTurn)
}

func (r *testRepo) DeleteSessionsByAgentID(ctx context.Context, agentID string) error {
	if r.deleteSessionsByAgentIDFn != nil {
		return r.deleteSessionsByAgentIDFn(ctx, agentID)
	}
	return r.mockSessionRepo.DeleteSessionsByAgentID(ctx, agentID)
}

func (r *testRepo) BumpSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	if r.bumpSessionRevisionFn != nil {
		return r.bumpSessionRevisionFn(ctx, sessionID)
	}
	return r.mockSessionRepo.BumpSessionRevision(ctx, sessionID)
}

func (r *testRepo) GetSessionRevision(ctx context.Context, sessionID string) (int64, error) {
	if r.getSessionRevisionFn != nil {
		return r.getSessionRevisionFn(ctx, sessionID)
	}
	return r.mockSessionRepo.GetSessionRevision(ctx, sessionID)
}

func (r *testRepo) UpdateSessionTurn(ctx context.Context, id string, fields SessionTurnUpdateFields) (SessionTurn, error) {
	if r.updateSessionTurnFn != nil {
		return r.updateSessionTurnFn(ctx, id, fields)
	}
	return r.mockSessionRepo.UpdateSessionTurn(ctx, id, fields)
}

func (r *testRepo) UpsertChatActivityMessage(ctx context.Context, sessionID string, msg ChatMessage) error {
	if r.upsertChatActivityMessageFn != nil {
		return r.upsertChatActivityMessageFn(ctx, sessionID, msg)
	}
	return r.mockSessionRepo.UpsertChatActivityMessage(ctx, sessionID, msg)
}

func (r *testRepo) ListMessagesAfterTurn(ctx context.Context, sessionID string, afterTurn int) ([]ChatMessage, error) {
	if r.listMessagesAfterTurnFn != nil {
		return r.listMessagesAfterTurnFn(ctx, sessionID, afterTurn)
	}
	return r.mockSessionRepo.ListMessagesAfterTurn(ctx, sessionID, afterTurn)
}

func (r *testRepo) ListMessagesRecent(ctx context.Context, sessionID string, limit int) ([]ChatMessage, error) {
	if r.listMessagesRecentFn != nil {
		return r.listMessagesRecentFn(ctx, sessionID, limit)
	}
	return r.mockSessionRepo.ListMessagesRecent(ctx, sessionID, limit)
}

func (r *testRepo) ListMessagesAfterRevision(ctx context.Context, sessionID string, afterRevision int64) ([]ChatMessage, error) {
	if r.listMessagesAfterRevisionFn != nil {
		return r.listMessagesAfterRevisionFn(ctx, sessionID, afterRevision)
	}
	return r.mockSessionRepo.ListMessagesAfterRevision(ctx, sessionID, afterRevision)
}

func TestApplyStateDelta_GetError(t *testing.T) {
	repo := &testRepo{
		getSessionStateFn: func(_ context.Context, _ string) (map[string]string, error) {
			return nil, errors.New("db error")
		},
	}
	uc := newTestUc(repo)
	err := uc.ApplyStateDelta(context.Background(), "sess-1", StateDelta{Operation: "set", Path: "k", ValueJSON: "v"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestApplyStateDelta_SaveError(t *testing.T) {
	repo := &testRepo{
		getSessionStateFn: func(_ context.Context, _ string) (map[string]string, error) {
			return map[string]string{}, nil
		},
		saveSessionStateFn: func(_ context.Context, _ string, _ map[string]string) error {
			return errors.New("db error")
		},
	}
	uc := newTestUc(repo)
	err := uc.ApplyStateDelta(context.Background(), "sess-1", StateDelta{Operation: "set", Path: "k", ValueJSON: "v"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestListMessagesPaged(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		limit     int
		offset    int
		countFn   func(_ context.Context, sid string) (int, error)
		listFn    func(_ context.Context, sid string, limit, offset int) ([]ChatMessage, error)
		wantErr   bool
		wantMsg   string
		check     func(t *testing.T, got MessageListResult)
	}{
		{
			"empty session id returns error",
			"", 0, 0, nil, nil, true, "session id is required", nil,
		},
		{
			"whitespace session id returns error",
			"  ", 0, 0, nil, nil, true, "session id is required", nil,
		},
		{
			"zero limit defaults to 100",
			"sess-1", 0, 0,
			func(_ context.Context, _ string) (int, error) { return 0, nil },
			func(_ context.Context, _ string, limit, _ int) ([]ChatMessage, error) {
				if limit != 100 {
					t.Errorf("limit = %d, want 100", limit)
				}
				return nil, nil
			},
			false, "", nil,
		},
		{
			"limit capped at 500",
			"sess-1", 600, 0,
			func(_ context.Context, _ string) (int, error) { return 0, nil },
			func(_ context.Context, _ string, limit, _ int) ([]ChatMessage, error) {
				if limit != 500 {
					t.Errorf("limit = %d, want 500", limit)
				}
				return nil, nil
			},
			false, "", nil,
		},
		{
			"negative offset reset to 0",
			"sess-1", 10, -5,
			func(_ context.Context, _ string) (int, error) { return 0, nil },
			func(_ context.Context, _ string, _, offset int) ([]ChatMessage, error) {
				if offset != 0 {
					t.Errorf("offset = %d, want 0", offset)
				}
				return nil, nil
			},
			false, "", nil,
		},
		{
			"returns total and items",
			"sess-1", 10, 0,
			func(_ context.Context, _ string) (int, error) { return 42, nil },
			func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{{ID: "m1"}}, nil
			},
			false, "",
			func(t *testing.T, got MessageListResult) {
				if got.Total != 42 {
					t.Errorf("Total = %d, want 42", got.Total)
				}
				if len(got.Items) != 1 {
					t.Errorf("len(Items) = %d, want 1", len(got.Items))
				}
			},
		},
		{
			"count error propagated",
			"sess-1", 10, 0,
			func(_ context.Context, _ string) (int, error) { return 0, errors.New("db error") },
			nil, true, "", nil,
		},
		{
			"list error propagated",
			"sess-1", 10, 0,
			func(_ context.Context, _ string) (int, error) { return 10, nil },
			func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return nil, errors.New("db error")
			},
			true, "", nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{}
			if tt.countFn != nil {
				repo.countMessagesBySessionFn = tt.countFn
			}
			if tt.listFn != nil {
				repo.listMessagesBySessionFn = tt.listFn
			}
			uc := newTestUc(repo)
			got, err := uc.ListMessagesPaged(context.Background(), tt.sessionID, tt.limit, tt.offset)
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
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestListMessages(t *testing.T) {
	repo := &testRepo{
		countMessagesBySessionFn: func(_ context.Context, _ string) (int, error) {
			return 2, nil
		},
		listMessagesBySessionFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
			return []ChatMessage{{ID: "m1"}, {ID: "m2"}}, nil
		},
	}
	uc := newTestUc(repo)
	got, err := uc.ListMessages(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(got) != 2 {
		t.Errorf("len = %d, want 2", len(got))
	}
}

func TestListMessagesAfterTurn(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
		wantMsg   string
	}{
		{"empty session id returns error", "", true, "session id is required"},
		{"whitespace session id returns error", "  ", true, "session id is required"},
		{"valid id passes", "sess-1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := newTestUc(&testRepo{})
			_, err := uc.ListMessagesAfterTurn(context.Background(), tt.sessionID, 5)
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

func TestListMessagesRecent(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		wantErr   bool
		wantMsg   string
	}{
		{"empty session id returns error", "", true, "session id is required"},
		{"whitespace session id returns error", "  ", true, "session id is required"},
		{"valid id passes", "sess-1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := newTestUc(&testRepo{})
			_, err := uc.ListMessagesRecent(context.Background(), tt.sessionID, 10)
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

func TestListParticipants_NilUsecase(t *testing.T) {
	var uc *SessionUsecase
	got, err := uc.ListParticipants(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil, got %v", got)
	}
}

func TestUpdateTurn(t *testing.T) {
	tests := []struct {
		name    string
		id      string
		wantErr bool
		wantMsg string
	}{
		{"empty id returns error", "", true, "turn id is required"},
		{"whitespace id returns error", "  ", true, "turn id is required"},
		{"valid id passes", "turn-1", false, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			uc := newTestUc(&testRepo{})
			_, err := uc.UpdateTurn(context.Background(), tt.id, SessionTurnUpdateFields{})
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

func TestBumpSessionRevision(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		bumpFn    func(_ context.Context, sid string) (int64, error)
		wantErr   bool
		wantMsg   string
		check     func(t *testing.T, got int64)
	}{
		{"empty session id returns error", "", nil, true, "session id is required", nil},
		{"whitespace session id returns error", "  ", nil, true, "session id is required", nil},
		{
			"valid id delegates to repo",
			"sess-1",
			func(_ context.Context, sid string) (int64, error) {
				if sid != "sess-1" {
					t.Errorf("sid = %q, want %q", sid, "sess-1")
				}
				return 7, nil
			},
			false, "",
			func(t *testing.T, got int64) {
				if got != 7 {
					t.Errorf("got = %d, want 7", got)
				}
			},
		},
		{
			"repo error propagated",
			"sess-1",
			func(_ context.Context, _ string) (int64, error) {
				return 0, errors.New("db error")
			},
			true, "", nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{bumpSessionRevisionFn: tt.bumpFn}
			uc := newTestUc(repo)
			got, err := uc.BumpSessionRevision(context.Background(), tt.sessionID)
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
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestGetSessionRevision(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		revFn     func(_ context.Context, sid string) (int64, error)
		wantErr   bool
		wantMsg   string
		check     func(t *testing.T, got int64)
	}{
		{"empty session id returns error", "", nil, true, "session id is required", nil},
		{"whitespace session id returns error", "  ", nil, true, "session id is required", nil},
		{
			"valid id delegates to repo",
			"sess-1",
			func(_ context.Context, _ string) (int64, error) { return 42, nil },
			false, "",
			func(t *testing.T, got int64) {
				if got != 42 {
					t.Errorf("got = %d, want 42", got)
				}
			},
		},
		{
			"repo error propagated",
			"sess-1",
			func(_ context.Context, _ string) (int64, error) {
				return 0, errors.New("db error")
			},
			true, "", nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{getSessionRevisionFn: tt.revFn}
			uc := newTestUc(repo)
			got, err := uc.GetSessionRevision(context.Background(), tt.sessionID)
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
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestDeleteByAgent(t *testing.T) {
	tests := []struct {
		name    string
		agentID string
		delFn   func(_ context.Context, aid string) error
		wantErr bool
		wantMsg string
	}{
		{"empty agent id returns error", "", nil, true, "agent_id is required"},
		{"whitespace agent id returns error", "  ", nil, true, "agent_id is required"},
		{
			"valid agent id delegates to repo",
			"agent-1",
			func(_ context.Context, aid string) error {
				if aid != "agent-1" {
					t.Errorf("aid = %q, want %q", aid, "agent-1")
				}
				return nil
			},
			false, "",
		},
		{
			"repo error propagated",
			"agent-1",
			func(_ context.Context, _ string) error {
				return errors.New("db error")
			},
			true, "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{deleteSessionsByAgentIDFn: tt.delFn}
			uc := newTestUc(repo)
			err := uc.DeleteByAgent(context.Background(), tt.agentID)
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

func TestUpsertChatActivityMessage(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		msg       ChatMessage
		getFn     func(_ context.Context, id string) (Session, error)
		upsertFn  func(_ context.Context, sid string, msg ChatMessage) error
		wantErr   bool
		wantMsg   string
	}{
		{
			"empty session id returns error",
			"", ChatMessage{ID: "msg-1"}, nil, nil, true, "session id is required",
		},
		{
			"empty message id returns error",
			"sess-1", ChatMessage{ID: ""}, nil, nil, true, "message id is required",
		},
		{
			"session not found returns error",
			"sess-1", ChatMessage{ID: "msg-1"},
			func(_ context.Context, _ string) (Session, error) {
				return Session{}, kerrors.NotFound("SESSION", "not found")
			}, nil, true, "",
		},
		{
			"valid upsert passes",
			"sess-1", ChatMessage{ID: "msg-1"},
			func(_ context.Context, id string) (Session, error) {
				return Session{ID: id}, nil
			},
			func(_ context.Context, sid string, msg ChatMessage) error {
				if sid != "sess-1" {
					t.Errorf("sid = %q, want %q", sid, "sess-1")
				}
				if msg.ID != "msg-1" {
					t.Errorf("msg.ID = %q, want %q", msg.ID, "msg-1")
				}
				return nil
			},
			false, "",
		},
		{
			"upsert repo error propagated",
			"sess-1", ChatMessage{ID: "msg-1"},
			func(_ context.Context, id string) (Session, error) {
				return Session{ID: id}, nil
			},
			func(_ context.Context, _ string, _ ChatMessage) error {
				return errors.New("db error")
			},
			true, "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{}
			if tt.getFn != nil {
				repo.getSessionByIDFn = tt.getFn
			}
			if tt.upsertFn != nil {
				repo.upsertChatActivityMessageFn = tt.upsertFn
			}
			uc := newTestUc(repo)
			err := uc.UpsertChatActivityMessage(context.Background(), tt.sessionID, tt.msg)
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

func TestListMessagesAfterRevision(t *testing.T) {
	tests := []struct {
		name      string
		sessionID string
		getFn     func(_ context.Context, id string) (Session, error)
		listFn    func(_ context.Context, sid string, rev int64) ([]ChatMessage, error)
		wantErr   bool
		wantMsg   string
		check     func(t *testing.T, got []ChatMessage)
	}{
		{
			"empty session id returns error",
			"", nil, nil, true, "session id is required", nil,
		},
		{
			"whitespace session id returns error",
			"  ", nil, nil, true, "session id is required", nil,
		},
		{
			"session not found returns error",
			"sess-1",
			func(_ context.Context, _ string) (Session, error) {
				return Session{}, kerrors.NotFound("SESSION", "not found")
			}, nil, true, "", nil,
		},
		{
			"valid id returns messages",
			"sess-1",
			func(_ context.Context, id string) (Session, error) {
				return Session{ID: id}, nil
			},
			func(_ context.Context, _ string, rev int64) ([]ChatMessage, error) {
				if rev != 1 {
					t.Errorf("rev = %d, want 1", rev)
				}
				return []ChatMessage{{ID: "m1"}}, nil
			},
			false, "",
			func(t *testing.T, got []ChatMessage) {
				if len(got) != 1 {
					t.Errorf("len = %d, want 1", len(got))
				}
			},
		},
		{
			"list repo error propagated",
			"sess-1",
			func(_ context.Context, id string) (Session, error) {
				return Session{ID: id}, nil
			},
			func(_ context.Context, _ string, _ int64) ([]ChatMessage, error) {
				return nil, errors.New("db error")
			},
			true, "", nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{}
			if tt.getFn != nil {
				repo.getSessionByIDFn = tt.getFn
			}
			if tt.listFn != nil {
				repo.listMessagesAfterRevisionFn = tt.listFn
			}
			uc := newTestUc(repo)
			got, err := uc.ListMessagesAfterRevision(context.Background(), tt.sessionID, 1)
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
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestCompressSessionInTx(t *testing.T) {
	called := false
	fnCalled := false
	repo := &testRepo{
		compressSessionInTxFn: func(_ context.Context, _ string, fn func(ctx context.Context) error) error {
			called = true
			return fn(context.Background())
		},
	}
	uc := newTestUc(repo)
	err := uc.CompressSessionInTx(context.Background(), "sess-1", func(ctx context.Context) error {
		fnCalled = true
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !called {
		t.Error("CompressSessionInTx should delegate to repo")
	}
	if !fnCalled {
		t.Error("callback function should be invoked")
	}
}

func TestCompressSessionInTx_RepoError(t *testing.T) {
	repo := &testRepo{
		compressSessionInTxFn: func(_ context.Context, _ string, _ func(ctx context.Context) error) error {
			return errors.New("tx error")
		},
	}
	uc := newTestUc(repo)
	err := uc.CompressSessionInTx(context.Background(), "sess-1", func(ctx context.Context) error {
		return nil
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestCompressSessionInTx_CallbackError(t *testing.T) {
	repo := &testRepo{
		compressSessionInTxFn: func(_ context.Context, _ string, fn func(ctx context.Context) error) error {
			return fn(context.Background())
		},
	}
	uc := newTestUc(repo)
	err := uc.CompressSessionInTx(context.Background(), "sess-1", func(ctx context.Context) error {
		return errors.New("callback error")
	})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestTryIncrementCompressVersion(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(_ context.Context, sid string) (int64, error)
		wantErr bool
		check   func(t *testing.T, got int64)
	}{
		{
			"returns version from repo",
			func(_ context.Context, _ string) (int64, error) { return 42, nil },
			false,
			func(t *testing.T, got int64) {
				if got != 42 {
					t.Errorf("got = %d, want 42", got)
				}
			},
		},
		{
			"repo error propagated",
			func(_ context.Context, _ string) (int64, error) {
				return 0, errors.New("db error")
			},
			true, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{tryIncrementCompressVersionFn: tt.fn}
			uc := newTestUc(repo)
			got, err := uc.TryIncrementCompressVersion(context.Background(), "sess-1")
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestSessionSummaryExists(t *testing.T) {
	tests := []struct {
		name    string
		fn      func(_ context.Context, _ string, _, _ int) (bool, error)
		wantErr bool
		check   func(t *testing.T, got bool)
	}{
		{
			"returns true from repo",
			func(_ context.Context, _ string, _, _ int) (bool, error) { return true, nil },
			false,
			func(t *testing.T, got bool) {
				if !got {
					t.Error("expected true")
				}
			},
		},
		{
			"returns false from repo",
			func(_ context.Context, _ string, _, _ int) (bool, error) { return false, nil },
			false,
			func(t *testing.T, got bool) {
				if got {
					t.Error("expected false")
				}
			},
		},
		{
			"repo error propagated",
			func(_ context.Context, _ string, _, _ int) (bool, error) {
				return false, errors.New("db error")
			},
			true, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{sessionSummaryExistsFn: tt.fn}
			uc := newTestUc(repo)
			got, err := uc.SessionSummaryExists(context.Background(), "sess-1", 1, 5)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestClampMessageListLimit(t *testing.T) {
	tests := []struct {
		name  string
		input int
		want  int
	}{
		{"zero defaults to 100", 0, 100},
		{"negative defaults to 100", -5, 100},
		{"within range passes through", 50, 50},
		{"at max passes through", 500, 500},
		{"over max capped to 500", 600, 500},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := clampMessageListLimit(tt.input)
			if got != tt.want {
				t.Errorf("clampMessageListLimit(%d) = %d, want %d", tt.input, got, tt.want)
			}
		})
	}
}
