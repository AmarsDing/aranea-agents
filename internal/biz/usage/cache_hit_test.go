package usage

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

type mockCacheHitRepo struct {
	mockUsageRepo
	stats     []CacheHitRatioStat
	err       error
	gotWindow time.Duration
}

func (m *mockCacheHitRepo) CacheHitRatioStats(_ context.Context, window time.Duration) ([]CacheHitRatioStat, error) {
	m.gotWindow = window
	return m.stats, m.err
}

// Usecase.CacheHitRatioStats 委托窄接口 CacheHitRatioStatsRepo，透传窗口与结果。
func TestCacheHitRatioStats_Delegates(t *testing.T) {
	repo := &mockCacheHitRepo{stats: []CacheHitRatioStat{
		{Provider: "deepseek", Model: "deepseek-chat", Samples: 25, PromptTok: 100000, CachedTok: 60000, WeightedRatio: 0.6, P50Ratio: 0.72},
	}}
	uc := NewUsecase(repo, loggateway.NewNoop())

	stats, err := uc.CacheHitRatioStats(context.Background(), 24*time.Hour)
	if err != nil {
		t.Fatalf("CacheHitRatioStats: %v", err)
	}
	if repo.gotWindow != 24*time.Hour {
		t.Errorf("repo window = %v, want 24h", repo.gotWindow)
	}
	if len(stats) != 1 || stats[0].P50Ratio != 0.72 {
		t.Errorf("stats = %+v, want 1 row p50 0.72", stats)
	}
}

// repo 未实现窄接口时返回空（不报错）——与 wire.go 类型断言收窄模式一致。
func TestCacheHitRatioStats_RepoWithoutCapability(t *testing.T) {
	uc := NewUsecase(&mockUsageRepo{}, loggateway.NewNoop())
	stats, err := uc.CacheHitRatioStats(context.Background(), time.Hour)
	if err != nil {
		t.Fatalf("CacheHitRatioStats: %v", err)
	}
	if len(stats) != 0 {
		t.Errorf("stats = %+v, want empty for repo without CacheHitRatioStatsRepo", stats)
	}
}

func TestCacheHitRatioStats_RepoError(t *testing.T) {
	repo := &mockCacheHitRepo{err: errors.New("db down")}
	uc := NewUsecase(repo, loggateway.NewNoop())
	if _, err := uc.CacheHitRatioStats(context.Background(), time.Hour); err == nil {
		t.Fatal("want error propagated from repo")
	}
}

type mockRunCacheHitRepo struct {
	mockUsageRepo
	hit      RunCacheHitRatio
	err      error
	gotRunID string
}

func (m *mockRunCacheHitRepo) RunCacheHitRatio(_ context.Context, runID string) (RunCacheHitRatio, error) {
	m.gotRunID = runID
	return m.hit, m.err
}

// Usecase.RunCacheHitRatio 委托窄接口 RunCacheHitRatioRepo，透传 runID 与结果。
func TestRunCacheHitRatio_Delegates(t *testing.T) {
	repo := &mockRunCacheHitRepo{hit: RunCacheHitRatio{Found: true, PromptTok: 4000, CachedTok: 3000, Ratio: 0.75}}
	uc := NewUsecase(repo, loggateway.NewNoop())

	hit, err := uc.RunCacheHitRatio(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("RunCacheHitRatio: %v", err)
	}
	if repo.gotRunID != "run-1" {
		t.Errorf("repo runID = %q, want run-1", repo.gotRunID)
	}
	if !hit.Found || hit.Ratio != 0.75 {
		t.Errorf("hit = %+v, want found ratio 0.75", hit)
	}
}

// repo 未实现窄接口时 Found=false（不报错）——调用方据此区分"无数据"。
func TestRunCacheHitRatio_RepoWithoutCapability(t *testing.T) {
	uc := NewUsecase(&mockUsageRepo{}, loggateway.NewNoop())
	hit, err := uc.RunCacheHitRatio(context.Background(), "run-1")
	if err != nil {
		t.Fatalf("RunCacheHitRatio: %v", err)
	}
	if hit.Found {
		t.Errorf("hit = %+v, want Found=false for repo without RunCacheHitRatioRepo", hit)
	}
}

func TestRunCacheHitRatio_RepoError(t *testing.T) {
	repo := &mockRunCacheHitRepo{err: errors.New("db down")}
	uc := NewUsecase(repo, loggateway.NewNoop())
	if _, err := uc.RunCacheHitRatio(context.Background(), "run-1"); err == nil {
		t.Fatal("want error propagated from repo")
	}
}
