package data

import (
	"context"
	"database/sql"
	"testing"
	"time"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

func newWriteBackOutboxTestRepo(t *testing.T) (bizknowledge.WriteBackOutboxRepo, *sql.DB) {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	// DDL mirrors sql/migrations/20261280_knowledge_writeback_outbox.sql（PG 方言形态——
	// 迁移 runner 会把 AUTOINCREMENT 翻译为 BIGSERIAL，此处直接以 PG 等价建表）。
	for _, s := range []string{
		`CREATE TABLE IF NOT EXISTS knowledge_writeback_outbox (
			id BIGSERIAL PRIMARY KEY,
			batch_key TEXT NOT NULL,
			workspace_id TEXT NOT NULL DEFAULT '',
			payload TEXT NOT NULL,
			status TEXT NOT NULL DEFAULT 'pending',
			attempts INTEGER NOT NULL DEFAULT 0,
			last_error TEXT NOT NULL DEFAULT '',
			created_at TEXT NOT NULL DEFAULT '',
			published_at TEXT
		)`,
		`CREATE UNIQUE INDEX IF NOT EXISTS uq_knowledge_writeback_outbox_key
			ON knowledge_writeback_outbox(batch_key)`,
		`CREATE INDEX IF NOT EXISTS idx_knowledge_writeback_outbox_status_created
			ON knowledge_writeback_outbox(status, created_at)`,
	} {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("writeback outbox schema: %v", err)
		}
	}
	d := &Data{
		rawDB:   db,
		readDB:  db,
		rwDB:    NewReadWriteDB(db, db),
		lg:      loggateway.NewNoop(),
		dialect: DialectPostgres,
	}
	return NewKnowledgeWriteBackOutboxRepo(d, loggateway.NewNoop()), db
}

func writeBackOutboxTestInput(sessionID string) bizknowledge.WriteBackInput {
	return bizknowledge.WriteBackInput{
		Workspace: "default",
		SessionID: sessionID,
		AgentID:   "agent-1",
		UserID:    "user-1",
		Facts: []bizknowledge.WriteBackFact{{
			FactID:     "f-1",
			Statement:  "用户偏好深色模式界面",
			FactKind:   "preference",
			Confidence: 0.95,
			SourceKind: "dialogue",
			Tags:       []string{"偏好", "界面"},
		}},
	}
}

func TestKnowledgeWriteBackOutbox_EnqueueListRoundtrip(t *testing.T) {
	ctx := context.Background()
	repo, _ := newWriteBackOutboxTestRepo(t)

	in := writeBackOutboxTestInput("sess-wb-1")
	key := bizknowledge.WriteBackBatchKey(in.SessionID, in.Facts)
	if err := repo.EnqueueWriteBackOutbox(ctx, key, in); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	// 同 batch_key 重复入队被幂等忽略（同批事实反复失败不堆行）。
	if err := repo.EnqueueWriteBackOutbox(ctx, key, in); err != nil {
		t.Fatalf("Enqueue dup: %v", err)
	}

	rows, err := repo.ListPendingWriteBackOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("ListPending rows = %d, want 1 (dup key ignored)", len(rows))
	}
	row := rows[0]
	if row.BatchKey != key || row.WorkspaceID != "default" {
		t.Fatalf("row key/workspace = %q/%q", row.BatchKey, row.WorkspaceID)
	}
	if row.Status != bizknowledge.WriteBackOutboxStatusPending || row.Attempts != 0 {
		t.Fatalf("row status/attempts = %q/%d", row.Status, row.Attempts)
	}
	// Payload 可解码且忠实还原（EntryTitle 不入队）。
	got, err := bizknowledge.DecodeWriteBackOutboxPayload([]byte(row.Payload))
	if err != nil {
		t.Fatalf("Decode payload: %v", err)
	}
	if got.SessionID != in.SessionID || len(got.Facts) != 1 || got.Facts[0].Statement != in.Facts[0].Statement {
		t.Fatalf("decoded input = %+v", got)
	}
	if len(got.Facts[0].Tags) != 2 {
		t.Fatalf("decoded tags = %+v", got.Facts[0].Tags)
	}
}

