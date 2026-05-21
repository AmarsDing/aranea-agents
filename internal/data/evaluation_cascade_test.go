package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
)

func TestEvalDeleteCascade(t *testing.T) {
	db, err := sql.Open("sqlite", "file:eval-cascade-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewEvalRepo(db)

	ds, err := repo.CreateDataset(ctx, biz.EvalDataset{ID: "ds-1", Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	if err := repo.InsertCases(ctx, []biz.EvalCase{
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
	db, err := sql.Open("sqlite", "file:eval-update-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewEvalRepo(db)
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
