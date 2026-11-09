package data

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// copyFactIntegDeps 把真实 l3FactRepo 的 GetFactRowsByIDs 接到
// MemoryAdminDeps 窄口上——biz.CopyFactRow 全链路走真 PG（信封加密、
// 指纹重算、审计落库都是生产实现）。
type copyFactIntegDeps struct {
	biz.MemoryAdminDeps
	repo *l3FactRepo
}

func (c copyFactIntegDeps) GetFactRowsByIDs(ctx context.Context, ids []string) ([][]byte, error) {
	return c.repo.GetFactRowsByIDs(ctx, ids)
}

// ListFactRows 返回空——CopyFactRow 内部的 best-effort 冲突检测需要它，
// 但本测试不断言冲突行为，空结果即短路（与 biz 单测的 fake 同策略）。
func (c copyFactIntegDeps) ListFactRows(context.Context, string, string, string, string, string, string, int32, int32) ([][]byte, int32, int32, int32, error) {
	return nil, 0, 0, 0, nil
}

func newCopyTestUsecase(t *testing.T, d *Data, r *l3FactRepo) *biz.MemoryAdminUsecase {
	t.Helper()
	uc := biz.NewMemoryAdminUsecase(copyFactIntegDeps{repo: r}, nil, nil, r, loggateway.NewNoop())
	uc.SetMemoryFactCopyAuditRecorder(NewMonitorAuditRepo(d))
	return uc
}

// TestFactCopy_UserNoteToWorkspace 是 acceptance #3 的端到端钉死：user 域
// note → workspace 域，全程走真实 UpsertFactRow 写信封。断言四件事：
// 目标行 scope 正确且信封重打（新指纹、content_enc 非空且不泄明文）、
// 血缘 metadata、源行原封不动、audit_logs 落 memory.fact.copy 且无明文。
func TestFactCopy_UserNoteToWorkspace(t *testing.T) {
	_, r, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	uc := newCopyTestUsecase(t, r.data, r)

	const stmt = "跨域复制测试：项目 Phoenix 的发版窗口是周四"
	srcRaw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType:       "user",
		ScopeID:         "user-1",
		UserID:          "user-1",
		Statement:       stmt,
		DetailsMarkdown: "细节：与运维窗口对齐",
		FactKind:        "note",
		Confidence:      1.0,
		SourceKind:      "manual",
	})
	if err != nil {
		t.Fatalf("source upsert: %v", err)
	}
	srcID := envelopeFactID(t, srcRaw)

	var srcFP, srcEnc string
	if err := db.QueryRowContext(ctx,
		`SELECT fingerprint, content_enc FROM memory_facts WHERE id = $1`, srcID).
		Scan(&srcFP, &srcEnc); err != nil {
		t.Fatalf("read source: %v", err)
	}
	if srcEnc == "" {
		t.Fatal("source must be envelope-encrypted")
	}

	raw, err := uc.CopyFactRow(ctx, biz.FactCopy{
		SourceFactID:    srcID,
		TargetScopeType: "workspace",
		TargetScopeID:   "ws-9",
		Actor:           "user-1",
	})
	if err != nil {
		t.Fatalf("CopyFactRow: %v", err)
	}
	newID := envelopeFactID(t, raw)
	if newID == srcID {
		t.Fatal("copy must produce a new row id")
	}

	var (
		newScopeType, newScopeID, newFP, newEnc, newMeta string
		newStmt, newDetails                              string
	)
	if err := db.QueryRowContext(ctx,
		`SELECT scope_type, scope_id, fingerprint, content_enc, metadata_json, statement, details_markdown FROM memory_facts WHERE id = $1`, newID).
		Scan(&newScopeType, &newScopeID, &newFP, &newEnc, &newMeta, &newStmt, &newDetails); err != nil {
		t.Fatalf("read copied row: %v", err)
	}
	if newScopeType != "workspace" || newScopeID != "ws-9" {
		t.Fatalf("target scope=%s/%s", newScopeType, newScopeID)
	}
	if newFP == srcFP {
		t.Fatal("fingerprint must be recomputed for target scope")
	}
	if newEnc == "" {
		t.Fatal("copied row must carry a fresh envelope")
	}
	if strings.Contains(newEnc, "Phoenix") || strings.Contains(newEnc, "周四") {
		t.Fatal("copied envelope leaks plaintext")
	}
	if newStmt != stmt {
		t.Fatalf("carrier statement mismatch: %q", newStmt)
	}
	if newDetails != "细节：与运维窗口对齐" {
		t.Fatalf("carrier details mismatch: %q", newDetails)
	}
	var meta map[string]any
	if err := json.Unmarshal([]byte(newMeta), &meta); err != nil {
		t.Fatalf("metadata json: %v", err)
	}
	if meta["copied_from_fact_id"] != srcID {
		t.Fatalf("copy lineage missing: %v", meta["copied_from_fact_id"])
	}

	// 源行原封不动。
	var srcStmtAfter string
	if err := db.QueryRowContext(ctx,
		`SELECT statement FROM memory_facts WHERE id = $1`, srcID).Scan(&srcStmtAfter); err != nil {
		t.Fatalf("source after copy: %v", err)
	}
	if srcStmtAfter != stmt {
		t.Fatal("source row must be untouched by copy")
	}

	// 审计落库：action 钉死 + detail 有血缘与 sha256、无明文。
	var action, detail, actor string
	if err := db.QueryRowContext(ctx,
		`SELECT action, detail, actor FROM audit_logs WHERE resource_id = $1`, newID).
		Scan(&action, &detail, &actor); err != nil {
		t.Fatalf("audit lookup: %v", err)
	}
	if action != "memory.fact.copy" {
		t.Fatalf("audit action=%q", action)
	}
	if actor != "user-1" {
		t.Fatalf("audit actor=%q", actor)
	}
	if strings.Contains(detail, "Phoenix") {
		t.Fatal("audit detail leaks plaintext")
	}
	var ad map[string]any
	if err := json.Unmarshal([]byte(detail), &ad); err != nil {
		t.Fatalf("audit detail json: %v", err)
	}
	if ad["source_fact_id"] != srcID || ad["target_scope_type"] != "workspace" {
		t.Fatalf("audit lineage: %v", ad)
	}
	if sha, _ := ad["statement_sha256"].(string); len(sha) != 16 {
		t.Fatalf("statement_sha256=%q", sha)
	}
}

