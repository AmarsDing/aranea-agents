package biz

import "context"

// AgentPerformance tracks an agent's execution history
type AgentPerformance struct {
	AgentKey       string
	TaskType       string
	TotalRuns      int
	SuccessRuns    int
	SuccessRate    float64
	AvgDQScore     float64
	AvgDurationMs  int64
	LastExecutedAt string
}

// AgentPerformanceRepository is the repository interface for AgentPerformance
type AgentPerformanceRepository interface {
	Get(ctx context.Context, agentKey, taskType string) (*AgentPerformance, error)
	GetBestForTaskType(ctx context.Context, taskType string, limit int) ([]*AgentPerformance, error)
	Upsert(ctx context.Context, perf *AgentPerformance) error
}
