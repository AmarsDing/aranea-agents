package data

import (
	"context"
	"testing"

	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// TestDecisionRecordsMigration_PG smoke-runs the real M80 Phase 1 migration
// files (20261250/51/52) against an isolated Postgres schema, including the
// PG-gated GIN Func, twice to prove idempotency, then inserts a row to verify
// the column contract used by the decision repo.
func TestDecisionRecordsMigration_PG(t *testing.T) {
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()

	files := []string{
		"sql/migrations/20261250_decision_records.sql",
		"sql/migrations/20261251_decision_record_outbox.sql",
		"sql/migrations/20261252_decision_records_idx.sql",
	}
	for round := 0; round < 2; round++ {
		for _, f := range files {
			if err := executeSQLFileWithDialect(ctx, db, f, DialectPostgres, lg); err != nil {
				t.Fatalf("round %d %s: %v", round, f, err)
			}
		}
		if err := ddlDecisionRecordsGINIndexes(ctx, db, nil, DialectPostgres, lg); err != nil {
			t.Fatalf("round %d gin indexes: %v", round, err)
		}
	}

	// GIN Func must skip cleanly on non-Postgres dialects.
	if err := ddlDecisionRecordsGINIndexes(ctx, db, nil, DialectSQLite, lg); err != nil {
		t.Fatalf("gin skip on sqlite dialect: %v", err)
	}

	var n int
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_indexes WHERE tablename='decision_records'`,
	).Scan(&n); err != nil {
		t.Fatalf("count decision_records indexes: %v", err)
	}
	// uq_decision_records_key + 4 regular + 2 GIN expression indexes.
	if n < 7 {
		t.Fatalf("decision_records indexes = %d, want >= 7", n)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM pg_indexes WHERE tablename='decision_record_outbox'`,
	).Scan(&n); err != nil {
		t.Fatalf("count outbox indexes: %v", err)
	}
	if n < 2 {
		t.Fatalf("decision_record_outbox indexes = %d, want >= 2", n)
	}

	// Column contract: insert a full record the way the outbox worker will.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO decision_records
		 (decision_key, category, scenario, reasoning, outcome, confidence,
		  actor_type, actor_key, parent_decision_id, related_entities, source_ref,
		  metadata, workspace_id, created_at, updated_at)
		 VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		"dk-test-1", "hitl_approval", "高危工具 gns3_fault_inject 待审批", "审批人备注",
		"approved", nil, "human", "user-1", nil,
		`[{"type":"tool","key":"gns3_fault_inject"}]`,
		`{"tool_invocation_id":"tc-1"}`,
		`{"decision_reason":"policy_danger"}`, "ws-1",
		"2026-08-26T00:00:00Z", "2026-08-26T00:00:00Z",
	); err != nil {
		t.Fatalf("insert decision_records row: %v", err)
	}
	var gotKey, gotCategory string
	if err := db.QueryRowContext(ctx,
		`SELECT decision_key, category FROM decision_records WHERE decision_key=$1`, "dk-test-1",
	).Scan(&gotKey, &gotCategory); err != nil {
		t.Fatalf("select back: %v", err)
	}
	if gotKey != "dk-test-1" || gotCategory != "hitl_approval" {
		t.Fatalf("roundtrip mismatch: %s %s", gotKey, gotCategory)
	}

	// outbox payload contract + idempotent decision_key.
	if _, err := db.ExecContext(ctx,
		`INSERT INTO decision_record_outbox (decision_key, payload, created_at)
		 VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		"dk-test-1", `{"decision_key":"dk-test-1"}`, "2026-08-26T00:00:00Z",
	); err != nil {
		t.Fatalf("insert outbox row: %v", err)
	}
	if _, err := db.ExecContext(ctx,
		`INSERT INTO decision_record_outbox (decision_key, payload, created_at)
		 VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`,
		"dk-test-1", `{"decision_key":"dk-test-1"}`, "2026-08-26T00:00:01Z",
	); err != nil {
		t.Fatalf("insert duplicate outbox row: %v", err)
	}
	if err := db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM decision_record_outbox WHERE decision_key=$1`, "dk-test-1",
	).Scan(&n); err != nil {
		t.Fatalf("count outbox rows: %v", err)
	}
	if n != 1 {
		t.Fatalf("outbox idempotent insert rows = %d, want 1", n)
	}
}