// TestFactCopy_PIIFlaggedUserSourceRefused：user 域 pii_flag=1 的行处于复核
// 隔离态，复制即"洗 PII 进共享域"，biz 层必须拒绝且不落任何目标行/审计。
func TestFactCopy_PIIFlaggedUserSourceRefused(t *testing.T) {
	_, r, db := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	uc := newCopyTestUsecase(t, r.data, r)

	srcRaw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType: "user",
		ScopeID:   "user-1",
		UserID:    "user-1",
		Statement: "PII 隔离内容：身份证 110101",
		FactKind:  "note",
	})
	if err != nil {
		t.Fatalf("source upsert: %v", err)
	}
	srcID := envelopeFactID(t, srcRaw)
	if _, err := db.ExecContext(ctx,
		`UPDATE memory_facts SET pii_flag = 1 WHERE id = $1`, srcID); err != nil {
		t.Fatalf("flag pii: %v", err)
	}

	_, err = uc.CopyFactRow(ctx, biz.FactCopy{
		SourceFactID:    srcID,
		TargetScopeType: "workspace",
		TargetScopeID:   "ws-pii-refused",
	})
	if err == nil {
		t.Fatal("copy of PII-quarantined user fact must fail")
	}

	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM memory_facts WHERE scope_type = 'workspace' AND scope_id = 'ws-pii-refused'`).Scan(&n); err != nil {
		t.Fatalf("count target: %v", err)
	}
	if n != 0 {
		t.Fatalf("no target rows allowed on refused copy, got %d", n)
	}
	// 审计断言按本测试专属目标域过滤（包内测试共享 schema，避免跨用例串扰）。
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM audit_logs WHERE action = 'memory.fact.copy' AND detail LIKE '%ws-pii-refused%'`).Scan(&n); err != nil {
		t.Fatalf("count audit: %v", err)
	}
	if n != 0 {
		t.Fatalf("refused copy must not audit, got %d", n)
	}
}

// TestFactCopy_PeekFactScope：真实 PG 上的 scope 三元组读取（service 层
// 源侧 ACL 的最小披露输入）。
func TestFactCopy_PeekFactScope(t *testing.T) {
	_, r, _ := setupEnvelopeTestRepo(t, 0x2a)
	ctx := context.Background()
	uc := newCopyTestUsecase(t, r.data, r)

	raw, err := r.UpsertFactRow(ctx, biz.FactUpsert{
		ScopeType: "agent",
		ScopeID:   "peek-agent",
		AgentID:   "peek-agent",
		Statement: "peek 源",
		FactKind:  "note",
	})
	if err != nil {
		t.Fatalf("upsert: %v", err)
	}
	id := envelopeFactID(t, raw)
	st, sid, pii, err := uc.PeekFactScope(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if st != "agent" || sid != "peek-agent" || pii {
		t.Fatalf("peek=%q/%q pii=%v", st, sid, pii)
	}
}
