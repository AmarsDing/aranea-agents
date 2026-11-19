package data

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/binary"
	"encoding/json"
	"strconv"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

// ── 测试替身（admin 读写 + 会话吊销；真实件需 ent admins 表，本测试聚焦于
// 离职台账 + 记忆事实 + 审计链路，admin 生命周期用可断言替身）──────────────

type offboardFakeAdminReader struct {
	admins map[int64]*biz.Admin
}

func (f offboardFakeAdminReader) FindByID(_ context.Context, id int64) (*biz.Admin, error) {
	if a, ok := f.admins[id]; ok {
		return a, nil
	}
	return nil, biz.ErrAdminNotFound
}
func (f offboardFakeAdminReader) FindByName(context.Context, string) (*biz.Admin, error) {
	return nil, biz.ErrAdminNotFound
}
func (f offboardFakeAdminReader) FindByEmail(context.Context, string) (*biz.Admin, error) {
	return nil, biz.ErrAdminNotFound
}
func (f offboardFakeAdminReader) ListAdmins(context.Context, ...biz.ListOption) ([]*biz.Admin, error) {
	return nil, nil
}

type offboardFakeAdminWriter struct {
	byID map[int64]*biz.Admin
}

func (f *offboardFakeAdminWriter) CreateAdmin(_ context.Context, a *biz.Admin) (*biz.Admin, error) {
	f.byID[a.ID] = a
	return a, nil
}
func (f *offboardFakeAdminWriter) UpdateAdmin(_ context.Context, a *biz.Admin) (*biz.Admin, error) {
	cp := *a
	f.byID[a.ID] = &cp
	return a, nil
}
func (f *offboardFakeAdminWriter) DeleteAdmin(context.Context, int64) error { return nil }

type offboardFakeRevoker struct {
	calls []int64
}

func (f *offboardFakeRevoker) RevokeSessions(_ context.Context, userID int64) (int64, error) {
	f.calls = append(f.calls, userID)
	return int64(len(f.calls)), nil
}

func offboardUIDStr(uid int64) string { return strconv.FormatInt(uid, 10) }

// offboardTestUID 生成每次运行唯一的离职目标 uid。测试 PG 为共享库，
// 审计表不受 schema 隔离保护（见 ensureOffboardAuditTable 注释），
// 固定 uid 会让跨运行的残留审计行混入断言。
func offboardTestUID() int64 {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return 885001
	}
	return 885000001 + int64(binary.BigEndian.Uint64(b[:])%900000)
}

