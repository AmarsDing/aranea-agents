package data

// D6 V4 脱敏五闸台账链路测试（验收 #4：提案→闸门→审批（不可自批/影子规则/
// 审计失败回滚）→激活灰度）。表结构镜像迁移 20261327 的
// mask_proposals/mask_samples（mask_rules URL 复用 secops_mask_repo_test.go
// 的 ensureMaskRuleTable）。db 必须与测试 Data 同一 schema——复用
// setupEnvelopeTestRepo 返回句柄（同 engiine tests 策略）。

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
)

func ensureMaskProposalTables(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	stmts := []string{
		`CREATE TABLE IF NOT EXISTS mask_proposals (
			id                  TEXT PRIMARY KEY,
			name                TEXT NOT NULL DEFAULT '',
			entity              TEXT NOT NULL,
			pattern             TEXT NOT NULL DEFAULT '',
			wordlist_json       TEXT NOT NULL DEFAULT '[]',
			operator            TEXT NOT NULL DEFAULT 'placeholder',
			source              TEXT NOT NULL DEFAULT 'manual',
			status              TEXT NOT NULL DEFAULT 'proposed',
			sample_recall       REAL,
			sample_detail_json  TEXT NOT NULL DEFAULT '',
			static_findings_json TEXT NOT NULL DEFAULT '',
			created_by          TEXT NOT NULL DEFAULT '',
			reviewed_by         TEXT NOT NULL DEFAULT '',
			reviewed_at         TEXT NOT NULL DEFAULT '',
			created_at          TEXT NOT NULL DEFAULT '',
			updated_at          TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS mask_samples (
			id       TEXT PRIMARY KEY,
			entity   TEXT NOT NULL,
			text     TEXT NOT NULL,
			positive INTEGER NOT NULL DEFAULT 1
		)`,
	}
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("ensure mask proposal tables: %v", err)
		}
	}
}

func newMaskProposalTestRepo(t *testing.T) (biz.SecopsMaskProposalRepo, context.Context, *sql.DB) {
	t.Helper()
	d, _, db := setupEnvelopeTestRepo(t, 0x2b)
	ctx := context.Background()
	ensureMaskRuleTable(t, ctx, db)
	ensureMaskProposalTables(t, ctx, db)
	return NewSecopsMaskProposalRepo(d).(*secopsMaskProposalRepo), ctx, db
}

func insertMaskProposalRow(t *testing.T, ctx context.Context, db *sql.DB, id, status, createdBy string) {
	t.Helper()
	now := time.Now().UTC().Format(time.RFC3339)
	_, err := db.ExecContext(ctx,
		`INSERT INTO mask_proposals (id, name, entity, pattern, wordlist_json, operator, source, status, created_by, created_at, updated_at)
		 VALUES ($1, $2, $2, '\b1[3-9]\d{9}\b', '[]', 'placeholder', 'manual', $3, $4, $5, $5)
		 ON CONFLICT (id) DO NOTHING`,
		id, "手机号", status, createdBy, now)
	if err != nil {
		t.Fatalf("insert proposal %s: %v", id, err)
	}
}

func insertMaskSampleRow(t *testing.T, ctx context.Context, db *sql.DB, id, entity string, positive int) {
	t.Helper()
	_, err := db.ExecContext(ctx,
		`INSERT INTO mask_samples (id, entity, text, positive) VALUES ($1, $2, '样本', $3) ON CONFLICT (id) DO NOTHING`,
		id, entity, positive)
	if err != nil {
		t.Fatalf("insert sample %s: %v", id, err)
	}
}

type maskProposalTestAudit struct {
	entries []biz.AuditLog
	err     error
}

func (a *maskProposalTestAudit) InsertAuditLog(_ context.Context, entry biz.AuditLog) error {
	if a.err != nil {
		return a.err
	}
	a.entries = append(a.entries, entry)
	return nil
}

func fetchMaskRule(t *testing.T, ctx context.Context, db *sql.DB, id string) (enabled, shadow, rollout, version int, ok bool) {
	t.Helper()
	err := db.QueryRowContext(ctx,
		`SELECT enabled, shadow, rollout_pct, version FROM mask_rules WHERE id = $1`, id).
		Scan(&enabled, &shadow, &rollout, &version)
	if err == sql.ErrNoRows {
		return 0, 0, 0, 0, false
	}
	if err != nil {
		t.Fatalf("fetch rule %s: %v", id, err)
	}
	return enabled, shadow, rollout, version, true
}

