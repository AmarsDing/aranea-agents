package data

import (
	"context"
	"encoding/json"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// ── 自治理知识图谱 M3 演化时序层 ────────────────────────────────────────────
// fresh/存量两路径收敛：knowledge_fact_version（supersedes 版本链旧段快照）+
// knowledge_governance_proposal（治理提案，M3.2 矛盾仲裁 kind=conflict）。

func setupEvolutionRepo(t *testing.T) *knowledgeRepo {
	t.Helper()
	db := testhelper.SetupTestPGRaw(t)
	if err := EnsureKnowledgeSchema(context.Background(), db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	repo, ok := NewKnowledgeRepo(&Data{rawDB: db, pg: db, pgRead: db, dialect: DialectPostgres},
		loggateway.NewNoop()).(*knowledgeRepo)
	if !ok {
		t.Fatal("NewKnowledgeRepo did not return *knowledgeRepo")
	}
	return repo
}

func TestEnsureKnowledgeSchema_M3EvolutionFreshShape(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	for _, table := range []string{"knowledge_fact_version", "knowledge_governance_proposal"} {
		var reg *string
		if err := db.QueryRowContext(ctx,
			`SELECT to_regclass($1)::text`, table).Scan(&reg); err != nil {
			t.Fatal(err)
		}
		if reg == nil || *reg == "" {
			t.Errorf("%s missing in fresh shape", table)
		}
	}
	// 版本链必备列：fact_id 可空（旧段无 ID 时留空）、superseded_at 非空默认 NOW()。
	for _, col := range []string{"fact_id", "old_body", "new_body", "superseded_at"} {
		var n int
		if err := db.QueryRowContext(ctx,
			`SELECT COUNT(*) FROM information_schema.columns
			 WHERE table_name='knowledge_fact_version' AND column_name=$1`, col).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 1 {
			t.Errorf("knowledge_fact_version.%s missing", col)
		}
	}
	// fresh 建库幂等。
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema rerun: %v", err)
	}
}

func TestMigration20261222_FactVersion(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	// 存量形态：只有 documents（fact_version FK 依赖）。
	if _, err := db.ExecContext(ctx, `CREATE TABLE knowledge_documents (
		id TEXT PRIMARY KEY, collection_id TEXT NOT NULL, source TEXT NOT NULL)`); err != nil {
		t.Fatal(err)
	}
	run := func() {
		if err := executeSQLFileWithDialect(ctx, db,
			"sql/migrations/20261222_knowledge_fact_version.sql", DialectPostgres, loggateway.NewNoop()); err != nil {
			t.Fatalf("migration: %v", err)
		}
	}
	run()
	for _, table := range []string{"knowledge_fact_version", "knowledge_governance_proposal"} {
		var reg *string
		if err := db.QueryRowContext(ctx,
			`SELECT to_regclass($1)::text`, table).Scan(&reg); err != nil {
			t.Fatal(err)
		}
		if reg == nil || *reg == "" {
			t.Errorf("%s missing after migration", table)
		}
	}
	run() // 幂等重跑
}

func TestKnowledgeRepo_InsertFactVersion(t *testing.T) {
	repo := setupEvolutionRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()
	seedRelationDocs(t, raw, "d1")

	// 带 fact_id 留痕。
	if err := repo.InsertFactVersion(ctx, bizknowledge.FactVersion{
		CollectionID: "c1", DocID: "d1", FactID: "fid-1",
		OldBody: "## constraint\n\n旧陈述。", NewBody: "## constraint\n\n新陈述。",
	}); err != nil {
		t.Fatalf("InsertFactVersion: %v", err)
	}
	// 无 fact_id（旧段无 ID）→ NULL 落库。
	if err := repo.InsertFactVersion(ctx, bizknowledge.FactVersion{
		CollectionID: "c1", DocID: "d1",
		OldBody: "## note\n\n无 ID 旧段。", NewBody: "## note\n\n新段。",
	}); err != nil {
		t.Fatalf("InsertFactVersion null fact_id: %v", err)
	}
	// 空 old_body / 空 doc_id → 跳过不落库（防御性契约）。
	if err := repo.InsertFactVersion(ctx, bizknowledge.FactVersion{CollectionID: "c1", DocID: "d1", NewBody: "x"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertFactVersion(ctx, bizknowledge.FactVersion{CollectionID: "c1", OldBody: "x"}); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_fact_version WHERE doc_id='d1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("fact versions = %d, want 2 (empty old_body/doc_id skipped)", n)
	}
	var factID *string
	var oldBody, newBody string
	if err := raw.QueryRowContext(ctx,
		`SELECT fact_id, old_body, new_body FROM knowledge_fact_version WHERE doc_id='d1' ORDER BY id LIMIT 1`).
		Scan(&factID, &oldBody, &newBody); err != nil {
		t.Fatal(err)
	}
	if factID == nil || *factID != "fid-1" {
		t.Fatalf("fact_id = %v, want fid-1", factID)
	}
	if oldBody != "## constraint\n\n旧陈述。" || newBody != "## constraint\n\n新陈述。" {
		t.Fatalf("bodies = %q / %q", oldBody, newBody)
	}
	// 第二条 fact_id 为 NULL。
	var cntNull int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_fact_version WHERE doc_id='d1' AND fact_id IS NULL`).Scan(&cntNull); err != nil {
		t.Fatal(err)
	}
	if cntNull != 1 {
		t.Fatalf("null fact_id rows = %d, want 1", cntNull)
	}
	// superseded_at 默认 NOW() 非空。
	var cntTs int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_fact_version WHERE superseded_at IS NOT NULL`).Scan(&cntTs); err != nil {
		t.Fatal(err)
	}
	if cntTs != 2 {
		t.Fatalf("superseded_at must default NOW(), non-null rows = %d", cntTs)
	}
}

func TestKnowledgeRepo_InsertProposal(t *testing.T) {
	repo := setupEvolutionRepo(t)
	ctx := context.Background()
	raw := repo.data.Postgres()

	// 标准矛盾提案：payload JSONB 往返。
	if err := repo.InsertProposal(ctx, bizknowledge.GovernanceProposal{
		CollectionID: "c1", Kind: bizknowledge.ProposalKindConflict, Risk: bizknowledge.ProposalRiskHigh,
		Payload: map[string]any{"doc_id": "d1", "target_fact_id": "fid-old", "confidence": 0.85},
	}); err != nil {
		t.Fatalf("InsertProposal: %v", err)
	}
	// risk 空 → 默认 high（M3.2 矛盾提案从严）。
	if err := repo.InsertProposal(ctx, bizknowledge.GovernanceProposal{
		CollectionID: "c1", Kind: bizknowledge.ProposalKindConflict,
	}); err != nil {
		t.Fatalf("InsertProposal default risk: %v", err)
	}
	// 空 kind / 空 collection → 跳过。
	if err := repo.InsertProposal(ctx, bizknowledge.GovernanceProposal{CollectionID: "c1"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertProposal(ctx, bizknowledge.GovernanceProposal{Kind: "conflict"}); err != nil {
		t.Fatal(err)
	}

	var n int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_governance_proposal WHERE collection_id='c1'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n != 2 {
		t.Fatalf("proposals = %d, want 2 (empty kind/collection skipped)", n)
	}
	var kind, risk, status string
	var payloadRaw []byte
	if err := raw.QueryRowContext(ctx,
		`SELECT kind, risk, status, payload FROM knowledge_governance_proposal ORDER BY id LIMIT 1`).
		Scan(&kind, &risk, &status, &payloadRaw); err != nil {
		t.Fatal(err)
	}
	if kind != "conflict" || risk != "high" || status != "pending" {
		t.Fatalf("proposal = %s/%s/%s, want conflict/high/pending", kind, risk, status)
	}
	var payload map[string]any
	if err := json.Unmarshal(payloadRaw, &payload); err != nil {
		t.Fatalf("payload not JSONB roundtrip: %v", err)
	}
	if payload["target_fact_id"] != "fid-old" || payload["doc_id"] != "d1" {
		t.Fatalf("payload = %+v", payload)
	}
	// 第二条 risk 默认 high。
	var cntHigh int
	if err := raw.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM knowledge_governance_proposal WHERE risk='high'`).Scan(&cntHigh); err != nil {
		t.Fatal(err)
	}
	if cntHigh != 2 {
		t.Fatalf("default risk=high rows = %d, want 2", cntHigh)
	}
}
