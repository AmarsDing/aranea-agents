package biz

import (
	"context"
	"strings"
	"time"
)

// ModelUsageStats 是计算模型热度所需的单模型统计数据。
// 数据来源：usage.AnalyticsRepo.ListTopModelUsageFromDaily 的 30 天聚合。
type ModelUsageStats struct {
	ProviderCode      string
	ModelAPIID        string
	CallCount         int
	TotalTokens       int
	TotalCostMicroUSD int64
	SuccessRate       float64 // 0-1
	AvgLatencyMS      float64 // 注入到 config_json.avg_latency_ms_30d（不参与 hotness 计算）
	LastActiveDate    string  // YYYY-MM-DD；最近有用量的日桶
}

// 热度计算权重（对齐 docs/development/9-provider.md §3.1）：
//
//	热度 = 调用次数标准分*0.45 + Token标准分*0.25 + 费用标准分*0.15 + 成功率修正*0.10 + 最近使用时间修正*0.05
const (
	weightCallCount   = 0.45
	weightTokens      = 0.25
	weightCost        = 0.15
	weightSuccessRate = 0.10
	weightRecency     = 0.05
	recencyWindowDays = 30.0
)

// standardize 对 value 做 Min-Max 标准化到 [0, 100]。
// 当 min == max（所有模型该维度值相同）时返回 50（中等），避免除零。
func standardize(value, min, max float64) float64 {
	if max == min {
		return 50
	}
	return (value - min) / (max - min) * 100
}

// recencyScore maps LastActiveDate to [0,100]: today=100, ≥30 days ago / empty=0.
func recencyScore(lastActiveDate string, now time.Time) float64 {
	raw := strings.TrimSpace(lastActiveDate)
	if raw == "" {
		return 0
	}
	d, err := time.Parse("2006-01-02", raw)
	if err != nil {
		return 0
	}
	days := now.UTC().Truncate(24*time.Hour).Sub(d.UTC().Truncate(24*time.Hour)).Hours() / 24
	if days <= 0 {
		return 100
	}
	if days >= recencyWindowDays {
		return 0
	}
	return (1 - days/recencyWindowDays) * 100
}

// ComputeHotness 根据设计文档 §3.1 计算每个模型的热度分数（0-100）。
// key 格式为 "provider_code/model_api_id"。
// 当 stats 为空时返回空 map。
func ComputeHotness(stats []ModelUsageStats) map[string]float64 {
	return ComputeHotnessAt(stats, time.Now())
}

// ComputeHotnessAt is ComputeHotness with an injectable clock (for tests).
func ComputeHotnessAt(stats []ModelUsageStats, now time.Time) map[string]float64 {
	if len(stats) == 0 {
		return map[string]float64{}
	}

	var minCalls, maxCalls float64 = float64(stats[0].CallCount), float64(stats[0].CallCount)
	var minTokens, maxTokens float64 = float64(stats[0].TotalTokens), float64(stats[0].TotalTokens)
	var minCost, maxCost float64 = float64(stats[0].TotalCostMicroUSD), float64(stats[0].TotalCostMicroUSD)
	for _, s := range stats[1:] {
		calls := float64(s.CallCount)
		tokens := float64(s.TotalTokens)
		cost := float64(s.TotalCostMicroUSD)
		if calls < minCalls {
			minCalls = calls
		}
		if calls > maxCalls {
			maxCalls = calls
		}
		if tokens < minTokens {
			minTokens = tokens
		}
		if tokens > maxTokens {
			maxTokens = tokens
		}
		if cost < minCost {
			minCost = cost
		}
		if cost > maxCost {
			maxCost = cost
		}
	}

	result := make(map[string]float64, len(stats))
	for _, s := range stats {
		callScore := standardize(float64(s.CallCount), minCalls, maxCalls)
		tokenScore := standardize(float64(s.TotalTokens), minTokens, maxTokens)
		costScore := standardize(float64(s.TotalCostMicroUSD), minCost, maxCost)
		successScore := s.SuccessRate * 100
		recentScore := recencyScore(s.LastActiveDate, now)

		hotness := callScore*weightCallCount +
			tokenScore*weightTokens +
			costScore*weightCost +
			successScore*weightSuccessRate +
			recentScore*weightRecency

		if hotness < 0 {
			hotness = 0
		}
		if hotness > 100 {
			hotness = 100
		}
		result[s.ProviderCode+"/"+s.ModelAPIID] = hotness
	}
	return result
}

// LatencyPercentiles 单模型延迟分位数（30 天窗口，来自原始事件表）。
// 动机（2026-09-03 实证）：avg_latency_ms_30d 是按请求数加权的均值，
// 单次供应商劣化（TTFT 211s）即可把均值污染一个数量级；p50/p95 抗噪，
// 与均值并列展示可区分「整体变慢」与「偶发尖刺」。
type LatencyPercentiles struct {
	ProviderCode string
	ModelAPIID   string
	P50LatencyMS float64
	P95LatencyMS float64
}

// ModelLatencyReader 可选扩展端口（data 层 usage repo 实现）。
// ModelStatsInjector 对 reader 做类型断言，支持即注入分位数字段，
// 不支持则静默跳过——测试 mock 与旧实现零成本兼容。
type ModelLatencyReader interface {
	ListModelLatencyPercentiles(ctx context.Context, days int) ([]LatencyPercentiles, error)
}
