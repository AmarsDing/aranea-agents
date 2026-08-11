package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// fakeReviewFactDeps records the last ReviewFactRow call and returns a fixed
// fact row so the handler hydration path can be asserted.
type fakeReviewFactDeps struct {
	biz.MemoryAdminDeps // nil-embedded; only ReviewFactRow is callable

	calls  int
	gotIn  biz.FactReview
	rowJSON []byte
	err    error
}

func (f *fakeReviewFactDeps) ReviewFactRow(_ context.Context, in biz.FactReview) ([]byte, error) {
	f.calls++
	f.gotIn = in
	return f.rowJSON, f.err
}

// TestReviewMemoryFact_Validation pins the handler contract: fact_id/action
// are required, action must be in the whitelist, refine requires a statement,
// and validation failures never reach the store.
func TestReviewMemoryFact_Validation(t *testing.T) {
	deps := &fakeReviewFactDeps{}
	admin := biz.NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	svc := NewMemoryService(MemoryServiceConfig{Admin: admin, Logger: loggateway.NewNoop()})
	ctx := workspace.WithSystemWorkspace(context.Background())

	cases := []struct {
		name string
		req  *v1.ReviewMemoryFactRequest
	}{
		{"missing fact_id", &v1.ReviewMemoryFactRequest{Action: "confirm"}},
		{"missing action", &v1.ReviewMemoryFactRequest{FactId: "f1"}},
		{"unknown action", &v1.ReviewMemoryFactRequest{FactId: "f1", Action: "explode"}},
		{"refine without statement", &v1.ReviewMemoryFactRequest{FactId: "f1", Action: "refine"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := svc.ReviewMemoryFact(ctx, tc.req); err == nil {
				t.Fatal("expected validation error")
			}
		})
	}
	if deps.calls != 0 {
		t.Fatalf("store calls = %d, want 0 (validation must short-circuit)", deps.calls)
	}
}

// TestReviewMemoryFact_PassesThrough verifies a valid request reaches the
// store with all fields mapped and the returned row is hydrated.
func TestReviewMemoryFact_PassesThrough(t *testing.T) {
	deps := &fakeReviewFactDeps{
		rowJSON: []byte(`{"id":"f1","statement":"User likes coffee","status":"active","version":2}`),
	}
	admin := biz.NewMemoryAdminUsecase(deps, nil, nil, nil, loggateway.NewNoop())
	svc := NewMemoryService(MemoryServiceConfig{Admin: admin, Logger: loggateway.NewNoop()})
	ctx := workspace.WithSystemWorkspace(context.Background())

	resp, err := svc.ReviewMemoryFact(ctx, &v1.ReviewMemoryFactRequest{
		FactId:          "f1",
		Action:          "refine",
		Statement:       "User likes coffee",
		DetailsMarkdown: "edited",
		FactKind:        "preference",
		TagsJson:        `["drink"]`,
	})
	if err != nil {
		t.Fatalf("ReviewMemoryFact: %v", err)
	}
	if deps.calls != 1 {
		t.Fatalf("store calls = %d, want 1", deps.calls)
	}
	if deps.gotIn.FactID != "f1" || deps.gotIn.Action != biz.FactReviewRefine {
		t.Errorf("gotIn = %+v, want FactID=f1 Action=refine", deps.gotIn)
	}
	if deps.gotIn.Statement != "User likes coffee" || deps.gotIn.TagsJSON != `["drink"]` {
		t.Errorf("refine fields not mapped: %+v", deps.gotIn)
	}
	if resp.GetFact().GetId() != "f1" || resp.GetFact().GetStatement() != "User likes coffee" {
		t.Errorf("response not hydrated: %+v", resp.GetFact())
	}
}
