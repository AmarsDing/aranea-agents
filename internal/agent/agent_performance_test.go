package agent

import (
	"context"
	"fmt"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// Mock perf repo for AgentPerformance tests
// ---------------------------------------------------------------------------

type memAgentPerformanceRepo struct {
	items map[string]*biz.AgentPerformance // keyed by agentKey_taskType
}

func newMemAgentPerformanceRepo() *memAgentPerformanceRepo {
	return &memAgentPerformanceRepo{items: make(map[string]*biz.AgentPerformance)}
}

func (m *memAgentPerformanceRepo) Get(_ context.Context, agentKey, taskType string) (*biz.AgentPerformance, error) {
	key := agentKey + "_" + taskType
	p, ok := m.items[key]
	if !ok {
		return nil, fmt.Errorf("not found")
	}
	return p, nil
}

func (m *memAgentPerformanceRepo) GetBestForTaskType(_ context.Context, taskType string, limit int) ([]*biz.AgentPerformance, error) {
	var results []*biz.AgentPerformance
	for _, p := range m.items {
		if p.TaskType == taskType {
			results = append(results, p)
		}
	}
	// Sort by success rate descending (simple bubble sort for test)
	for i := 0; i < len(results); i++ {
		for j := i + 1; j < len(results); j++ {
			if results[j].SuccessRate > results[i].SuccessRate {
				results[i], results[j] = results[j], results[i]
			}
		}
	}
	if limit > 0 && len(results) > limit {
		results = results[:limit]
	}
	return results, nil
}

func (m *memAgentPerformanceRepo) Upsert(_ context.Context, perf *biz.AgentPerformance) error {
	key := perf.AgentKey + "_" + perf.TaskType
	m.items[key] = perf
	return nil
}

// ---------------------------------------------------------------------------
// TestAgentPerformance: Task 3.2
// ---------------------------------------------------------------------------

func TestAgentPerformance_GetBestForTaskType_PrioritizedInSubTask(t *testing.T) {
	perfRepo := newMemAgentPerformanceRepo()
	lg := loggateway.NewNoop()

	// Seed performance data: agent-a has better success rate than agent-b
	perfRepo.Upsert(context.Background(), &biz.AgentPerformance{
		AgentKey:    "agent-a",
		TaskType:    "go-backend",
		TotalRuns:   10,
		SuccessRuns: 9,
		SuccessRate: 0.9,
		AvgDQScore:  0.85,
	})
	perfRepo.Upsert(context.Background(), &biz.AgentPerformance{
		AgentKey:    "agent-b",
		TaskType:    "go-backend",
		TotalRuns:   5,
		SuccessRuns: 2,
		SuccessRate: 0.4,
		AvgDQScore:  0.3,
	})

	allocator := &agentAllocatorImpl{
		perfRepo:   perfRepo,
		capBuilder: nil,
		lg:         lg,
	}

	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Agent A", Roles: []string{"go-backend"}},
		{AgentKey: "agent-b", DisplayName: "Agent B", Roles: []string{"go-backend", "testing"}},
	}

	subTask := biz.SubTask{
		ID:                   "st_001",
		Name:                 "Implement API endpoint",
		Description:          "Create a new REST API endpoint",
		RequiredCapabilities: []string{"go-backend"},
		EstimatedComplexity:  0.3,
	}

	allocation, err := allocator.matchSubTask(context.Background(), subTask, capabilities, "trace-001")
	if err != nil {
		t.Fatalf("matchSubTask failed: %v", err)
	}

	// Agent-a should be selected because it has better performance history
	if allocation.AssignedKey != "agent-a" {
		t.Errorf("expected agent-a (best performance), got %q", allocation.AssignedKey)
	}
	if allocation.MatchLayer != "performance" {
		t.Errorf("expected match layer 'performance', got %q", allocation.MatchLayer)
	}
}

