package biz

import "context"

// DQSuccessThreshold is the DQ cut-off for counting one orchestration run as
// a success in an agent's track record（SuccessRuns/SuccessRate）。与
// DQEvolutionThreshold 同值但语义不同：后者是"低于则触发进化建议"的拓扑质量线，
// 本常量是"达到则本次运行记入成功履历"的及格线。
const DQSuccessThreshold = 0.5

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
