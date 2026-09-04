package biz

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"time"

	"aranea-agents/internal/biz/usage"
	"aranea-agents/pkg/loggateway"
)

// ModelStatsReader 是注入统计所需的查询端口（usage.AnalyticsRepo 的子集）。
// 定义为窄接口以便测试 mock。
type ModelStatsReader interface {
	ListTopModelUsageFromDaily(ctx context.Context, q usage.Query) ([]usage.BreakdownRow, error)
}

// statsCache 缓存 30 天聚合统计数据，避免每次 List 都查 DB。
type statsCache struct {
	stats       map[string]ModelUsageStats
	hotness     map[string]float64
	percentiles map[string]LatencyPercentiles
	expireAt    time.Time
}

// ModelStatsInjector 在 List 时注入 30 天用量统计到 ProviderModel.ConfigJSON。
// 统计数据由 ModelStatsReader 查询，带 cacheTTL 缓存避免 N+1。
type ModelStatsInjector struct {
	reader   ModelStatsReader
	lg       loggateway.Logger
	cacheMu  sync.RWMutex
	cache    *statsCache
	cacheTTL time.Duration
}

// NewModelStatsInjector 创建统计注入器。cacheTTL 默认 5 分钟。
func NewModelStatsInjector(reader ModelStatsReader, lg loggateway.Logger) *ModelStatsInjector {
	return &ModelStatsInjector{
		reader:   reader,
		lg:       lg,
		cacheTTL: 5 * time.Minute,
	}
}

// InjectStats 将 30 天统计注入到 ProviderModel 列表的 ConfigJSON。
// 注入字段：model_hotness_score / usage_call_count_30d / usage_total_tokens_30d /
// usage_cost_micro_usd_30d / success_rate_30d / avg_latency_ms_30d；
// reader 实现 ModelLatencyReader 时追加 p50/p95_latency_ms_30d（抗单次劣化污染）。
// 不在统计列表中的模型保持统计字段缺失（前端显示「—」）。
func (inj *ModelStatsInjector) InjectStats(ctx context.Context, items []ProviderModel) {
	if inj == nil || inj.reader == nil {
		return
	}
	stats, hotness, percentiles, err := inj.loadStats(ctx)
	if err != nil {
		if inj.lg != nil {
			inj.lg.Warn("inject stats: load failed", loggateway.Err(err))
		}
		return
	}
	for i := range items {
		key := items[i].Provider + "/" + items[i].Model
		if s, ok := stats[key]; ok {
			items[i] = injectStatsIntoConfig(items[i], s, hotness[key], percentiles[key])
		}
	}
}