func TestAgentPerformance_GetBestForTaskType_FallbackWhenNoData(t *testing.T) {
	perfRepo := newMemAgentPerformanceRepo()
	lg := loggateway.NewNoop()

	// No performance data seeded
	allocator := &agentAllocatorImpl{
		perfRepo:   perfRepo,
		capBuilder: nil,
		lg:         lg,
	}

	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Agent A", Roles: []string{"go-backend"}},
		{AgentKey: "agent-b", DisplayName: "Agent B", Roles: []string{"vue3-frontend"}},
	}

	subTask := biz.SubTask{
		ID:                   "st_002",
		Name:                 "Implement API endpoint",
		Description:          "Create a new REST API endpoint",
		RequiredCapabilities: []string{"go-backend"},
		EstimatedComplexity:  0.3,
	}

	allocation, err := allocator.matchSubTask(context.Background(), subTask, capabilities, "trace-002")
	if err != nil {
		t.Fatalf("matchSubTask failed: %v", err)
	}

	// Should fall through to exact match (Layer 1) since no performance data
	if allocation.AssignedKey != "agent-a" {
		t.Errorf("expected agent-a (exact match fallback), got %q", allocation.AssignedKey)
	}
	if allocation.MatchLayer == "performance" {
		t.Error("should not use performance layer when no data exists")
	}
}

func TestAgentPerformance_GetBestForTaskType_WholePlan(t *testing.T) {
	perfRepo := newMemAgentPerformanceRepo()
	lg := loggateway.NewNoop()

	// Seed performance data
	perfRepo.Upsert(context.Background(), &biz.AgentPerformance{
		AgentKey:    "agent-c",
		TaskType:    "database",
		TotalRuns:   8,
		SuccessRuns: 7,
		SuccessRate: 0.875,
		AvgDQScore:  0.9,
	})

	allocator := &agentAllocatorImpl{
		perfRepo:   perfRepo,
		capBuilder: nil,
		lg:         lg,
	}

	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Agent A", Roles: []string{"go-backend"}},
		{AgentKey: "agent-c", DisplayName: "Agent C", Roles: []string{"database"}},
	}

	taskPlan := &biz.TaskPlan{
		UserMessage: "Optimize the database schema for better query performance",
		Strategy:    biz.StrategySingleAgent,
	}

	allocation, err := allocator.matchWholePlan(context.Background(), taskPlan, capabilities, "trace-003")
	if err != nil {
		t.Fatalf("matchWholePlan failed: %v", err)
	}

	// Agent-c should be selected because it has performance history for "database"
	if allocation.AssignedKey != "agent-c" {
		t.Errorf("expected agent-c (best performance for database), got %q", allocation.AssignedKey)
	}
	if allocation.MatchLayer != "performance" {
		t.Errorf("expected match layer 'performance', got %q", allocation.MatchLayer)
	}
}

func TestAgentPerformance_GetBestForTaskType_AgentNotInCapabilities(t *testing.T) {
	perfRepo := newMemAgentPerformanceRepo()
	lg := loggateway.NewNoop()

	// Seed performance data for an agent that is NOT in the current capabilities list
	perfRepo.Upsert(context.Background(), &biz.AgentPerformance{
		AgentKey:    "agent-x",
		TaskType:    "go-backend",
		TotalRuns:   10,
		SuccessRuns: 10,
		SuccessRate: 1.0,
		AvgDQScore:  1.0,
	})

	allocator := &agentAllocatorImpl{
		perfRepo:   perfRepo,
		capBuilder: nil,
		lg:         lg,
	}

	capabilities := []biz.AgentCapability{
		{AgentKey: "agent-a", DisplayName: "Agent A", Roles: []string{"go-backend"}},
	}

	subTask := biz.SubTask{
		ID:                   "st_003",
		Name:                 "Implement API endpoint",
		RequiredCapabilities: []string{"go-backend"},
		EstimatedComplexity:  0.3,
	}

	allocation, err := allocator.matchSubTask(context.Background(), subTask, capabilities, "trace-004")
	if err != nil {
		t.Fatalf("matchSubTask failed: %v", err)
	}

	// Should fall through to exact match since agent-x is not in capabilities
	if allocation.AssignedKey != "agent-a" {
		t.Errorf("expected agent-a (fallback when best agent not available), got %q", allocation.AssignedKey)
	}
}
