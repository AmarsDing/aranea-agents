package biz

import (
	"context"
	"strings"
)

// UsageModelInsight flags models with high cost and poor efficiency signals.
type UsageModelInsight struct {
	ProviderCode     string
	ModelAPIID       string
	ModelDisplayName string
	CallCount        int
	TotalTokens      int
	TotalCostMicroUSD int64
	AvgLatencyMS     float64
	AvgTokensPerSecond float64
	SuccessRate      float64
	Flags            []string
}

const (
	inefficientMinCalls       = 3
	inefficientCostMicroFloor = int64(100_000) // $0.10
	inefficientLowTPS         = 5.0
	inefficientLowSuccess     = 0.85
)

// InefficientModels returns top models in range that match high-cost + (low TPS or low success).
func (u *UsageUsecase) InefficientModels(ctx context.Context, query UsageQuery) ([]UsageModelInsight, error) {
	q := withUsageLimit(u.normalizeQuery(query, u.now()), 32)
	rows, err := u.repo.ListTopModelUsage(ctx, q)
	if err != nil {
		return nil, err
	}
	var out []UsageModelInsight
	for _, r := range rows {
		if r.CallCount < inefficientMinCalls || r.TotalCostMicroUSD < inefficientCostMicroFloor {
			continue
		}
		var flags []string
		if r.AvgTokensPerSecond > 0 && r.AvgTokensPerSecond < inefficientLowTPS {
			flags = append(flags, "low_tps")
		}
		if r.SuccessRate > 0 && r.SuccessRate < inefficientLowSuccess {
			flags = append(flags, "high_failure")
		}
		if r.TotalCostMicroUSD >= inefficientCostMicroFloor*10 {
			flags = append(flags, "high_cost")
		}
		if len(flags) == 0 {
			continue
		}
		out = append(out, UsageModelInsight{
			ProviderCode:       r.ProviderCode,
			ModelAPIID:         r.ModelAPIID,
			ModelDisplayName:   strings.TrimSpace(r.ModelDisplayName),
			CallCount:          r.CallCount,
			TotalTokens:        r.TotalTokens,
			TotalCostMicroUSD:  r.TotalCostMicroUSD,
			AvgLatencyMS:       r.AvgLatencyMS,
			AvgTokensPerSecond: r.AvgTokensPerSecond,
			SuccessRate:        r.SuccessRate,
			Flags:              flags,
		})
		if len(out) >= 8 {
			break
		}
	}
	return out, nil
}
