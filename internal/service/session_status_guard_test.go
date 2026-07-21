package service

import (
	"context"
	"errors"
	"testing"
	"time"

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

func newTestGuardUC(repo biz.SessionRepo) *biz.SessionUsecase {
	return biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
}

// ctxAwareSessionRepo wraps stubSessionRepo and fails calls when ctx is done,
// simulating real DB behavior under a canceled context (2026-07-21 P1-5 F2).
type ctxAwareSessionRepo struct {
	*stubSessionRepo
}

func (r *ctxAwareSessionRepo) ListSessionsForBatch(ctx context.Context, q biz.SessionSearchQuery) ([]biz.Session, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return r.stubSessionRepo.ListSessionsForBatch(ctx, q)
}

func (r *ctxAwareSessionRepo) UpdateSession(ctx context.Context, id string, fields biz.SessionUpdateFields) (biz.Session, error) {
	if err := ctx.Err(); err != nil {
		return biz.Session{}, err
	}
	return r.stubSessionRepo.UpdateSession(ctx, id, fields)
}

// stubV2RecoveryRepo records FailOrphanedInFlight calls (2026-07-21 P1-5 F1).
type stubV2RecoveryRepo struct {
	called bool
	stats  biz.V2RecoveryStats
	err    error
}

func (s *stubV2RecoveryRepo) FailOrphanedInFlight(_ context.Context, _ time.Time) (biz.V2RecoveryStats, error) {
	s.called = true
	return s.stats, s.err
}

func TestSessionStatusGuard_OnStartup(t *testing.T) {
	repo := newStubSessionRepoForGuard("s1", "s2")
	uc := newTestGuardUC(repo)
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, loggateway.NewNoop())

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
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, loggateway.NewNoop())

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
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, loggateway.NewNoop())

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

// 2026-07-21 P1-5 F2：Kratos 在 Stop 钩子前已取消 server ctx，OnShutdown
// 必须脱离取消信号完成 running→interrupted 转移，否则重启后 sessions 永远
// 卡在 running。
func TestSessionStatusGuard_OnShutdown_CanceledContext(t *testing.T) {
	repo := &ctxAwareSessionRepo{newStubSessionRepoForGuard("s1")}
	uc := newTestGuardUC(repo)
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, loggateway.NewNoop())

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Kratos cancels server ctx before invoking Stop hooks
	if err := g.OnShutdown(ctx); err != nil {
		t.Fatalf("OnShutdown with canceled ctx: %v", err)
	}
	if len(repo.writer.updated) != 1 {
		t.Fatalf("expected 1 session updated despite canceled ctx, got %d", len(repo.writer.updated))
	}
}

// 2026-07-21 P1-5 F1：OnStartup 必须调用 v2 orphaned-recovery。
func TestSessionStatusGuard_OnStartup_RecoversV2Entities(t *testing.T) {
	repo := newStubSessionRepoForGuard()
	uc := newTestGuardUC(repo)
	rec := &stubV2RecoveryRepo{stats: biz.V2RecoveryStats{Tasks: 2, Steps: 3}}
	g := NewSessionStatusGuard(uc, nil, nil, nil, rec, loggateway.NewNoop())

	if err := g.OnStartup(context.Background()); err != nil {
		t.Fatalf("OnStartup: %v", err)
	}
	if !rec.called {
		t.Fatal("expected FailOrphanedInFlight to be called on startup")
	}
}

// v2 恢复失败不得阻断启动（与 team/orchestration 恢复一致的 non-fatal 语义）。
func TestSessionStatusGuard_OnStartup_V2RecoveryErrorNonFatal(t *testing.T) {
	repo := newStubSessionRepoForGuard()
	uc := newTestGuardUC(repo)
	rec := &stubV2RecoveryRepo{err: errors.New("db down")}
	g := NewSessionStatusGuard(uc, nil, nil, nil, rec, loggateway.NewNoop())

	if err := g.OnStartup(context.Background()); err != nil {
		t.Fatalf("OnStartup should be non-fatal on v2 recovery error: %v", err)
	}
}
