package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"

	"entgo.io/ent/dialect"
	entsql "entgo.io/ent/dialect/sql"
)

func TestEvalDeleteCascade(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db, DialectPostgres); err != nil {
		t.Fatal(err)
	}
	repo := NewEvalRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}, loggateway.NewNoop())

	ds, err := repo.CreateDataset(ctx, biz.EvalDataset{ID: "ds-1", Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertCasesWithCountUpdate(ctx, ds.ID, []biz.EvalCase{
		{ID: "c1", DatasetID: ds.ID, Input: "q", ExpectedOutput: "a"},
	}); err != nil {
		t.Fatal(err)
	}
	run, err := repo.CreateRun(ctx, biz.EvalRun{ID: "run-1", DatasetID: ds.ID, AgentID: "agent-1", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertCaseResult(ctx, biz.EvalCaseResult{ID: "res-1", RunID: run.ID, CaseID: "c1"}); err != nil {
		t.Fatal(err)
	}

	if err := repo.DeleteDataset(ctx, ds.ID); err != nil {
		t.Fatal(err)
	}
	cases, err := repo.ListCases(ctx, ds.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 0 {
		t.Fatalf("expected 0 cases after delete, got %d", len(cases))
	}
	runs, _, err := repo.ListRuns(ctx, ds.ID, "", 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(runs) != 0 {
		t.Fatalf("expected 0 runs after dataset delete, got %d", len(runs))
	}
	results, _, err := repo.ListCaseResults(ctx, run.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after dataset delete (cascade), got %d", len(results))
	}
}

func TestEvalDeleteRun(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db, DialectPostgres); err != nil {
		t.Fatal(err)
	}
	repo := NewEvalRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}, loggateway.NewNoop())
	ds, err := repo.CreateDataset(ctx, biz.EvalDataset{ID: "ds-r", Name: "rt"})
	if err != nil {
		t.Fatal(err)
	}
	run, err := repo.CreateRun(ctx, biz.EvalRun{ID: "run-r", DatasetID: ds.ID, AgentID: "agent-r", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertCaseResult(ctx, biz.EvalCaseResult{ID: "res-r", RunID: run.ID, CaseID: "c-r"}); err != nil {
		t.Fatal(err)
	}
	if err := repo.DeleteRun(ctx, run.ID); err != nil {
		t.Fatal(err)
	}
	results, _, err := repo.ListCaseResults(ctx, run.ID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results after delete run, got %d", len(results))
	}
}

func TestEvalUpdateDataset(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db, DialectPostgres); err != nil {
		t.Fatal(err)
	}
	repo := NewEvalRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}, loggateway.NewNoop())
	if _, err := repo.CreateDataset(ctx, biz.EvalDataset{ID: "ds-2", Name: "old", Description: "desc"}); err != nil {
		t.Fatal(err)
	}
	updated, err := repo.UpdateDataset(ctx, "ds-2", "new-name", "new-desc")
	if err != nil {
		t.Fatal(err)
	}
	if updated.Name != "new-name" || updated.Description != "new-desc" {
		t.Fatalf("update mismatch: %+v", updated)
	}
}

// TestEvalInsertCasesWithCountUpdateAtomic verifies that InsertCasesWithCountUpdate
// bumps dataset.case_count and inserts cases in one transaction. If a failure
// occurs mid-transaction (e.g. duplicate case ID), both writes must roll back so
// case_count cannot diverge from the actual row count in eval_cases.
func TestEvalInsertCasesWithCountUpdateAtomic(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db, DialectPostgres); err != nil {
		t.Fatal(err)
	}
	// Set up a real ent.Client so ExecInTx actually opens a transaction
	// (otherwise ExecInTx no-ops when entClient is nil and writes auto-commit).
	client := ent.NewClient(ent.Driver(entsql.OpenDB(dialect.Postgres, db)))
	t.Cleanup(func() { _ = client.Close() })
	d := &Data{}
	d.SetEntClientForTest(client, db, loggateway.NewNoop())
	repo := NewEvalRepo(d, loggateway.NewNoop())

	if _, err := repo.CreateDataset(ctx, biz.EvalDataset{ID: "ds-atomic", Name: "atomic"}); err != nil {
		t.Fatal(err)
	}

	// Happy path: 2 new cases → case_count becomes 2.
	if err := repo.InsertCasesWithCountUpdate(ctx, "ds-atomic", []biz.EvalCase{
		{ID: "c-a1", DatasetID: "ds-atomic", Input: "q1", ExpectedOutput: "a1"},
		{ID: "c-a2", DatasetID: "ds-atomic", Input: "q2", ExpectedOutput: "a2"},
	}); err != nil {
		t.Fatalf("happy path InsertCasesWithCountUpdate: %v", err)
	}
	ds, err := repo.GetDataset(ctx, "ds-atomic")
	if err != nil {
		t.Fatal(err)
	}
	if ds.CaseCount != 2 {
		t.Fatalf("expected case_count=2 after happy path, got %d", ds.CaseCount)
	}
	cases, err := repo.ListCases(ctx, "ds-atomic")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 case rows after happy path, got %d", len(cases))
	}

	// Failure path: include a duplicate ID (PRIMARY KEY conflict). The whole
	// transaction must roll back: case_count stays at 2, no new case rows.
	err = repo.InsertCasesWithCountUpdate(ctx, "ds-atomic", []biz.EvalCase{
		{ID: "c-a3", DatasetID: "ds-atomic", Input: "q3", ExpectedOutput: "a3"},
		{ID: "c-a1", DatasetID: "ds-atomic", Input: "dup", ExpectedOutput: "dup"}, // duplicate → conflict
	})
	if err == nil {
		t.Fatal("expected error from InsertCasesWithCountUpdate with duplicate ID, got nil")
	}
	ds, err = repo.GetDataset(ctx, "ds-atomic")
	if err != nil {
		t.Fatal(err)
	}
	if ds.CaseCount != 2 {
		t.Fatalf("expected case_count to remain 2 after rollback, got %d (diverged from rows)", ds.CaseCount)
	}
	cases, err = repo.ListCases(ctx, "ds-atomic")
	if err != nil {
		t.Fatal(err)
	}
	if len(cases) != 2 {
		t.Fatalf("expected 2 case rows after rollback, got %d", len(cases))
	}
}
