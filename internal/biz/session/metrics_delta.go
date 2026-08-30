package session

import "time"

type SessionMetricsDelta struct {
	SessionID           string
	MessageCount        int
	RunCount            int
	ModelCallCount      int
	ToolCallCount       int
	SkillCallCount      int
	McpCallCount        int
	InputTokens         int64
	OutputTokens        int64
	TotalTokens         int64
	TotalCostMicroUsd   int64
	// ErrorCount 是本批次内失败 run 数（turn status=error/failed；cancelled 是
	// 用户主动行为、timeout_degraded 有产出，均不计入）。R4-Q10 复测：
	// session_metrics.error_count 此前无任何写入方恒 0（S09 first_byte_timeout
	// critical 失败未计数）。
	ErrorCount          int
	LastMessageAt       string
	ContextUsedTokens   int
	ContextUsedRatio    float64
	MaxContextUsedRatio float64
	// LatencySumMs 是本批次内各 run 的墙钟耗时之和（ms）。样本基数 = RunCount
	// （每个 run 记账时必须同批携带其耗时，未观测到则计 0），落库端据此做
	// 滚动平均：avg = (avg*(run_count-RunCount) + LatencySumMs) / run_count。
	// R4-Q10：avg_latency_ms 此前无任何写入方，恒 0。
	LatencySumMs        int64
	AccumulatedCount    int
	FirstAccumulatedAt  time.Time
	// FlushFailCount 记录连续 flush 失败次数（SP-1c）。失败回炉时 +1，
	// 达到 MaxFlushFailCount 后 delta 被丢弃并告警，防无限重试循环。
	FlushFailCount int
}

const (
	MaxDeltaAge   = 5 * time.Minute
	MaxDeltaCount = 1000
	// MaxFlushFailCount 是单个 delta 批次的 flush 重试上限（SP-1c）。
	// flush 周期 200ms，5 次 ≈ 1s 重试窗口；超限批次带丢失统计量告警丢弃。
	MaxFlushFailCount = 5
)