func TestSecopsMaskProposalRepo_CreateGetList(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	p := biz.MaskProposal{
		ID: "mp_a", Name: "手机号", Entity: "手机号", Pattern: `\b1[3-9]\d{9}\b`,
		WordlistJSON: "[\"phoenix\"]", Operator: "placeholder", Source: "manual",
		Status: biz.MaskProposalStatusProposed, CreatedBy: "u1",
		CreatedAt: time.Now().UTC(),
	}
	if err := repo.CreateProposal(ctx, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := repo.GetProposal(ctx, "mp_a")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Entity != "手机号" || got.Status != "proposed" || got.WordlistJSON != "[\"phoenix\"]" {
		t.Fatalf("got = %+v", got)
	}
	if got.SampleRecall != nil {
		t.Fatalf("recall = %v, want nil (NULL)", got.SampleRecall)
	}
	if got.CreatedAt.IsZero() {
		t.Fatal("created_at parse failed")
	}
	if _, err := repo.GetProposal(ctx, "mp_missing"); !apierror.IsCode(err, apierror.CodeNotFound) {
		t.Fatalf("missing must be NotFound, got %v", err)
	}

	// list：status 过滤 + 倒序 + 分页 + total。
	insertMaskProposalRow(t, ctx, db, "mp_b", biz.MaskProposalStatusPendingApproval, "u1")
	insertMaskProposalRow(t, ctx, db, "mp_c", biz.MaskProposalStatusProposed, "u1")
	items, total, err := repo.ListProposals(ctx, "proposed", 10, 0)
	if err != nil || total != 2 || len(items) != 2 {
		t.Fatalf("list: items=%d total=%d err=%v", len(items), total, err)
	}
	items, total, err = repo.ListProposals(ctx, "", 1, 1)
	if err != nil || total != 3 || len(items) != 1 {
		t.Fatalf("page: items=%d total=%d err=%v", len(items), total, err)
	}
}

func TestSecopsMaskProposalRepo_ListSamplesByEntity(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskSampleRow(t, ctx, db, "ms_a1", "手机号", 1)
	insertMaskSampleRow(t, ctx, db, "ms_a2", "手机号", 0)
	insertMaskSampleRow(t, ctx, db, "ms_b1", "邮箱", 1)
	samples, err := repo.ListSamplesByEntity(ctx, "手机号")
	if err != nil || len(samples) != 2 {
		t.Fatalf("samples = %v err=%v", samples, err)
	}
	if !samples[0].Positive || samples[1].Positive {
		t.Fatalf("positive mapping broken: %+v", samples)
	}
	if got, _ := repo.ListSamplesByEntity(ctx, "无此实体"); len(got) != 0 {
		t.Fatalf("empty entity: %v", got)
	}
}

func TestSecopsMaskProposalRepo_UpdateGatesCAS(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskProposalRow(t, ctx, db, "mp_1", biz.MaskProposalStatusProposed, "u1")

	ok, err := repo.UpdateGates(ctx, "mp_1", biz.MaskProposalStatusProposed, biz.MaskProposalStatusPendingApproval, 1.0, `[]`, "[]")
	if err != nil || !ok {
		t.Fatalf("CAS first: ok=%v err=%v", ok, err)
	}
	// 已从 proposed 迁走 → 同一 fromStatus 的并发写落空。
	ok, err = repo.UpdateGates(ctx, "mp_1", biz.MaskProposalStatusProposed, biz.MaskProposalStatusSampleFailed, 0, "[]", "[]")
	if err != nil || ok {
		t.Fatalf("stale CAS must miss: ok=%v err=%v", ok, err)
	}
	got, err := repo.GetProposal(ctx, "mp_1")
	if err != nil || got.Status != biz.MaskProposalStatusPendingApproval {
		t.Fatalf("status = %v err=%v", got.Status, err)
	}
	if got.SampleRecall == nil || *got.SampleRecall != 1.0 {
		t.Fatalf("recall = %v", got.SampleRecall)
	}
}

func TestSecopsMaskProposalRepo_ReviewApproveCreatesShadowRule(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskProposalRow(t, ctx, db, "mp_1", biz.MaskProposalStatusPendingApproval, "u1")
	sink := &maskProposalTestAudit{}
	ruleID := biz.MaskRuleIDForProposal("mp_1")
	audit := func(txCtx context.Context) error {
		return sink.InsertAuditLog(txCtx, biz.AuditLog{Action: biz.SecopsMaskProposalActionApprove, ResourceID: "mp_1"})
	}
	p, err := repo.ReviewProposal(ctx, "mp_1", "u2", true, ruleID, audit)
	if err != nil {
		t.Fatalf("approve: %v", err)
	}
	if p.Status != biz.MaskProposalStatusShadow || p.ReviewedBy != "u2" {
		t.Fatalf("p = %+v", p)
	}
	enabled, shadow, rollout, version, ok := fetchMaskRule(t, ctx, db, ruleID)
	if !ok || enabled != 1 || shadow != 1 || rollout != 100 || version != 1 {
		t.Fatalf("shadow rule = enabled:%d shadow:%d rollout:%d version:%d ok:%v", enabled, shadow, rollout, version, ok)
	}
	if len(sink.entries) != 1 {
		t.Fatalf("audit entries = %d", len(sink.entries))
	}
}

func TestSecopsMaskProposalRepo_ReviewSelfForbidden(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskProposalRow(t, ctx, db, "mp_1", biz.MaskProposalStatusPendingApproval, "u1")
	_, err := repo.ReviewProposal(ctx, "mp_1", "u1", true, "mr_x", func(context.Context) error { return nil })
	if !apierror.IsCode(err, apierror.CodeForbidden) {
		t.Fatalf("self review must be Forbidden, got %v", err)
	}
	if _, _, _, _, ok := fetchMaskRule(t, ctx, db, "mr_x"); ok {
		t.Fatal("self-approval must not insert rule")
	}
}

func TestSecopsMaskProposalRepo_ReviewStatusConflict(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskProposalRow(t, ctx, db, "mp_1", biz.MaskProposalStatusShadow, "u1")
	_, err := repo.ReviewProposal(ctx, "mp_1", "u2", true, "mr_x", func(context.Context) error { return nil })
	if !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("non pending_approval must be FailedPrecondition, got %v", err)
	}
	if _, _, _, _, ok := fetchMaskRule(t, ctx, db, "mr_x"); ok {
		t.Fatal("shadow rule must not be inserted on status guard")
	}
}

