package usage

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// TestAllModelsBreakdown_PassesQueryToRepo verifies that Usecase.AllModelsBreakdown
// normalizes pagination/defaults and passes the query to the repo.
// RED: fails because Usecase.AllModelsBreakdown doesn't exist yet.
func TestAllModelsBreakdown_PassesQueryToRepo(t *testing.T) {
	var capturedQuery BreakdownQuery
	mock := &mockUsageRepo{
		listAllModelsBreakdownFn: func(ctx context.Context, q BreakdownQuery) (BreakdownResult, error) {
			capturedQuery = q
			return BreakdownResult{
				Items:    []BreakdownRow{{ProviderCode: "openai", ModelAPIID: "gpt-4o", CallCount: 10}},
				Total:    1,
				Page:     1,
				PageSize: 20,
			}, nil
		},
	}
	u := NewUsecase(mock, loggateway.NewNoop())

	result, err := u.AllModelsBreakdown(context.Background(), BreakdownQuery{
		Range:     "30d",
		SortField: "total_cost_micro_usd",
		SortDir:   "desc",
		Page:      1,
		PageSize:  20,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(result.Items))
	}
	if result.Total != 1 {
		t.Fatalf("expected total=1, got %d", result.Total)
	}
	if result.Items[0].ProviderCode != "openai" {
		t.Fatalf("expected provider=openai, got %s", result.Items[0].ProviderCode)
	}
	// query should be passed through (with normalization if any)
	if capturedQuery.Range != "30d" {
		t.Fatalf("expected range=30d, got %s", capturedQuery.Range)
	}
}

// TestAllModelsBreakdown_PropagatesRepoError verifies that errors from the repo
// are propagated without modification.
func TestAllModelsBreakdown_PropagatesRepoError(t *testing.T) {
	mock := &mockUsageRepo{
		listAllModelsBreakdownFn: func(ctx context.Context, q BreakdownQuery) (BreakdownResult, error) {
			return BreakdownResult{}, errors.New("db connection lost")
		},
	}
	u := NewUsecase(mock, loggateway.NewNoop())

	_, err := u.AllModelsBreakdown(context.Background(), BreakdownQuery{Range: "30d"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
