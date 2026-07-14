package biz

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
}

// 热度计算权重（简化版，去掉"最近使用时间"修正，权重并入调用次数）。
// 参考 docs/development/9-provider.md §3.1：
//   热度 = 调用次数标准分*0.45 + Token标准分*0.25 + 费用标准分*0.15 + 成功率修正*0.10 + 最近使用时间修正*0.05
// 简化后（无最近使用时间数据）：
//   热度 = 调用次数标准分*0.50 + Token标准分*0.25 + 费用标准分*0.15 + 成功率*0.10
const (
	weightCallCount   = 0.50
	weightTokens      = 0.25
	weightCost        = 0.15
	weightSuccessRate = 0.10
)

// standardize 对 value 做 Min-Max 标准化到 [0, 100]。
// 当 min == max（所有模型该维度值相同）时返回 50（中等），避免除零。
func standardize(value, min, max float64) float64 {
	if max == min {
		return 50
	}
	return (value - min) / (max - min) * 100
}

// ComputeHotness 根据设计文档 §3.1 计算每个模型的热度分数（0-100）。
// key 格式为 "provider_code/model_api_id"。
// 当 stats 为空时返回空 map。
func ComputeHotness(stats []ModelUsageStats) map[string]float64 {
	if len(stats) == 0 {
		return map[string]float64{}
	}

	// 计算各维度的 min/max
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

		hotness := callScore*weightCallCount +
			tokenScore*weightTokens +
			costScore*weightCost +
			successScore*weightSuccessRate

		// 限制到 [0, 100]
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
