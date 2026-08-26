package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestMemoryFactPendingMigration_PG smoke-runs the 20261249 migration file
// against an isolated Postgres schema twice to prove idempotency, then
// inserts a row to verify the column contract used by the pending store.
func TestMemoryFactPendingMigration_PG(t *testing.T) {
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()

	const file = "sql/migrations/20261249_memory_fact_pending.sql"
	for round := 0; round < 2; round++ {
		if err := executeSQLFileWithDialect(ctx, db, file, DialectPostgres, lg); err != nil {
			t.Fatalf("round %d %s: %v", round, file, err)
		}
	}

	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_indexes WHERE tablename='memory_fact_pending'`,
	).Scan(&n); err != nil {
		t.Fatalf("count memory_fact_pending indexes: %v", err)
	}
	// idx_mfp_status + idx_mfp_agent_status（PK 索引不计入 pg_indexes 断言下限 2）。
	if n < 2 {
		t.Fatalf("memory_fact_pending indexes = %d, want >= 2", n)
	}

	// Column contract: insert a contested verdict row the way the write gate
	// will, then decide it (approved + approver + decided_at).
	if _, err := db.ExecContext(ctx,
		`INSERT INTO memory_fact_pending
		 (id, agent_id, fact_key, verdict, proposed_body, prior_body,
		  adjudicator_reason, status, approver, created_at, decided_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
		"mfp-test-1", "agent-1", "preference.editor", "UPDATE",
		"用户偏好编辑器为 Neovim", "用户偏好编辑器为 VS Code",
		"与既有事实语义冲突，需人工裁决", "pending", "", 1787702400, 0,
	); err != nil {
		t.Fatalf("insert memory_fact_pending row: %v", err)
	}
	var gotStatus, gotPrior string
	if err := db.QueryRowContext(ctx,
		`SELECT status, prior_body FROM memory_fact_pending WHERE id=$1`, "mfp-test-1",
	).Scan(&gotStatus, &gotPrior); err != nil {
		t.Fatalf("select back: %v", err)
	}
	if gotStatus != "pending" || gotPrior == "" {
		t.Fatalf("roundtrip mismatch: status=%s prior=%q", gotStatus, gotPrior)
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE memory_fact_pending SET status=$1, approver=$2, decided_at=$3 WHERE id=$4`,
		"approved", "user-1", 1787702460, "mfp-test-1",
	); err != nil {
		t.Fatalf("decide row: %v", err)
	}
	var gotApprover string
	var decidedAt int64
	if err := db.QueryRowContext(ctx,
		`SELECT status, approver, decided_at FROM memory_fact_pending WHERE id=$1`, "mfp-test-1",
	).Scan(&gotStatus, &gotApprover, &decidedAt); err != nil {
		t.Fatalf("select decided: %v", err)
	}
	if gotStatus != "approved" || gotApprover != "user-1" || decidedAt == 0 {
		t.Fatalf("decide mismatch: status=%s approver=%s decided_at=%d", gotStatus, gotApprover, decidedAt)
	}

	// 20261255 payload_json（R3 3.3）：重放快照列契约——幂等重跑 + 读写回合。
	const file2 = "sql/migrations/20261255_memory_fact_pending_payload.sql"
	for round := 0; round < 2; round++ {
		if err := executeSQLFileWithDialect(ctx, db, file2, DialectPostgres, lg); err != nil {
			t.Fatalf("round %d %s: %v", round, file2, err)
		}
	}
	if _, err := db.ExecContext(ctx,
		`UPDATE memory_fact_pending SET payload_json=$1 WHERE id=$2`,
		`{"candidate":{"statement":"SRV-DB-03 已迁移"},"target_fact_id":"fact-1"}`, "mfp-test-1",
	); err != nil {
		t.Fatalf("update payload_json: %v", err)
	}
	var gotPayload string
	if err := db.QueryRowContext(ctx,
		`SELECT payload_json FROM memory_fact_pending WHERE id=$1`, "mfp-test-1",
	).Scan(&gotPayload); err != nil {
		t.Fatalf("select payload_json: %v", err)
	}
	if gotPayload == "" {
		t.Fatal("payload_json roundtrip empty")
	}
}
