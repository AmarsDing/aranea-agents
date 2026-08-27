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

// TestMemoryFactPendingRepo_CountPendingByAge pins the P5.2 stale bucketing
// contract against real PG: staleFail = age > failAgeSec (strict), staleWarn =
// warnAgeSec < age <= failAgeSec — boundary rows at exactly 24h/72h must land
// outside/inside respectively, decided rows must not be counted at all.
func TestMemoryFactPendingRepo_CountPendingByAge(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryFactPendingTestRepo(t)
	counter, ok := repo.(biz.MemoryFactPendingCounter)
	if !ok {
		t.Fatal("repo must expose MemoryFactPendingCounter narrow capability")
	}

	const (
		now      = int64(1_800_000_000)
		warnAge  = int64(86_400)  // 24h
		failAge  = int64(259_200) // 72h
	)
	rows := []biz.MemoryFactPendingRecord{
		{ID: "mfp-c-fresh", AgentID: "a", FactKey: "k1", Verdict: "ADD", Status: "pending", CreatedAt: now - 3_600},       // age 1h: total only
		{ID: "mfp-c-warnedge", AgentID: "a", FactKey: "k2", Verdict: "ADD", Status: "pending", CreatedAt: now - warnAge}, // age == 24h: NOT warn (strict >)
		{ID: "mfp-c-warn", AgentID: "a", FactKey: "k3", Verdict: "ADD", Status: "pending", CreatedAt: now - 90_000},      // age 25h: warn
		{ID: "mfp-c-failedge", AgentID: "a", FactKey: "k4", Verdict: "ADD", Status: "pending", CreatedAt: now - failAge}, // age == 72h: warn (age <= fail)
		{ID: "mfp-c-fail", AgentID: "a", FactKey: "k5", Verdict: "ADD", Status: "pending", CreatedAt: now - failAge - 1}, // age 72h+1s: fail
		{ID: "mfp-c-decided", AgentID: "a", FactKey: "k6", Verdict: "ADD", Status: "approved", CreatedAt: now - 300_000}, // decided: excluded
	}
	for _, rec := range rows {
		if err := repo.InsertPending(ctx, rec); err != nil {
			t.Fatalf("insert %s: %v", rec.ID, err)
		}
	}

	total, staleWarn, staleFail, err := counter.CountPendingByAge(ctx, warnAge, failAge, now)
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if total != 5 || staleWarn != 2 || staleFail != 1 {
		t.Fatalf("want total=5 warn=2 fail=1, got total=%d warn=%d fail=%d", total, staleWarn, staleFail)
	}

	// now 前移 10^7s 后所有行变为"未来行"（age<0）：不得计入 stale 分档，
	// total 仍为全量 pending COUNT（与 now 无关）。
	total, staleWarn, staleFail, err = counter.CountPendingByAge(ctx, warnAge, failAge, now-10_000_000)
	if err != nil {
		t.Fatalf("count shifted-now: %v", err)
	}
	if total != 5 || staleWarn != 0 || staleFail != 0 {
		t.Fatalf("future rows: want total=5 warn=0 fail=0, got total=%d warn=%d fail=%d", total, staleWarn, staleFail)
	}
}
