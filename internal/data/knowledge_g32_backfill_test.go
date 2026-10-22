package data

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestBackfillVersionBodyHashDriftGuard DIS-3 漂移护航确定性钉：用行锁构造
// SELECT→UPDATE 窗口——加锁方在 runner 的 UPDATE 阻塞期间把正文换代再提交，
// READ COMMITTED 语义下谓词重估命中 d.content_text=$4 失配 → 0 行落盘，
// 杜绝把从未对应任何提交态的 hash 钉进治理账本。二次续跑按新正文重算收复。
func TestBackfillVersionBodyHashDriftGuard(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("exec %.120q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	mustExec(`INSERT INTO knowledge_documents (id, collection_id, source, content_text) VALUES ('d1', 'c1', 's1', 'body v1')`)
	mustExec(`INSERT INTO document_versions (doc_id, version_no, change_kind, producer) VALUES ('d1', 1, 'seed', 'g32-seed')`)

	// 加锁方：锁住【documents 行】（no-op UPDATE 拿行锁），把 runner 卡在
	// FOR SHARE 核身步——锁目标与核身语句同表同行，逃不出「A 持锁等待、
	// B 持 KeyShare 再升级」的双锁环（单锁链确定性成立）。
	conn1, err := db.Conn(ctx)
	if err != nil {
		t.Fatalf("conn1: %v", err)
	}
	defer func() { _ = conn1.Close() }()
	tx1, err := conn1.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("tx1: %v", err)
	}
	defer func() { _ = tx1.Rollback() }() // 任何失败路径都解释行锁（否则 runner 永久挂死）。
	if _, err := tx1.ExecContext(ctx, `UPDATE knowledge_documents SET chunk_count = chunk_count WHERE id = 'd1'`); err != nil {
		t.Fatalf("lock documents row: %v", err)
	}

	type driftResult struct {
		n   int
		err error
	}
	done := make(chan driftResult, 1)
	go func() {
		n, err := backfillVersionBodyHashes(ctx, db, 10)
		done <- driftResult{n: n, err: err}
	}()
	// 确定性屏障：轮询 pg_stat_activity 等 runner 的 FOR SHARE 核身进入 Lock
	// 等待（行锁阻塞表现为 wait_event_type='Lock'；勿用 pg_locks.locktype
	// 过滤——行冲突在 PG 里呈现 transactionid 等待而非 tuple 等待；LIKE 必须
	// 前导通配——SQL 原文以换行起头）。带节拍与预算：goroutine 侧新连接 TCP
	// 握手在容器回环上可达数百 ms，纯自旋会先于 runner 到达阻塞点就耗尽预算。
	blocked := false
	for i := 0; i < 400; i++ {
		var waiting int
		if err := db.QueryRowContext(ctx, `
			SELECT count(*) FROM pg_stat_activity
			WHERE datname = current_database() AND wait_event_type = 'Lock'
			  AND query LIKE '%FOR SHARE%'`).Scan(&waiting); err != nil {
			t.Fatalf("poll activity: %v", err)
		}
		if waiting > 0 {
			blocked = true
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !blocked {
		t.Fatalf("runner never blocked on the dv row lock — fixture broken")
	}
	// 窗口内正文换代，提交放行。
	if _, err := tx1.ExecContext(ctx, `UPDATE knowledge_documents SET content_text = 'body v2' WHERE id = 'd1'`); err != nil {
		t.Fatalf("drift body: %v", err)
	}
	if err := tx1.Commit(); err != nil {
		t.Fatalf("commit drift: %v", err)
	}
	first := <-done
	if first.err != nil {
		t.Fatalf("runner: %v", first.err)
	}
	// 护航钉：陈旧 body 的 hash 零落盘（谓词重估失配）。
	var hash string
	if err := db.QueryRowContext(ctx, `SELECT content_hash FROM document_versions WHERE doc_id = 'd1'`).Scan(&hash); err != nil {
		t.Fatalf("read hash: %v", err)
	}
	if first.n != 0 || hash != "" {
		t.Fatalf("stale body hash written: applied=%d hash=%q", first.n, hash)
	}
	// 续跑钉：下轮按新正文重算收复。
	n, err := backfillVersionBodyHashes(ctx, db, 10)
	if err != nil {
		t.Fatalf("runner rerun: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT content_hash FROM document_versions WHERE doc_id = 'd1'`).Scan(&hash); err != nil {
		t.Fatalf("reread hash: %v", err)
	}
	if n != 1 || hash != chunkContentKey("body v2") {
		t.Fatalf("recovery mismatch: applied=%d hash=%q want sha1 of body v2", n, hash)
	}
}

// TestBackfillKnowledgeG32 G-32 T1 回填 runner 钉：content_key 空串 行按
// 'sha1:'+hex 口径回填；文本 doc 版本行补 body hash、资产 doc 保持 空串 未知档；
// 二次运行零增量（幂等断点续跑）。
func TestBackfillKnowledgeG32(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureKnowledgeSchema(ctx, db, 3); err != nil {
		t.Fatalf("ensure knowledge schema: %v", err)
	}
	mustExec := func(q string) {
		t.Helper()
		if _, err := db.ExecContext(ctx, q); err != nil {
			t.Fatalf("exec %.120q: %v", q, err)
		}
	}
	mustExec(`INSERT INTO knowledge_collections (id, name, embedding_model) VALUES ('c1', 'c1', 'm0')`)
	// d1 文本 doc（asset_uri=空串），d2 资产 doc（图片口径，content_hash=资产字节）。
	mustExec(`INSERT INTO knowledge_documents (id, collection_id, source, content_text) VALUES ('d1', 'c1', 's1', 'hello body')`)
	mustExec(`INSERT INTO knowledge_documents (id, collection_id, source, content_text, asset_uri, content_hash)
		VALUES ('d2', 'c1', 's2', 'llm extract', 'assets/d2.png', 'sha1:asset-d2')`)
	mustExec(`INSERT INTO knowledge_chunks (id, doc_id, collection_id, content, chunk_index) VALUES
		('d1-ch-0', 'd1', 'c1', 'alpha', 0),
		('d1-ch-1', 'd1', 'c1', 'alpha', 1),
		('d2-ch-0', 'd2', 'c1', 'beta', 0)`)
	mustExec(`INSERT INTO document_versions (doc_id, version_no, change_kind, producer) VALUES
		('d1', 1, 'seed', 'g32-seed'),
		('d2', 1, 'seed', 'g32-seed')`)

	if err := BackfillKnowledgeG32(ctx, db, 2, loggateway.NewNoop()); err != nil {
		t.Fatalf("backfill run 1: %v", err)
	}
	// content_key 口径钉（含批大小=2 的翻页：3 行须两轮）。
	var got string
	if err := db.QueryRowContext(ctx, `SELECT content_key FROM knowledge_chunks WHERE id = 'd1-ch-0'`).Scan(&got); err != nil {
		t.Fatalf("read backfilled key: %v", err)
	}
	if want := chunkContentKey("alpha"); got != want {
		t.Fatalf("content_key caliber mismatch: got %q want %q", got, want)
	}
	var unfilled int
	if err := db.QueryRowContext(ctx, `SELECT count(*) FROM knowledge_chunks WHERE content_key = ''`).Scan(&unfilled); err != nil {
		t.Fatalf("count unfilled: %v", err)
	}
	if unfilled != 0 {
		t.Fatalf("content_key backfill incomplete: %d rows remain ''", unfilled)
	}
	// 版本行：文本 doc 补 body hash，资产 doc 保持 空串 未知档。
	var d1Hash, d2Hash string
	if err := db.QueryRowContext(ctx, `SELECT content_hash FROM document_versions WHERE doc_id = 'd1'`).Scan(&d1Hash); err != nil {
		t.Fatalf("read d1 version hash: %v", err)
	}
	if err := db.QueryRowContext(ctx, `SELECT content_hash FROM document_versions WHERE doc_id = 'd2'`).Scan(&d2Hash); err != nil {
		t.Fatalf("read d2 version hash: %v", err)
	}
	if want := chunkContentKey("hello body"); d1Hash != want {
		t.Fatalf("text-doc body hash mismatch: got %q want %q", d1Hash, want)
	}
	if d2Hash != "" {
		t.Fatalf("asset-doc version hash must stay '' (unknown caliber), got %q", d2Hash)
	}
	// 幂等：健康行非 空串 绝不被重写（空串 护航谓词钉——埋一行人为异常值，
	// 二次运行后必须原样保留且其余行零漂移）。
	mustExec(`UPDATE knowledge_chunks SET content_key = 'corrupt-row' WHERE id = 'd2-ch-0'`)
	if err := BackfillKnowledgeG32(ctx, db, 2, loggateway.NewNoop()); err != nil {
		t.Fatalf("backfill run 2: %v", err)
	}
	var after string
	if err := db.QueryRowContext(ctx, `SELECT content_key FROM knowledge_chunks WHERE id = 'd2-ch-0'`).Scan(&after); err != nil {
		t.Fatalf("read guarded row: %v", err)
	}
	if after != "corrupt-row" {
		t.Fatalf("non-'' row rewritten by rerun: got %q", after)
	}
	if err := db.QueryRowContext(ctx, `SELECT content_key FROM knowledge_chunks WHERE id = 'd1-ch-0'`).Scan(&after); err != nil {
		t.Fatalf("reread d1-ch-0: %v", err)
	}
	if want := chunkContentKey("alpha"); after != want {
		t.Fatalf("healthy key drifted after rerun: got %q want %q", after, want)
	}
}
