package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// ensureOffboardTable 建离职申请单表（镜像迁移 20261325 DDL；测试自包含，
// 与 EnsureSessionMemorySchema 同策略）。db 必须与测试 Data 同一 schema——
// SetupTestPG 每次调用生成独立 schema，故用 setupEnvelopeTestRepo 返回的
// 句柄，不得另开 SetupTestPG。
func ensureOffboardTable(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS secops_offboard_requests (
			id             TEXT PRIMARY KEY,
			target_user_id INTEGER NOT NULL,
			requester_id   INTEGER NOT NULL DEFAULT 0,
			approver_id    INTEGER NOT NULL DEFAULT 0,
			status         TEXT NOT NULL DEFAULT 'requested',
			export_checksum TEXT NOT NULL DEFAULT '',
			exported_at    TEXT NOT NULL DEFAULT '',
			steps_json     TEXT NOT NULL DEFAULT '{}',
			created_at     TEXT NOT NULL DEFAULT '',
			updated_at     TEXT NOT NULL DEFAULT ''
		)`); err != nil {
		t.Fatalf("create secops_offboard_requests: %v", err)
	}
}

func newOffboardTestRepo(t *testing.T) (*secopsOffboardRepo, context.Context) {
	t.Helper()
	d, _, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	ensureOffboardTable(t, ctx, db)
	return &secopsOffboardRepo{data: d}, ctx
}

func TestSecopsOffboardRepo_CRUD(t *testing.T) {
	repo, ctx := newOffboardTestRepo(t)
	const uid = int64(880001)

	req := &biz.SecopsOffboardRequest{TargetUserID: uid, RequesterID: 7, Status: biz.OffboardStatusRequested}
	if err := repo.Insert(ctx, req); err != nil {
		t.Fatalf("insert: %v", err)
	}
	if req.ID == "" {
		t.Fatal("insert must assign id")
	}
	if req.CreatedAt.IsZero() {
		t.Fatal("insert must stamp created_at")
	}

	latest, err := repo.GetLatestByTargetUser(ctx, uid)
	if err != nil || latest == nil {
		t.Fatalf("latest: %v %v", latest, err)
	}
	if latest.TargetUserID != uid || latest.RequesterID != 7 || latest.Status != biz.OffboardStatusRequested {
		t.Fatalf("latest mismatch: %+v", latest)
	}

	missing, err := repo.GetLatestByTargetUser(ctx, 999999)
	if err != nil || missing != nil {
		t.Fatalf("missing must be (nil,nil): %v %v", missing, err)
	}

	// 审批：approver 上列，note/approved_at 入 steps_json。
	at := time.Now().UTC().Truncate(time.Second)
	if err := repo.MarkDecision(ctx, req.ID, 9, biz.OffboardStatusApproved, "同意导出", at); err != nil {
		t.Fatalf("mark decision: %v", err)
	}
	latest, _ = repo.GetLatestByTargetUser(ctx, uid)
	if latest.Status != biz.OffboardStatusApproved || latest.ApproverID != 9 {
		t.Fatalf("decision not persisted: %+v", latest)
	}
	if latest.Note != "同意导出" {
		t.Fatalf("note from steps_json: %q", latest.Note)
	}
	if !latest.ApprovedAt.Equal(at) {
		t.Fatalf("approved_at from steps_json: %v want %v", latest.ApprovedAt, at)
	}

	// 导出校验和。
	if err := repo.MarkExported(ctx, req.ID, "abc123", at.Add(time.Minute)); err != nil {
		t.Fatalf("mark exported: %v", err)
	}
	latest, _ = repo.GetLatestByTargetUser(ctx, uid)
	if latest.ExportChecksum != "abc123" || latest.ExportedAt.IsZero() {
		t.Fatalf("exported fields: %+v", latest)
	}

	// 完成态。
	if err := repo.MarkCompleted(ctx, req.ID, at.Add(2*time.Minute)); err != nil {
		t.Fatalf("mark completed: %v", err)
	}
	latest, _ = repo.GetLatestByTargetUser(ctx, uid)
	if latest.Status != biz.OffboardStatusCompleted {
		t.Fatalf("status=%q", latest.Status)
	}

	// ListRecent / ListPending。
	pending, err := repo.ListPending(ctx, 10)
	if err != nil {
		t.Fatalf("list pending: %v", err)
	}
	for _, p := range pending {
		if p.ID == req.ID {
			t.Fatal("completed request must not appear in pending")
		}
	}
	recent, err := repo.ListRecent(ctx, 10)
	if err != nil || len(recent) == 0 || recent[0].ID != req.ID {
		t.Fatalf("recent: %v %v", recent, err)
	}

	// 同用户新单插入（旧单 completed 后不冲突）。
	req2 := &biz.SecopsOffboardRequest{TargetUserID: uid, RequesterID: 8, Status: biz.OffboardStatusRequested}
	if err := repo.Insert(ctx, req2); err != nil {
		t.Fatalf("second insert: %v", err)
	}
	latest, _ = repo.GetLatestByTargetUser(ctx, uid)
	if latest.ID != req2.ID {
		t.Fatalf("latest must be second request: %+v", latest)
	}
}
