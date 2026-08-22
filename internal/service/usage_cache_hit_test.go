package service

import (
	"context"
	"testing"
	"time"

	v1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz/usage"
	"aranea-agents/internal/workspace"
)

// stubCacheHitUsageRepo adds the narrow CacheHitRatioStatsRepo capability to
// stubUsageRepo and records the window it was called with.
type stubCacheHitUsageRepo struct {
	stubUsageRepo
	stats     []usage.CacheHitRatioStat
	gotWindow time.Duration
}

func (s *stubCacheHitUsageRepo) CacheHitRatioStats(_ context.Context, window time.Duration) ([]usage.CacheHitRatioStat, error) {
	s.gotWindow = window
	return s.stats, nil
}

func newCacheHitUsageService(repo *stubCacheHitUsageRepo) *UsageService {
	return NewUsageService(usage.NewUsecase(repo, nil))
}

// window_hours clamping: 0/negative -> 1h default (alert window), >168 -> 168h,
// in-range values pass through.
func TestUsageService_GetCacheHitRatioStats_ClampsWindow(t *testing.T) {
	cases := []struct {
		name string
		in   int32
		want time.Duration
	}{
		{"zero defaults to 1h", 0, time.Hour},
		{"negative clamps to 1h", -5, time.Hour},
		{"in-range passthrough", 24, 24 * time.Hour},
		{"upper bound", 168, 168 * time.Hour},
		{"over upper bound clamps", 999, 168 * time.Hour},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			repo := &stubCacheHitUsageRepo{}
			svc := newCacheHitUsageService(repo)
			ctx := workspace.WithSystemWorkspace(context.Background())
			if _, err := svc.GetCacheHitRatioStats(ctx, &v1.GetCacheHitRatioStatsRequest{WindowHours: tc.in}); err != nil {
				t.Fatalf("GetCacheHitRatioStats: %v", err)
			}
			if repo.gotWindow != tc.want {
				t.Errorf("window = %v, want %v", repo.gotWindow, tc.want)
			}
		})
	}
}

// Field mapping from biz stat to proto message.
func TestUsageService_GetCacheHitRatioStats_MapsFields(t *testing.T) {
	repo := &stubCacheHitUsageRepo{stats: []usage.CacheHitRatioStat{
		{Provider: "deepseek", Model: "deepseek-chat", Samples: 25, PromptTok: 100000, CachedTok: 60000, WeightedRatio: 0.6, P50Ratio: 0.72},
	}}
	svc := newCacheHitUsageService(repo)
	ctx := workspace.WithSystemWorkspace(context.Background())

	resp, err := svc.GetCacheHitRatioStats(ctx, &v1.GetCacheHitRatioStatsRequest{WindowHours: 1})
	if err != nil {
		t.Fatalf("GetCacheHitRatioStats: %v", err)
	}
	if len(resp.GetStats()) != 1 {
		t.Fatalf("len(stats) = %d, want 1", len(resp.GetStats()))
	}
	got := resp.GetStats()[0]
	if got.GetProvider() != "deepseek" || got.GetModel() != "deepseek-chat" {
		t.Errorf("provider/model = %s/%s", got.GetProvider(), got.GetModel())
	}
	if got.GetSamples() != 25 || got.GetPromptTokens() != 100000 || got.GetCachedTokens() != 60000 {
		t.Errorf("tokens = samples %d prompt %d cached %d", got.GetSamples(), got.GetPromptTokens(), got.GetCachedTokens())
	}
	if got.GetWeightedRatio() != 0.6 || got.GetP50Ratio() != 0.72 {
		t.Errorf("ratios = weighted %v p50 %v", got.GetWeightedRatio(), got.GetP50Ratio())
	}
}

// Global observability metric: same caller rule as budget alerts — rejects
// non-system non-admin callers.
func TestUsageService_GetCacheHitRatioStats_RejectsNonSystemNonAdmin(t *testing.T) {
	repo := &stubCacheHitUsageRepo{}
	svc := newCacheHitUsageService(repo)
	ctx := workspace.WithContext(context.Background(), "ws-regular")

	if _, err := svc.GetCacheHitRatioStats(ctx, &v1.GetCacheHitRatioStatsRequest{}); err == nil {
		t.Fatal("expected Forbidden error for non-system non-admin caller")
	}
}