// loadStats 加载 30 天聚合统计（带缓存）。
func (inj *ModelStatsInjector) loadStats(ctx context.Context) (map[string]ModelUsageStats, map[string]float64, map[string]LatencyPercentiles, error) {
	// 先读缓存（读锁）
	inj.cacheMu.RLock()
	if inj.cache != nil && time.Now().Before(inj.cache.expireAt) {
		stats := inj.cache.stats
		hotness := inj.cache.hotness
		percentiles := inj.cache.percentiles
		inj.cacheMu.RUnlock()
		return stats, hotness, percentiles, nil
	}
	inj.cacheMu.RUnlock()

	// 缓存未命中，查 DB
	rows, err := inj.reader.ListTopModelUsageFromDaily(ctx, usage.Query{Range: "30d", Limit: 1000})
	if err != nil {
		return nil, nil, nil, err
	}

	statsList := make([]ModelUsageStats, 0, len(rows))
	statsMap := make(map[string]ModelUsageStats, len(rows))
	for _, r := range rows {
		s := ModelUsageStats{
			ProviderCode:      r.ProviderCode,
			ModelAPIID:        r.ModelAPIID,
			CallCount:         r.CallCount,
			TotalTokens:       r.TotalTokens,
			TotalCostMicroUSD: r.TotalCostMicroUSD,
			SuccessRate:       r.SuccessRate,
			AvgLatencyMS:      r.AvgLatencyMS,
			LastActiveDate:    r.LastActiveDate,
		}
		statsList = append(statsList, s)
		statsMap[r.ProviderCode+"/"+r.ModelAPIID] = s
	}

	hotness := ComputeHotness(statsList)

	// 可选分位数端口：reader 支持则并入（失败不阻断主统计——p50/p95 是展示增强）。
	percentiles := map[string]LatencyPercentiles{}
	if lr, ok := inj.reader.(ModelLatencyReader); ok {
		if rows, lerr := lr.ListModelLatencyPercentiles(ctx, 30); lerr != nil {
			if inj.lg != nil {
				inj.lg.Warn("inject stats: latency percentiles load failed", loggateway.Err(lerr))
			}
		} else {
			for _, p := range rows {
				percentiles[p.ProviderCode+"/"+p.ModelAPIID] = p
			}
		}
	}

	// 更新缓存（写锁）
	inj.cacheMu.Lock()
	inj.cache = &statsCache{
		stats:       statsMap,
		hotness:     hotness,
		percentiles: percentiles,
		expireAt:    time.Now().Add(inj.cacheTTL),
	}
	inj.cacheMu.Unlock()

	return statsMap, hotness, percentiles, nil
}

// injectedStatsConfigKeys 是 InjectStats 写入 ConfigJSON 的全部响应装饰键。
// 这些键是「读取时计算」的展示数据，不属于模型配置——写路径
// （ProcessConfigJSONForStorage）必须剥离，否则前端「GET 列表（带注入）→ 编辑
// → PATCH 整包回写」会把瞬态统计固化进 DB（2026-09-03 108 实证：glm-4.7 存库
// config_json 被污染 usage_call_count_30d/avg_latency_ms_30d 等 6 键，统计窗口
// 过后仍显示陈旧值）。
var injectedStatsConfigKeys = []string{
	"model_hotness_score",
	"usage_call_count_30d",
	"usage_total_tokens_30d",
	"usage_cost_micro_usd_30d",
	"success_rate_30d",
	"avg_latency_ms_30d",
	"p50_latency_ms_30d",
	"p95_latency_ms_30d",
}

// stripInjectedStatsKeys 从待持久化的 config map 中删除全部响应装饰键。
func stripInjectedStatsKeys(m map[string]any) {
	for _, k := range injectedStatsConfigKeys {
		delete(m, k)
	}
}

// injectStatsIntoConfig 将统计数据注入到单个 ProviderModel 的 ConfigJSON。
// 保留原有字段（如 api_key_set），只新增/覆盖统计字段。
// p 为可选分位数（P50>0 时注入 p50/p95_latency_ms_30d）。
func injectStatsIntoConfig(m ProviderModel, s ModelUsageStats, hotness float64, p LatencyPercentiles) ProviderModel {
	cfg := strings.TrimSpace(m.ConfigJSON)
	if cfg == "" {
		cfg = "{}"
	}
	var cm map[string]any
	if err := json.Unmarshal([]byte(cfg), &cm); err != nil {
		cm = make(map[string]any)
	}
	cm["model_hotness_score"] = hotness
	cm["usage_call_count_30d"] = s.CallCount
	cm["usage_total_tokens_30d"] = s.TotalTokens
	cm["usage_cost_micro_usd_30d"] = s.TotalCostMicroUSD
	cm["success_rate_30d"] = s.SuccessRate
	cm["avg_latency_ms_30d"] = s.AvgLatencyMS
	if p.P50LatencyMS > 0 {
		cm["p50_latency_ms_30d"] = p.P50LatencyMS
		cm["p95_latency_ms_30d"] = p.P95LatencyMS
	}
	out, err := json.Marshal(cm)
	if err != nil {
		return m
	}
	m.ConfigJSON = string(out)
	return m
}
