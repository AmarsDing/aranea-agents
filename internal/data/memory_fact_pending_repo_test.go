package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// newMemoryFactPendingTestRepo builds the pending repo over an isolated PG
// schema with the real 20261249 + 20261255 (payload_json) migration DDL applied.
func newMemoryFactPendingTestRepo(t *testing.T) biz.MemoryFactPendingStore {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261249_memory_fact_pending.sql", DialectPostgres, lg); err != nil {
		t.Fatalf("migrate 20261249: %v", err)
	}
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261255_memory_fact_pending_payload.sql", DialectPostgres, lg); err != nil {
		t.Fatalf("migrate 20261255: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: lg, dialect: DialectPostgres}
	repo := NewMemoryFactPendingRepo(d, lg)
	if repo == nil {
		t.Fatal("repo constructor returned nil over live DB")
	}
	return repo
}

// TestMemoryFactPendingRepo_Roundtrip covers insert → get → list → decide,
// including the fail-closed double-decision guard and insert idempotency.
func TestMemoryFactPendingRepo_Roundtrip(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryFactPendingTestRepo(t)

	rec := biz.MemoryFactPendingRecord{
		ID: "mfp-rt-1", AgentID: "agent-1", FactKey: "fact-old",
		Verdict: biz.MemoryFactPendingVerdictUpdate,
		ProposedBody: "用户偏好编辑器为 Neovim", PriorBody: "用户偏好编辑器为 VS Code",
		AdjudicatorReason: "adjudicated_update",
		PayloadJSON:       `{"candidate":{"Statement":"用户偏好编辑器为 Neovim","FactKind":"preference"},"target_fact_id":"fact-old"}`,
		Status:            biz.MemoryFactPendingStatusPending,
		CreatedAt:         1787702400,
	}
	if err := repo.InsertPending(ctx, rec); err != nil {
		t.Fatalf("insert: %v", err)
	}
	// Idempotent on id — replay must not duplicate or error.
	if err := repo.InsertPending(ctx, rec); err != nil {
		t.Fatalf("idempotent re-insert: %v", err)
	}

	got, found, err := repo.GetPending(ctx, "mfp-rt-1")
	if err != nil || !found {
		t.Fatalf("get: found=%v err=%v", found, err)
	}
	if got.Verdict != "UPDATE" || got.PriorBody == "" || got.Status != "pending" || got.CreatedAt != 1787702400 {
		t.Fatalf("get mismatch: %+v", got)
	}
	if got.PayloadJSON != rec.PayloadJSON {
		t.Fatalf("payload_json roundtrip mismatch: got %q", got.PayloadJSON)
	}
	if _, found, err := repo.GetPending(ctx, "mfp-absent"); err != nil || found {
		t.Fatalf("absent get: found=%v err=%v", found, err)
	}

	pend, err := repo.ListPending(ctx, "agent-1", biz.MemoryFactPendingStatusPending, 10)
	if err != nil || len(pend) != 1 {
		t.Fatalf("list pending: n=%d err=%v", len(pend), err)
	}
	other, err := repo.ListPending(ctx, "agent-other", "", 10)
	if err != nil || len(other) != 0 {
		t.Fatalf("list other agent: n=%d err=%v", len(other), err)
	}

	// Decide: approve. Second decide must be a fail-closed no-op.
	applied, err := repo.MarkDecided(ctx, "mfp-rt-1", biz.MemoryFactPendingStatusApproved, "user-admin", 1787702460)
	if err != nil || !applied {
		t.Fatalf("decide: applied=%v err=%v", applied, err)
	}
	applied, err = repo.MarkDecided(ctx, "mfp-rt-1", biz.MemoryFactPendingStatusRejected, "user-admin2", 1787702500)
	if err != nil || applied {
		t.Fatalf("double decide must be no-op: applied=%v err=%v", applied, err)
	}
	got, found, err = repo.GetPending(ctx, "mfp-rt-1")
	if err != nil || !found {
		t.Fatalf("get decided: found=%v err=%v", found, err)
	}
	if got.Status != "approved" || got.Approver != "user-admin" || got.DecidedAt != 1787702460 {
		t.Fatalf("decided row mismatch: %+v", got)
	}
	pend, err = repo.ListPending(ctx, "", biz.MemoryFactPendingStatusPending, 10)
	if err != nil || len(pend) != 0 {
		t.Fatalf("decided row must leave pending list: n=%d err=%v", len(pend), err)
	}
}
