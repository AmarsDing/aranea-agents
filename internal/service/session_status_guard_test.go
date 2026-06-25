package service

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/session"
	sessstatus "aranea-agents/internal/biz/session"
	"aranea-agents/pkg/loggateway"
)

type stubSessionReader struct {
	biz.SessionReader
	sessions []biz.Session
}

func (s *stubSessionReader) ListSessionsForBatch(_ context.Context, q biz.SessionSearchQuery) ([]biz.Session, error) {
	if q.Status != string(session.SessionStatusRunning) {
		return nil, nil
	}
	return s.sessions, nil
}

type stubSessionWriter struct {
	biz.SessionWriter
	updated map[string]biz.SessionUpdateFields
}

func newStubSessionWriter() *stubSessionWriter {
	return &stubSessionWriter{updated: make(map[string]biz.SessionUpdateFields)}
}

func (s *stubSessionWriter) UpdateSession(_ context.Context, id string, fields biz.SessionUpdateFields) (biz.Session, error) {
	s.updated[id] = fields
	return biz.Session{ID: id}, nil
}

type stubSessionRepo struct {
	biz.SessionRepo
	reader *stubSessionReader
	writer *stubSessionWriter
}

func newStubSessionRepoForGuard(runningIDs ...string) *stubSessionRepo {
	sessions := make([]biz.Session, len(runningIDs))
	for i, id := range runningIDs {
		sessions[i] = biz.Session{ID: id, Status: string(session.SessionStatusRunning)}
	}
	return &stubSessionRepo{
		reader: &stubSessionReader{sessions: sessions},
		writer: newStubSessionWriter(),
	}
}

func (s *stubSessionRepo) GetSessionByID(_ context.Context, id string) (biz.Session, error) {
	for _, sess := range s.reader.sessions {
		if sess.ID == id {
			return sess, nil
		}
	}
	return biz.Session{}, nil
}

func (s *stubSessionRepo) ListSessionsForBatch(ctx context.Context, q biz.SessionSearchQuery) ([]biz.Session, error) {
	return s.reader.ListSessionsForBatch(ctx, q)
}

func (s *stubSessionRepo) UpdateSession(ctx context.Context, id string, fields biz.SessionUpdateFields) (biz.Session, error) {
	return s.writer.UpdateSession(ctx, id, fields)
}

func (s *stubSessionRepo) SearchSessions(_ context.Context, _ biz.SessionSearchQuery) (biz.SessionListResult, error) {
	return biz.SessionListResult{}, nil
}

func newTestGuardUC(repo *stubSessionRepo) *biz.SessionUsecase {
	return biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
}

func TestSessionStatusGuard_OnStartup(t *testing.T) {
	repo := newStubSessionRepoForGuard("s1", "s2")
	uc := newTestGuardUC(repo)
	g := NewSessionStatusGuard(uc, nil, nil, nil, loggateway.NewNoop())

	err := g.OnStartup(context.Background())
	if err != nil {
		t.Fatalf("OnStartup: %v", err)
	}
	if len(repo.writer.updated) != 2 {
		t.Errorf("expected 2 sessions updated, got %d", len(repo.writer.updated))
	}
	for id, fields := range repo.writer.updated {
		if fields.Status == nil || *fields.Status != string(sessstatus.SessionStatusInterrupted) {
			t.Errorf("session %s: expected status=interrupted, got %v", id, *fields.Status)
		}
		if fields.StatusReason == nil || *fields.StatusReason != string(sessstatus.StatusReasonUnexpectedShutdown) {
			t.Errorf("session %s: expected reason=unexpected_shutdown, got %v", id, *fields.StatusReason)
		}
	}
}

func TestSessionStatusGuard_OnStartup_NoRunning(t *testing.T) {
	repo := newStubSessionRepoForGuard()
	uc := newTestGuardUC(repo)
	g := NewSessionStatusGuard(uc, nil, nil, nil, loggateway.NewNoop())

	err := g.OnStartup(context.Background())
	if err != nil {
		t.Fatalf("OnStartup: %v", err)
	}
	if len(repo.writer.updated) != 0 {
		t.Errorf("expected 0 sessions updated, got %d", len(repo.writer.updated))
	}
}

func TestSessionStatusGuard_OnShutdown(t *testing.T) {
	repo := newStubSessionRepoForGuard("s1")
	uc := newTestGuardUC(repo)
	g := NewSessionStatusGuard(uc, nil, nil, nil, loggateway.NewNoop())

	err := g.OnShutdown(context.Background())
	if err != nil {
		t.Fatalf("OnShutdown: %v", err)
	}
	if len(repo.writer.updated) != 1 {
		t.Fatalf("expected 1 session updated, got %d", len(repo.writer.updated))
	}
	fields, ok := repo.writer.updated["s1"]
	if !ok {
		t.Fatal("expected s1 to be updated")
	}
	if fields.Status == nil || *fields.Status != string(sessstatus.SessionStatusInterrupted) {
		t.Errorf("expected status=interrupted, got %v", fields.Status)
	}
	if fields.StatusReason == nil || *fields.StatusReason != string(sessstatus.StatusReasonServerShutdown) {
		t.Errorf("expected reason=server_shutdown, got %v", fields.StatusReason)
	}
}