func TestSecopsMaskProposalRepo_ReviewAuditFailureRollsBack(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskProposalRow(t, ctx, db, "mp_1", biz.MaskProposalStatusPendingApproval, "u1")
	ruleID := biz.MaskRuleIDForProposal("mp_1")
	_, err := repo.ReviewProposal(ctx, "mp_1", "u2", true, ruleID, func(context.Context) error {
		return sql.ErrTxDone // 任一审计失败
	})
	if err == nil || !strings.Contains(err.Error(), "audit persist failed") {
		t.Fatalf("audit failure must abort, got %v", err)
	}
	// 回滚：影子规则未落库、提案状态未变。
	if _, _, _, _, ok := fetchMaskRule(t, ctx, db, ruleID); ok {
		t.Fatal("audit failure must roll back shadow rule")
	}
	got, getErr := repo.GetProposal(ctx, "mp_1")
	if getErr != nil || got.Status != biz.MaskProposalStatusPendingApproval || got.ReviewedBy != "" {
		t.Fatalf("rollback: status=%s reviewed_by=%s err=%v", got.Status, got.ReviewedBy, getErr)
	}
}

func TestSecopsMaskProposalRepo_ActivateFullFlow(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskProposalRow(t, ctx, db, "mp_1", biz.MaskProposalStatusPendingApproval, "u1")
	ruleID := biz.MaskRuleIDForProposal("mp_1")
	if _, err := repo.ReviewProposal(ctx, "mp_1", "u2", true, ruleID, func(context.Context) error { return nil }); err != nil {
		t.Fatalf("approve: %v", err)
	}
	p, err := repo.ActivateProposal(ctx, "mp_1", 30, ruleID, func(context.Context) error { return nil })
	if err != nil {
		t.Fatalf("activate: %v", err)
	}
	if p.Status != biz.MaskProposalStatusActivated {
		t.Fatalf("status = %s", p.Status)
	}
	enabled, shadow, rollout, version, ok := fetchMaskRule(t, ctx, db, ruleID)
	if !ok || enabled != 1 || shadow != 0 || rollout != 30 || version != 2 {
		t.Fatalf("activated rule = enabled:%d shadow:%d rollout:%d version:%d ok:%v", enabled, shadow, rollout, version, ok)
	}
	// 重复激活：proposal 已离开 shadow → FailedPrecondition。
	if _, err := repo.ActivateProposal(ctx, "mp_1", 50, ruleID, func(context.Context) error { return nil }); !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
		t.Fatalf("second activate must be FailedPrecondition, got %v", err)
	}
}

func TestSecopsMaskProposalRepo_ActivateMissingShadowRule(t *testing.T) {
	repo, ctx, db := newMaskProposalTestRepo(t)
	insertMaskProposalRow(t, ctx, db, "mp_1", biz.MaskProposalStatusShadow, "u1")
	_, err := repo.ActivateProposal(ctx, "mp_1", 30, "mr_ghost", func(context.Context) error { return nil })
	if !apierror.IsCode(err, apierror.CodeInternal) {
		t.Fatalf("missing shadow rule must be Internal, got %v", err)
	}
	got, _ := repo.GetProposal(ctx, "mp_1")
	if got.Status != biz.MaskProposalStatusShadow {
		t.Fatalf("rollback: status = %s", got.Status)
	}
}
