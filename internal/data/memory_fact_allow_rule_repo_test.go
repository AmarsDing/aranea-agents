package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// newMemoryFactAllowRuleTestRepo builds the allow-rule repo over an isolated PG
// schema with the real 20261256 migration DDL applied.
func newMemoryFactAllowRuleTestRepo(t *testing.T) biz.MemoryFactAllowRuleStore {
	t.Helper()
	ctx := context.Background()
	db := testhelper.SetupTestPGRaw(t)
	lg := loggateway.NewNoop()
	if err := executeSQLFileWithDialect(ctx, db, "sql/migrations/20261256_memory_fact_allow_rules.sql", DialectPostgres, lg); err != nil {
		t.Fatalf("migrate 20261256: %v", err)
	}
	d := &Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: lg, dialect: DialectPostgres}
	repo := NewMemoryFactAllowRuleRepo(d, lg)
	if repo == nil {
		t.Fatal("repo constructor returned nil over live DB")
	}
	return repo
}

// TestMemoryFactAllowRuleRepo_Roundtrip covers grant → has → list → revoke,
// including grant idempotency on the (agent_id, verdict) unique index.
func TestMemoryFactAllowRuleRepo_Roundtrip(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryFactAllowRuleTestRepo(t)

	ok, err := repo.HasAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictUpdate)
	if err != nil || ok {
		t.Fatalf("absent has: ok=%v err=%v", ok, err)
	}

	if err := repo.GrantAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictUpdate, "admin"); err != nil {
		t.Fatalf("grant: %v", err)
	}
	// Idempotent on (agent_id, verdict) — re-grant must not duplicate or error.
	if err := repo.GrantAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictUpdate, "admin"); err != nil {
		t.Fatalf("idempotent re-grant: %v", err)
	}
	if err := repo.GrantAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictDelete, "admin"); err != nil {
		t.Fatalf("grant second verdict: %v", err)
	}

	ok, err = repo.HasAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictUpdate)
	if err != nil || !ok {
		t.Fatalf("has after grant: ok=%v err=%v", ok, err)
	}
	// Rules are verdict-scoped and agent-scoped.
	ok, err = repo.HasAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictContested)
	if err != nil || ok {
		t.Fatalf("verdict scope leak: ok=%v err=%v", ok, err)
	}
	ok, err = repo.HasAllowRule(ctx, "agent-2", biz.MemoryFactPendingVerdictUpdate)
	if err != nil || ok {
		t.Fatalf("agent scope leak: ok=%v err=%v", ok, err)
	}

	rules, err := repo.ListAllowRules(ctx, "agent-1", 10)
	if err != nil || len(rules) != 2 {
		t.Fatalf("list: n=%d err=%v", len(rules), err)
	}
	all, err := repo.ListAllowRules(ctx, "", 10)
	if err != nil || len(all) < 2 {
		t.Fatalf("list all: n=%d err=%v", len(all), err)
	}

	applied, err := repo.RevokeAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictUpdate)
	if err != nil || !applied {
		t.Fatalf("revoke: applied=%v err=%v", applied, err)
	}
	applied, err = repo.RevokeAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictUpdate)
	if err != nil || applied {
		t.Fatalf("double revoke must be applied=false: applied=%v err=%v", applied, err)
	}
	ok, err = repo.HasAllowRule(ctx, "agent-1", biz.MemoryFactPendingVerdictUpdate)
	if err != nil || ok {
		t.Fatalf("has after revoke: ok=%v err=%v", ok, err)
	}
}
