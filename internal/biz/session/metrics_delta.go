package session

import "time"

type SessionMetricsDelta struct {
	SessionID         string
	MessageCount      int
	ModelCallCount    int
	ToolCallCount     int
	SkillCallCount    int
	McpCallCount      int
	InputTokens       int64
	OutputTokens      int64
	TotalTokens       int64
	TotalCostMicroUsd int64
	AccumulatedCount  int
	FirstAccumulatedAt time.Time
}

const (
	MaxDeltaAge   = 30 * time.Second
	MaxDeltaCount = 100
)