func TestKnowledgeWriteBackOutbox_AttemptFlipsDeadAtCap(t *testing.T) {
	ctx := context.Background()
	repo, db := newWriteBackOutboxTestRepo(t)

	in := writeBackOutboxTestInput("sess-wb-dead")
	if err := repo.EnqueueWriteBackOutbox(ctx, bizknowledge.WriteBackBatchKey(in.SessionID, in.Facts), in); err != nil {
		t.Fatalf("Enqueue: %v", err)
	}
	rows, err := repo.ListPendingWriteBackOutbox(ctx, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("ListPending: %v rows=%d", err, len(rows))
	}
	id := rows[0].ID
	for i := 0; i < bizknowledge.WriteBackOutboxMaxAttempts; i++ {
		if err := repo.MarkWriteBackOutboxAttempt(ctx, id, "kb down"); err != nil {
			t.Fatalf("MarkAttempt(%d): %v", i, err)
		}
	}
	// 触达上限 → status=dead 且退出 pending 扫描（防 head-of-line blocking）。
	rows, err = repo.ListPendingWriteBackOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending after cap: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("dead row still pending: %+v", rows)
	}
	var status string
	var attempts int
	var lastErr string
	if err := db.QueryRowContext(ctx,
		`SELECT status, attempts, last_error FROM knowledge_writeback_outbox WHERE id=$1`, id,
	).Scan(&status, &attempts, &lastErr); err != nil {
		t.Fatalf("select row: %v", err)
	}
	if status != bizknowledge.WriteBackOutboxStatusDead || attempts != bizknowledge.WriteBackOutboxMaxAttempts || lastErr == "" {
		t.Fatalf("row = status:%q attempts:%d err:%q", status, attempts, lastErr)
	}
}

func TestKnowledgeWriteBackOutbox_PublishedAndDeadExit(t *testing.T) {
	ctx := context.Background()
	repo, db := newWriteBackOutboxTestRepo(t)

	in1 := writeBackOutboxTestInput("sess-wb-pub")
	in2 := writeBackOutboxTestInput("sess-wb-poison")
	in2.Facts[0].Statement = "另一条事实，内容不同"
	for _, in := range []bizknowledge.WriteBackInput{in1, in2} {
		if err := repo.EnqueueWriteBackOutbox(ctx, bizknowledge.WriteBackBatchKey(in.SessionID, in.Facts), in); err != nil {
			t.Fatalf("Enqueue(%s): %v", in.SessionID, err)
		}
	}
	rows, err := repo.ListPendingWriteBackOutbox(ctx, 10)
	if err != nil || len(rows) != 2 {
		t.Fatalf("ListPending: %v rows=%d", err, len(rows))
	}
	if err := repo.MarkWriteBackOutboxPublished(ctx, []int64{rows[0].ID}, time.Now().UTC()); err != nil {
		t.Fatalf("MarkPublished: %v", err)
	}
	if err := repo.MarkWriteBackOutboxDead(ctx, []int64{rows[1].ID}, "poison: payload undecodable"); err != nil {
		t.Fatalf("MarkDead: %v", err)
	}
	rows, err = repo.ListPendingWriteBackOutbox(ctx, 10)
	if err != nil {
		t.Fatalf("ListPending: %v", err)
	}
	if len(rows) != 0 {
		t.Fatalf("terminal rows still pending: %+v", rows)
	}
	var pubAt sql.NullString
	if err := db.QueryRowContext(ctx,
		`SELECT published_at FROM knowledge_writeback_outbox WHERE status='published'`,
	).Scan(&pubAt); err != nil {
		t.Fatalf("select published: %v", err)
	}
	if !pubAt.Valid || pubAt.String == "" {
		t.Fatal("published_at not recorded")
	}
}
