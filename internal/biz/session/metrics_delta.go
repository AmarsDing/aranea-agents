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
	LastMessageAt       string
	ContextUsedTokens   int
	ContextUsedRatio    float64
	MaxContextUsedRatio float64
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
