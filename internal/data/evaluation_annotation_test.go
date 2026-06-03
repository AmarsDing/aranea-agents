package data

import (
	"context"
	"database/sql"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestEvalCaseResultAnnotation(t *testing.T) {
	db, err := sql.Open("sqlite", "file:eval-annotation-test?mode=memory&cache=shared")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	ctx := context.Background()
	if err := EnsureEvalSchema(ctx, db); err != nil {
		t.Fatal(err)
	}
	repo := NewEvalRepo(&Data{rawDB: db, readDB: db, rwDB: NewReadWriteDB(db, db), lg: loggateway.NewNoop()}, loggateway.NewNoop())
	runID := "run-1"
	if err := repo.InsertCaseResult(ctx, biz.EvalCaseResult{
		ID:     "res-1",
		RunID:  runID,
		CaseID: "case-1",
	}); err != nil {
		t.Fatal(err)
	}
	pass := true
	score := float32(0.9)
	comment := "looks good"
	updated, err := repo.UpdateCaseResultAnnotation(ctx, runID, "res-1", biz.EvalCaseResultAnnotation{
		HumanPass:    &pass,
		HumanScore:   &score,
		HumanComment: &comment,
		AnnotatedBy:  "tester",
	})
	if err != nil {
		t.Fatal(err)
	}
	if updated.HumanPass == nil || !*updated.HumanPass {
		t.Fatal("expected human pass true")
	}
	if updated.AnnotatedBy != "tester" {
		t.Fatalf("annotated_by=%q", updated.AnnotatedBy)
	}
	items, _, err := repo.ListCaseResults(ctx, runID, 10, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].HumanComment != comment {
		t.Fatalf("list mismatch: %+v", items)
	}
}