// ensureOffboardAuditTable 在测试 schema 内强制重建 12 列审计表。
// 背景（2026-11-10 复跑现形）：EnsureSessionMemorySchema 的 audit_logs
// 是 7 列精简 DDL 且带 IF NOT EXISTS——共享库 public.audit_logs（12 列）
// 已存在时 PG 按 search_path 判定"已存在"而跳过，导致全部测试的审计写
// 落到 public、跨运行互相可见。这里 DROP+CREATE 把 12 列表钉进私有
// schema（search_path 首位），恢复隔离。
func ensureOffboardAuditTable(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `DROP TABLE IF EXISTS audit_logs`); err != nil {
		t.Fatalf("drop audit_logs: %v", err)
	}
	if _, err := db.ExecContext(ctx, `CREATE TABLE audit_logs (
		id TEXT PRIMARY KEY,
		action TEXT NOT NULL DEFAULT '',
		resource TEXT NOT NULL DEFAULT '',
		resource_id TEXT NOT NULL DEFAULT '',
		request_id TEXT NOT NULL DEFAULT '',
		detail TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		actor TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT '',
		user_agent TEXT NOT NULL DEFAULT '',
		severity TEXT NOT NULL DEFAULT '',
		metadata_json TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatalf("create audit_logs: %v", err)
	}
}

// TestSecopsOffboardFiveStepChain 是 acceptance #4（离职五步全链路）的端到端
// 钉死：freeze→request→approve(不可自批)→export→delete(前置已导出)，
// 外加负路径（未审批导出 / 未导出先删 / 重复申请 / 自批）。全程走真实
// UpsertFactRow 写信封、真实 audit_logs 落库。
func TestSecopsOffboardFiveStepChain(t *testing.T) {
	d, r, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	ensureOffboardTable(t, ctx, db)
	ensureOffboardAuditTable(t, ctx, db)

	uid := offboardTestUID() // 离职目标（每次运行唯一，防共享库残留行干扰）
	const (
		sysadminA = int64(71) // 申请人
		sysadminB = int64(72) // 审批人
	)
	uidEarly := uid + 1 // 未导出先删负路径
	admins := map[int64]*biz.Admin{
		uid:       {ID: uid, Name: "off-target", Status: biz.AdminStatusActive},
		uidEarly:  {ID: uidEarly, Name: "off-early", Status: biz.AdminStatusActive},
		sysadminA: {ID: sysadminA, Name: "sysA"},
		sysadminB: {ID: sysadminB, Name: "sysB"},
	}
	writer := &offboardFakeAdminWriter{byID: map[int64]*biz.Admin{}}
	revoker := &offboardFakeRevoker{}
	uc := biz.NewSecopsOffboardUsecase(
		offboardFakeAdminReader{admins: admins}, writer,
		&secopsOffboardRepo{data: d}, r, r, nil)
	uc.SetSessionRevoker(revoker)
	uc.SetSecopsOffboardAuditRecorder(NewMonitorAuditRepo(d))

	ctxA := auth.NewContext(ctx, &auth.Auth{UserID: sysadminA, Roles: []string{"sysadmin"}})
	ctxB := auth.NewContext(ctx, &auth.Auth{UserID: sysadminB, Roles: []string{"sysadmin"}})

	// 离职目标不存在 → NotFound。
	if err := uc.FreezeUser(ctxA, 424242, sysadminA); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("freeze missing user: %v", err)
	}

	// ── 第一步：冻结 ──
	if err := uc.FreezeUser(ctxA, uid, sysadminA); err != nil {
		t.Fatalf("freeze: %v", err)
	}
	if got := writer.byID[uid].Status; got != biz.AdminStatusFrozen {
		t.Fatalf("status=%q", got)
	}
	if len(revoker.calls) != 1 || revoker.calls[0] != uid {
		t.Fatalf("revoke calls=%v", revoker.calls)
	}

	// ── 第二步：申请（重复申请 → Conflict）──
	req, err := uc.RequestOffboardExport(ctxA, uid, sysadminA)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if req.ID == "" || req.Status != biz.OffboardStatusRequested {
		t.Fatalf("req: %+v", req)
	}
	if _, err := uc.RequestOffboardExport(ctxA, uid, sysadminA); !apierror.IsCode(err, apierror.CodeConflict) {
		t.Fatalf("duplicate request must conflict: %v", err)
	}

	// ── 负路径：无申请删除 → Forbidden；未审批导出 → Forbidden ──
	if err := uc.DeleteOffboardedUser(ctxA, uidEarly, sysadminA); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("delete without request must forbid: %v", err)
	}
	if _, err := uc.GetOffboardExport(ctxA, uid, sysadminA); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("export before approve must forbid: %v", err)
	}

	// ── 第三步：审批（自批 → Forbidden；他人审批 → approved）──
	if _, err := uc.ApproveOffboardExport(ctxA, uid, sysadminA, true, "自批"); !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("self-approve must forbid: %v", err)
	}
	approved, err := uc.ApproveOffboardExport(ctxB, uid, sysadminB, true, "同意导出")
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if approved.Status != biz.OffboardStatusApproved || approved.ApproverID != sysadminB {
		t.Fatalf("approved: %+v", approved)
	}
	if approved.ApprovedAt.IsZero() {
		t.Fatal("approved_at must be parsed from steps_json")
	}

	// ── 第四步：导出（user 域全量，含 PII 隔离行；审计无明文）──
	seedOffboardFacts(t, ctx, r, uid)
	export, err := uc.GetOffboardExport(ctxB, uid, sysadminB)
	if err != nil {
		t.Fatalf("export: %v", err)
	}
	if export.FactCount != 3 {
		t.Fatalf("fact_count=%d", export.FactCount)
	}
	var items []map[string]any
	if err := json.Unmarshal([]byte(export.ItemsJSON), &items); err != nil {
		t.Fatalf("items json: %v", err)
	}
	joined, _ := json.Marshal(items)
	for _, want := range []string{"离职事实Alpha", "离职事实Beta", "离职事实PII隔离"} {
		if !strings.Contains(string(joined), want) {
			t.Fatalf("export missing %q: %s", want, joined)
		}
	}
	if len(export.Checksum) != 64 {
		t.Fatalf("checksum=%q", export.Checksum)
	}
	var exportDetail string
	if err := db.QueryRowContext(ctx,
		`SELECT detail FROM audit_logs WHERE action='secops.offboard.export' AND resource_id=$1`, offboardUIDStr(uid)).
		Scan(&exportDetail); err != nil {
		t.Fatalf("export audit lookup: %v", err)
	}
	if strings.Contains(exportDetail, "离职事实Alpha") {
		t.Fatal("export audit leaks plaintext")
	}
	var ed map[string]any
	if err := json.Unmarshal([]byte(exportDetail), &ed); err != nil {
		t.Fatalf("export audit json: %v", err)
	}
	if ed["sha256"] != export.Checksum || ed["fact_count"] != float64(3) {
		t.Fatalf("export audit detail: %v", ed)
	}

	// ── 第五步：删除（uidEarly 已审批未导出 → 拒，验收 #4）──
	if _, err := uc.RequestOffboardExport(ctxA, uidEarly, sysadminA); err != nil {
		t.Fatalf("request early: %v", err)
	}
	if _, err := uc.ApproveOffboardExport(ctxB, uidEarly, sysadminB, true, ""); err != nil {
		t.Fatalf("approve early: %v", err)
	}
	seedOffboardFacts(t, ctx, r, uidEarly)
	if err := uc.DeleteOffboardedUser(ctxB, uidEarly, sysadminB); !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("delete before export must fail precondition: %v", err)
	}
	var earlyActive int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memory_facts WHERE scope_type='user' AND scope_id=$1 AND status='active' AND deleted_at=''`, offboardUIDStr(uidEarly)).
		Scan(&earlyActive); err != nil {
		t.Fatalf("count early facts: %v", err)
	}
	if earlyActive != 3 {
		t.Fatalf("refused delete must not clear facts, active=%d", earlyActive)
	}

	// ── 第五步（正路径）：uid 已导出 → 删除成功 ──
	if err := uc.DeleteOffboardedUser(ctxB, uid, sysadminB); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got := writer.byID[uid].Status; got != biz.AdminStatusOffboarded {
		t.Fatalf("status=%q", got)
	}
	var activeLeft int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memory_facts WHERE scope_type='user' AND scope_id=$1 AND status='active' AND deleted_at=''`, offboardUIDStr(uid)).
		Scan(&activeLeft); err != nil {
		t.Fatalf("count facts: %v", err)
	}
	if activeLeft != 0 {
		t.Fatalf("offboard delete must clear user domain, active=%d", activeLeft)
	}
	latest, err := (&secopsOffboardRepo{data: d}).GetLatestByTargetUser(ctx, uid)
	if err != nil || latest.Status != biz.OffboardStatusCompleted {
		t.Fatalf("completed: %+v %v", latest, err)
	}

	// ── 五步审计齐全（freeze/request/approve/export/delete）──
	actions := map[string]bool{}
	rows, err := db.QueryContext(ctx,
		`SELECT action FROM audit_logs WHERE resource_id=$1`, offboardUIDStr(uid))
	if err != nil {
		t.Fatalf("audit rows: %v", err)
	}
	for rows.Next() {
		var a string
		if err := rows.Scan(&a); err != nil {
			t.Fatal(err)
		}
		actions[a] = true
	}
	rows.Close()
	for _, want := range []string{
		"secops.offboard.freeze", "secops.offboard.request",
		"secops.offboard.approve", "secops.offboard.export", "secops.offboard.delete",
	} {
		if !actions[want] {
			t.Fatalf("missing audit action %q (got %v)", want, actions)
		}
	}

	// ── 完成后可再次发起（新单可审批导出）──
	if _, err := uc.RequestOffboardExport(ctxA, uid, sysadminA); err != nil {
		t.Fatalf("re-request after completed: %v", err)
	}
}

func seedOffboardFacts(t *testing.T, ctx context.Context, r *l3FactRepo, uid int64) {
	t.Helper()
	for _, stmt := range []string{"离职事实Alpha", "离职事实Beta", "离职事实PII隔离"} {
		in := biz.FactUpsert{
			ScopeType:  "user",
			ScopeID:    offboardUIDStr(uid),
			UserID:     offboardUIDStr(uid),
			Statement:  stmt,
			FactKind:   "note",
			Confidence: 1.0,
			SourceKind: "manual",
		}
		if _, err := r.UpsertFactRow(ctx, in); err != nil {
			t.Fatalf("seed fact %q: %v", stmt, err)
		}
	}
	// 标记一条 PII 隔离（导出必须包含隔离行）。
	if _, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx,
		`UPDATE memory_facts SET pii_flag=1 WHERE scope_type='user' AND scope_id=$1 AND statement='离职事实PII隔离'`, offboardUIDStr(uid)); err != nil {
		t.Fatalf("flag pii: %v", err)
	}
}
