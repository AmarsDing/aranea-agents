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

func (s *stubSessionWriter) UpdateSessionMetadataKey(context.Context, string, string, string) error {
	return nil
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
	return biz.NewSessionUsecase(repo, nil, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop(), nil)
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
	called      bool
	stats       biz.V2RecoveryStats
	interrupted []biz.InterruptedTaskRef
	err         error
}

func (s *stubV2RecoveryRepo) FailOrphanedInFlight(_ context.Context, _ time.Time) (biz.V2RecoveryStats, []biz.InterruptedTaskRef, error) {
	s.called = true
	return s.stats, s.interrupted, s.err
}

func TestSessionStatusGuard_OnStartup(t *testing.T) {
	repo := newStubSessionRepoForGuard("s1", "s2")
	uc := newTestGuardUC(repo)
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, nil, loggateway.NewNoop())

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
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, nil, loggateway.NewNoop())

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
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, nil, loggateway.NewNoop())

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
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, nil, loggateway.NewNoop())

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
	g := NewSessionStatusGuard(uc, nil, nil, nil, rec, nil, loggateway.NewNoop())

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
	g := NewSessionStatusGuard(uc, nil, nil, nil, rec, nil, loggateway.NewNoop())

	if err := g.OnStartup(context.Background()); err != nil {
		t.Fatalf("OnStartup should be non-fatal on v2 recovery error: %v", err)
	}
}

// stubDurableEscalator records EscalateAllActiveToDurable calls (2026-07-22 L2).
type stubDurableEscalator struct {
	called bool
	n      int
}

func (s *stubDurableEscalator) EscalateAllActiveToDurable(_ context.Context) int {
	s.called = true
	return s.n
}

// stubGuardEventBus captures published events (L3 notice assertions).
type stubGuardEventBus struct {
	published []biz.Event
}

func (s *stubGuardEventBus) Publish(_ context.Context, e biz.Event) {
	s.published = append(s.published, e)
}

func (s *stubGuardEventBus) Subscribe(_ biz.EventSubscribeOptions) (<-chan biz.Event, func()) {
	return make(chan biz.Event), func() {}
}

// 2026-07-22 L3-F3：启动恢复后必须对每个有 interrupted task 的 session 发
// 一条 task_interrupted system.notice（按 session 分组，meta 带可续跑 task 列表）。
func TestSessionStatusGuard_OnStartup_NotifiesInterruptedTasks(t *testing.T) {
	repo := newStubSessionRepoForGuard()
	uc := newTestGuardUC(repo)
	rec := &stubV2RecoveryRepo{
		stats: biz.V2RecoveryStats{Tasks: 3},
		interrupted: []biz.InterruptedTaskRef{
			{TaskID: "t-1", SessionID: "s-a", UserMessage: "写报告"},
			{TaskID: "t-2", SessionID: "s-a", UserMessage: "翻译"},
			{TaskID: "t-3", SessionID: "s-b", UserMessage: "查资料"},
		},
	}
	bus := &stubGuardEventBus{}
	g := NewSessionStatusGuard(uc, nil, nil, bus, rec, nil, loggateway.NewNoop())

	if err := g.OnStartup(context.Background()); err != nil {
		t.Fatalf("OnStartup: %v", err)
	}
	// 2 sessions → 2 notices, grouped per session.
	var notices []*biz.SystemNoticeEvent
	for _, e := range bus.published {
		if n, ok := e.(*biz.SystemNoticeEvent); ok && n.NoticeType == "task_interrupted" {
			notices = append(notices, n)
		}
	}
	if len(notices) != 2 {
		t.Fatalf("expected 2 task_interrupted notices, got %d (%+v)", len(notices), bus.published)
	}
	bySession := map[string]*biz.SystemNoticeEvent{}
	for _, n := range notices {
		bySession[n.SpiritSessionID()] = n
	}
	sa, ok := bySession["s-a"]
	if !ok {
		t.Fatalf("missing notice for s-a: %+v", bySession)
	}
	tasks, _ := sa.Meta["tasks"].([]map[string]any)
	if len(tasks) != 2 {
		t.Errorf("s-a notice tasks = %+v, want 2 entries", sa.Meta["tasks"])
	}
	if resumable, _ := sa.Meta["resumable"].(bool); !resumable {
		t.Errorf("s-a notice resumable = %v, want true", sa.Meta["resumable"])
	}
	if _, ok := bySession["s-b"]; !ok {
		t.Errorf("missing notice for s-b")
	}
}

// 2026-07-22 L2：OnShutdown 必须先把活跃 interactive run durable 化（写
// checkpoint 供重启续跑），再做 running→interrupted 批量兜底。
func TestSessionStatusGuard_OnShutdown_EscalatesToDurable(t *testing.T) {
	repo := newStubSessionRepoForGuard("s1")
	uc := newTestGuardUC(repo)
	esc := &stubDurableEscalator{n: 1}
	g := NewSessionStatusGuard(uc, nil, nil, nil, nil, esc, loggateway.NewNoop())

	if err := g.OnShutdown(context.Background()); err != nil {
		t.Fatalf("OnShutdown: %v", err)
	}
	if !esc.called {
		t.Fatal("expected EscalateAllActiveToDurable to be called on shutdown")
	}
	if len(repo.writer.updated) != 1 {
		t.Fatalf("expected batch interrupt still applied, got %d updates", len(repo.writer.updated))
	}
}
