package data

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/testhelper"
	"aranea-agents/pkg/loggateway"
)

// Regression: GetRunsByIDs built the IN list with Dialect().Placeholders
// (which emits $1..$n on Postgres) while the workspace filter still used ?,
// and the combined SQL then went through RenumberPlaceholders — renumbering
// the ? from $1 again, colliding with the IN-list placeholders. Postgres
// rejected the statement with a bind-param count mismatch (500 on the
// compare/preference endpoints). SQLite never sees this (no renumbering),
// so the coverage must run against Postgres.
func TestGetRunsByIDsPostgres(t *testing.T) {
	db := testhelper.SetupTestPGRaw(t)
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db, DialectPostgres); err != nil {
		t.Fatal(err)
	}
	repo := NewEvalRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop(), dialect: DialectPostgres}, loggateway.NewNoop())

	ds, err := repo.CreateDataset(ctx, biz.EvalDataset{ID: "ds-grb", Name: "test"})
	if err != nil {
		t.Fatal(err)
	}
	run1, err := repo.CreateRun(ctx, biz.EvalRun{ID: "run-grb-1", DatasetID: ds.ID, AgentID: "agent-1", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}
	run2, err := repo.CreateRun(ctx, biz.EvalRun{ID: "run-grb-2", DatasetID: ds.ID, AgentID: "agent-1", Status: "completed"})
	if err != nil {
		t.Fatal(err)
	}

	runs, err := repo.GetRunsByIDs(ctx, []string{run1.ID, run2.ID})
	if err != nil {
		t.Fatalf("GetRunsByIDs: %v", err)
	}
	if len(runs) != 2 {
		t.Fatalf("expected 2 runs, got %d", len(runs))
	}
}
