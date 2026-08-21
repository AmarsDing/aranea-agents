package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// TestAllModelsBreakdown_PassesQueryToRepo verifies that Usecase.AllModelsBreakdown
// normalizes pagination/defaults and passes the query to the repo.
// Range "30d" includes today, so the realtime events path is taken (see
// Usecase.AllModelsBreakdown source-selection rule).
func TestAllModelsBreakdown_PassesQueryToRepo(t *testing.T) {
	var capturedQuery BreakdownQuery
	mock := &mockUsageRepo{
		listAllModelsBreakdownRealtimeFn: func(ctx context.Context, q BreakdownQuery) (BreakdownResult, error) {
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
		listAllModelsBreakdownRealtimeFn: func(ctx context.Context, q BreakdownQuery) (BreakdownResult, error) {
			return BreakdownResult{}, errors.New("db connection lost")
		},
	}
	u := NewUsecase(mock, loggateway.NewNoop())

	_, err := u.AllModelsBreakdown(context.Background(), BreakdownQuery{Range: "30d"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// TestAllModelsBreakdown_UsesDailyForPastRange verifies the source-selection rule:
// a range ending before today reads the daily rollup table, never the realtime path.
func TestAllModelsBreakdown_UsesDailyForPastRange(t *testing.T) {
	dailyCalled := false
	mock := &mockUsageRepo{
		listAllModelsBreakdownFn: func(ctx context.Context, q BreakdownQuery) (BreakdownResult, error) {
			dailyCalled = true
			return BreakdownResult{Page: 1, PageSize: 20}, nil
		},
		listAllModelsBreakdownRealtimeFn: func(ctx context.Context, q BreakdownQuery) (BreakdownResult, error) {
			t.Error("realtime path must not be used for a past-only range")
			return BreakdownResult{}, nil
		},
	}
	u := NewUsecase(mock, loggateway.NewNoop())

	past := time.Now().UTC().AddDate(0, 0, -10)
	_, err := u.AllModelsBreakdown(context.Background(), BreakdownQuery{
		StartDate: past.Format("2006-01-02"),
		EndDate:   past.AddDate(0, 0, 1).Format("2006-01-02"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !dailyCalled {
		t.Fatal("expected daily rollup path to be used for past-only range")
	}
}
