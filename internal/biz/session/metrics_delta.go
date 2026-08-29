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
}

const (
	MaxDeltaAge   = 5 * time.Minute
	MaxDeltaCount = 1000
)
