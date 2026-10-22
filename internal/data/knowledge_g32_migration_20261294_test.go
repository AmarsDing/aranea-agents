package data

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/data/testhelper"
)

// TestG32Migration20261294_RepairSeedIdempotent G-32 T1 冒烟（ADR-KN-20261022-I
// 测试矩阵① PG 段）：Ensure 建全量 fresh → 拆 G-32 对象回到存量形态 → 应用迁移
// 文件修复 → 种子行口径 + 化身区间视图钉 → 二次应用幂等零错。
func TestG32Migration20261294_RepairSeedIdempotent(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 1536); err != nil {
		t.Fatalf("ensure base knowledge schema: %v", err)
	}
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("exec %.120q: %v", q, err)
		}
	}
	// 预置存量文档/块（种子与视图钉的对象）。
	mustExec(`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExec(`INSERT INTO knowledge_documents (id, collection_id, source, content_hash, chunk_count) VALUES ('d1', 'c1', 's1', 'sha1:asset-d1', 2)`)
	mustExec(`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES ('d1-ch-0', 'd1', 'c1', 'alpha', 0)`)
	// 回到存量形态：拆掉 G-32 全部对象（迁移修复路径）。
	for _, s := range []string{
		`DROP VIEW IF EXISTS knowledge_chunk_incarnations`,
		`DROP TABLE IF EXISTS chunk_embeddings_archive`,
		`DROP TABLE IF EXISTS chunk_archive`,
		`DROP TABLE IF EXISTS document_versions`,
		`ALTER TABLE knowledge_chunks DROP COLUMN IF EXISTS content_key`,
	} {
		mustExec(s)
	}
	raw, err := os.ReadFile("sql/migrations/20261294_document_versions_chunk_archive.sql")
	if err != nil {
		t.Fatalf("read migration file: %v", err)
	}
	apply := func() {
		t.Helper()
		if _, err := db.ExecContext(ctx, string(raw)); err != nil {
			t.Fatalf("apply migration 20261294: %v", err)
		}
	}
	apply()

	// 对象修复钉。
	for _, name := range []string{
		"document_versions", "chunk_archive", "chunk_embeddings_archive", "knowledge_chunk_incarnations",
	} {
		var exists bool
		if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&exists); err != nil || !exists {
			t.Fatalf("regclass %s missing after migration: exists=%v err=%v", name, exists, err)
		}
	}
	var hasContentKey bool
	if err := db.QueryRowContext(ctx, `SELECT EXISTS (
		SELECT 1 FROM information_schema.columns
		WHERE table_name = 'knowledge_chunks' AND column_name = 'content_key')`).Scan(&hasContentKey); err != nil || !hasContentKey {
		t.Fatalf("knowledge_chunks.content_key missing after migration: exists=%v err=%v", hasContentKey, err)
	}

	// 种子行口径钉：version_no=1 / content_hash=空串 未知档 / asset_hash=documents 列值 /
	// change_kind='seed' / producer / chunk_stats / created_at=doc.created_at。
	var contentHash, assetHash, kind, producer, stats string
	var seedCreatedAt time.Time
	err = db.QueryRowContext(ctx, `SELECT content_hash, asset_hash, change_kind, producer,
		chunk_stats::text, created_at
		FROM document_versions WHERE doc_id = 'd1' AND version_no = 1`).
		Scan(&contentHash, &assetHash, &kind, &producer, &stats, &seedCreatedAt)
	if err != nil {
		t.Fatalf("seed row missing: %v", err)
	}
	if contentHash != "" || assetHash != "sha1:asset-d1" || kind != "seed" || producer != "g32-seed" {
		t.Fatalf("seed row caliber mismatch: hash=%q asset=%q kind=%q producer=%q", contentHash, assetHash, kind, producer)
	}
	if !strings.Contains(stats, `"chunk_count": 2`) {
		t.Fatalf("seed chunk_stats mismatch: %s", stats)
	}

	// 视图 current 腿钉：legacy 位置 ID 归 seed 版本档（valid_from=seed created_at），
	// valid_to='infinity'，archived=false。
	var currentOK bool
	if err := db.QueryRowContext(ctx, `SELECT valid_to = 'infinity'::timestamptz
		AND NOT archived AND valid_from = $1
		FROM knowledge_chunk_incarnations WHERE chunk_id = 'd1-ch-0'`, seedCreatedAt).Scan(&currentOK); err != nil || !currentOK {
		t.Fatalf("incarnation view current-leg pin failed: ok=%v err=%v", currentOK, err)
	}

	// archive 腿钉：v2 版本行 created_at 定为死亡时刻——归档 revision_no=2 的块
	// valid_to=v2.created_at、valid_from=seed 档、archived=true。
	mustExec(`INSERT INTO document_versions (doc_id, version_no, content_hash, change_kind, producer, created_at)
		VALUES ('d1', 2, 'sha1:body2', 'edit', 'g32-test', now() + interval '1 second')`)
	mustExec(`INSERT INTO chunk_archive (doc_id, chunk_id, revision_no, collection_id, content, content_key)
		VALUES ('d1', 'd1-ch-9', 2, 'c1', 'ghost', 'sha1:ghost')`)
	var archiveOK bool
	if err := db.QueryRowContext(ctx, `SELECT archived AND valid_from = $1
		AND valid_to = (SELECT created_at FROM document_versions WHERE doc_id = 'd1' AND version_no = 2)
		FROM knowledge_chunk_incarnations WHERE chunk_id = 'd1-ch-9' AND archived`, seedCreatedAt).Scan(&archiveOK); err != nil || !archiveOK {
		t.Fatalf("incarnation view archive-leg pin failed: ok=%v err=%v", archiveOK, err)
	}

	// 幂等：二次应用零错，种子不翻倍（v1 冲突 DO NOTHING，统计含手工 v2=2 行）。
	apply()
	var versionRows int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM document_versions WHERE doc_id = 'd1'`).Scan(&versionRows); err != nil {
		t.Fatalf("count versions: %v", err)
	}
	if versionRows != 2 {
		t.Fatalf("idempotent re-apply duplicated seed: versions=%d (want 2)", versionRows)
	}
}
