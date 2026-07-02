package service

import (
	"testing"
	"time"

	"aranea-agents/internal/biz"
)

// TestToTaskPlanSummary verifies the conversion from biz.TaskPlan to proto TaskPlanSummary.
func TestToTaskPlanSummary(t *testing.T) {
	now := time.Now().UTC()
	plan := &biz.TaskPlan{
		ID:              "plan-001",
		SpiritSessionID: "sess-001",
		TraceID:         "trace-001",
		UserMessage:     "Build a web app",
		ComplexityLevel: biz.ComplexityComplex,
		ComplexityScore: 0.85,
		Strategy:        biz.StrategyDAG,
		Status:          biz.LegacyPlanStatusConfirmed,
		SubTasks:        []biz.SubTask{{ID: "st-1"}, {ID: "st-2"}, {ID: "st-3"}},
		CreatedAt:       now,
		UpdatedAt:       now,
	}

	summary := toTaskPlanSummary(plan)
	if summary == nil {
		t.Fatal("expected non-nil summary")
	}
	if summary.PlanId != "plan-001" {
		t.Errorf("PlanId: got %q, want %q", summary.PlanId, "plan-001")
	}
	if summary.SessionId != "sess-001" {
		t.Errorf("SessionId: got %q, want %q", summary.SessionId, "sess-001")
	}
	if summary.TraceId != "trace-001" {
		t.Errorf("TraceId: got %q, want %q", summary.TraceId, "trace-001")
	}
	if summary.UserMessage != "Build a web app" {
		t.Errorf("UserMessage: got %q, want %q", summary.UserMessage, "Build a web app")
	}
	if summary.ComplexityLevel != "complex" {
		t.Errorf("ComplexityLevel: got %q, want %q", summary.ComplexityLevel, "complex")
	}
	if summary.ComplexityScore != 0.85 {
		t.Errorf("ComplexityScore: got %f, want %f", summary.ComplexityScore, 0.85)
	}
	if summary.Strategy != "dag" {
		t.Errorf("Strategy: got %q, want %q", summary.Strategy, "dag")
	}
	if summary.Status != "confirmed" {
		t.Errorf("Status: got %q, want %q", summary.Status, "confirmed")
	}
	if summary.SubtaskCount != 3 {
		t.Errorf("SubtaskCount: got %d, want %d", summary.SubtaskCount, 3)
	}
	if summary.CreatedAt == "" {
		t.Error("CreatedAt should not be empty")
	}
	if summary.UpdatedAt == "" {
		t.Error("UpdatedAt should not be empty")
	}
}

// TestToTaskPlanSummary_NilPlan verifies nil plan returns nil.
func TestToTaskPlanSummary_NilPlan(t *testing.T) {
	if summary := toTaskPlanSummary(nil); summary != nil {
		t.Errorf("expected nil for nil plan, got %+v", summary)
	}
}

// TestToTaskPlanDetail verifies the conversion from biz.TaskPlan to proto TaskPlanDetail.
func TestToTaskPlanDetail(t *testing.T) {
	now := time.Now().UTC()
	plan := &biz.TaskPlan{
		ID:                 "plan-002",
		SpiritSessionID:    "sess-002",
		TraceID:            "trace-002",
		UserMessage:        "Analyze data",
		IntentArtifactJSON: `{"intent":"analysis"}`,
		ComplexityLevel:    biz.ComplexityModerate,
		ComplexityScore:    0.45,
		Dimensions: biz.DimensionScores{
			Semantic:   0.5,
			Structural: 0.3,
			Domain:     0.4,
			Tool:       0.2,
			Context:    0.6,
			Historical: 0.7,
		},
		SubTasks: []biz.SubTask{
			{
				ID:                   "st-1",
				Name:                 "Fetch data",
				Description:          "Fetch from API",
				DependsOn:            []string{},
				RequiredCapabilities: []string{"http"},
				Priority:             1,
				EstimatedComplexity:  0.3,
			},
			{
				ID:                   "st-2",
				Name:                 "Process",
				Description:          "Transform data",
				DependsOn:            []string{"st-1"},
				RequiredCapabilities: []string{"compute"},
				Priority:             2,
				EstimatedComplexity:  0.5,
			},
		},
		TaskDAG: &biz.PlanTaskDAG{
			Nodes:   []biz.SubTask{{ID: "st-1"}, {ID: "st-2"}},
			RootIDs: []string{"st-1"},
			LeafIDs: []string{"st-2"},
		},
		DecomposeReason: "Multiple steps required",
		Strategy:        biz.StrategyParallel,
		StrategyReason:  "Linear dependency chain",
		TopologyHint:    biz.TopologySequential,
		MemoryHit: &biz.MemoryHit{
			CacheID:       "cache-001",
			DQScore:       0.92,
			TopologyUsed:  "linear",
			AgentKeysUsed: []string{"agent-a", "agent-b"},
		},
		Status:    biz.LegacyPlanStatusExecuting,
		CreatedAt: now,
		UpdatedAt: now,
	}

	detail := toTaskPlanDetail(plan)
	if detail == nil {
		t.Fatal("expected non-nil detail")
	}

	// Verify basic fields.
	if detail.PlanId != "plan-002" {
		t.Errorf("PlanId: got %q, want %q", detail.PlanId, "plan-002")
	}
	if detail.IntentArtifactJson != `{"intent":"analysis"}` {
		t.Errorf("IntentArtifactJson: got %q", detail.IntentArtifactJson)
	}
	if detail.DecomposeReason != "Multiple steps required" {
		t.Errorf("DecomposeReason: got %q, want %q", detail.DecomposeReason, "Multiple steps required")
	}
	if detail.StrategyReason != "Linear dependency chain" {
		t.Errorf("StrategyReason: got %q, want %q", detail.StrategyReason, "Linear dependency chain")
	}

	// Verify dimensions.
	if detail.Dimensions == nil {
		t.Fatal("expected non-nil Dimensions")
	}
	if detail.Dimensions.Semantic != 0.5 {
		t.Errorf("Dimensions.Semantic: got %f, want %f", detail.Dimensions.Semantic, 0.5)
	}
	if detail.Dimensions.Historical != 0.7 {
		t.Errorf("Dimensions.Historical: got %f, want %f", detail.Dimensions.Historical, 0.7)
	}

	// Verify subtasks.
	if len(detail.SubTasks) != 2 {
		t.Fatalf("SubTasks: got %d, want %d", len(detail.SubTasks), 2)
	}
	if detail.SubTasks[0].Id != "st-1" {
		t.Errorf("SubTasks[0].Id: got %q, want %q", detail.SubTasks[0].Id, "st-1")
	}
	if detail.SubTasks[1].DependsOn[0] != "st-1" {
		t.Errorf("SubTasks[1].DependsOn[0]: got %q, want %q", detail.SubTasks[1].DependsOn[0], "st-1")
	}
	if detail.SubTasks[0].Priority != 1 {
		t.Errorf("SubTasks[0].Priority: got %d, want %d", detail.SubTasks[0].Priority, 1)
	}

	// Verify TaskDAG.
	if detail.TaskDag == nil {
		t.Fatal("expected non-nil TaskDag")
	}
	if len(detail.TaskDag.Nodes) != 2 {
		t.Errorf("TaskDag.Nodes: got %d, want %d", len(detail.TaskDag.Nodes), 2)
	}
	if len(detail.TaskDag.RootIds) != 1 || detail.TaskDag.RootIds[0] != "st-1" {
		t.Errorf("TaskDag.RootIds: got %v, want [st-1]", detail.TaskDag.RootIds)
	}
	if len(detail.TaskDag.LeafIds) != 1 || detail.TaskDag.LeafIds[0] != "st-2" {
		t.Errorf("TaskDag.LeafIds: got %v, want [st-2]", detail.TaskDag.LeafIds)
	}

	// Verify MemoryHit.
	if detail.MemoryHit == nil {
		t.Fatal("expected non-nil MemoryHit")
	}
	if detail.MemoryHit.CacheId != "cache-001" {
		t.Errorf("MemoryHit.CacheId: got %q, want %q", detail.MemoryHit.CacheId, "cache-001")
	}
	if detail.MemoryHit.DqScore != 0.92 {
		t.Errorf("MemoryHit.DqScore: got %f, want %f", detail.MemoryHit.DqScore, 0.92)
	}
	if len(detail.MemoryHit.AgentKeysUsed) != 2 {
		t.Errorf("MemoryHit.AgentKeysUsed: got %d, want %d", len(detail.MemoryHit.AgentKeysUsed), 2)
	}
}

// TestToTaskPlanDetail_NilPlan verifies nil plan returns nil.
func TestToTaskPlanDetail_NilPlan(t *testing.T) {
	if detail := toTaskPlanDetail(nil); detail != nil {
		t.Errorf("expected nil for nil plan, got %+v", detail)
	}
}

// TestToTaskPlanDetail_NilOptionals verifies that nil TaskDAG and MemoryHit
// produce nil proto fields (not empty structs).
func TestToTaskPlanDetail_NilOptionals(t *testing.T) {
	plan := &biz.TaskPlan{
		ID:              "plan-003",
		SpiritSessionID: "sess-003",
		Strategy:        biz.StrategyDirect,
		Status:          biz.LegacyPlanStatusDraft,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	detail := toTaskPlanDetail(plan)
	if detail.TaskDag != nil {
		t.Errorf("expected nil TaskDag, got %+v", detail.TaskDag)
	}
	if detail.MemoryHit != nil {
		t.Errorf("expected nil MemoryHit, got %+v", detail.MemoryHit)
	}
	if len(detail.SubTasks) != 0 {
		t.Errorf("expected 0 SubTasks, got %d", len(detail.SubTasks))
	}
}

// TestToProtoSubTask verifies the SubTask conversion.
func TestToProtoSubTask(t *testing.T) {
	st := biz.SubTask{
		ID:                   "st-x",
		Name:                 "Test task",
		Description:          "A test",
		DependsOn:            []string{"st-a", "st-b"},
		RequiredCapabilities: []string{"cap-1"},
		Priority:             5,
		EstimatedComplexity:  0.7,
	}

	proto := toProtoSubTask(st)
	if proto.Id != "st-x" {
		t.Errorf("Id: got %q, want %q", proto.Id, "st-x")
	}
	if proto.Name != "Test task" {
		t.Errorf("Name: got %q, want %q", proto.Name, "Test task")
	}
	if len(proto.DependsOn) != 2 {
		t.Errorf("DependsOn: got %d, want %d", len(proto.DependsOn), 2)
	}
	if proto.Priority != 5 {
		t.Errorf("Priority: got %d, want %d", proto.Priority, 5)
	}
	if proto.EstimatedComplexity != 0.7 {
		t.Errorf("EstimatedComplexity: got %f, want %f", proto.EstimatedComplexity, 0.7)
	}
}

// TestToProtoPlanTaskDAG verifies the PlanTaskDAG conversion.
func TestToProtoPlanTaskDAG(t *testing.T) {
	dag := &biz.PlanTaskDAG{
		Nodes: []biz.SubTask{
			{ID: "n-1", Name: "Node 1"},
			{ID: "n-2", Name: "Node 2"},
		},
		RootIDs: []string{"n-1"},
		LeafIDs: []string{"n-2"},
	}

	proto := toProtoPlanTaskDAG(dag)
	if proto == nil {
		t.Fatal("expected non-nil proto")
	}
	if len(proto.Nodes) != 2 {
		t.Errorf("Nodes: got %d, want %d", len(proto.Nodes), 2)
	}
	if proto.Nodes[0].Id != "n-1" {
		t.Errorf("Nodes[0].Id: got %q, want %q", proto.Nodes[0].Id, "n-1")
	}
	if len(proto.RootIds) != 1 || proto.RootIds[0] != "n-1" {
		t.Errorf("RootIds: got %v, want [n-1]", proto.RootIds)
	}
}

// TestToProtoPlanTaskDAG_NilDAG verifies nil DAG returns nil.
func TestToProtoPlanTaskDAG_NilDAG(t *testing.T) {
	if proto := toProtoPlanTaskDAG(nil); proto != nil {
		t.Errorf("expected nil for nil DAG, got %+v", proto)
	}
}

// TestToTaskPlanSummary_StatusMapping verifies all LegacyPlanStatus values map correctly.
func TestToTaskPlanSummary_StatusMapping(t *testing.T) {
	tests := []struct {
		name   string
		status biz.LegacyPlanStatus
		want   string
	}{
		{"draft", biz.LegacyPlanStatusDraft, "draft"},
		{"approved", biz.LegacyPlanStatusApproved, "approved"},
		{"confirmed", biz.LegacyPlanStatusConfirmed, "confirmed"},
		{"executing", biz.LegacyPlanStatusExecuting, "executing"},
		{"completed", biz.LegacyPlanStatusCompleted, "completed"},
		{"failed", biz.LegacyPlanStatusFailed, "failed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &biz.TaskPlan{
				ID:        "p",
				Status:    tt.status,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			summary := toTaskPlanSummary(plan)
			if summary.Status != tt.want {
				t.Errorf("Status: got %q, want %q", summary.Status, tt.want)
			}
		})
	}
}

// TestToTaskPlanSummary_ComplexityMapping verifies all ComplexityLevel values map correctly.
func TestToTaskPlanSummary_ComplexityMapping(t *testing.T) {
	tests := []struct {
		name  string
		level biz.ComplexityLevel
		want  string
	}{
		{"simple", biz.ComplexitySimple, "simple"},
		{"moderate", biz.ComplexityModerate, "moderate"},
		{"complex", biz.ComplexityComplex, "complex"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &biz.TaskPlan{
				ID:              "p",
				ComplexityLevel: tt.level,
				CreatedAt:       time.Now(),
				UpdatedAt:       time.Now(),
			}
			summary := toTaskPlanSummary(plan)
			if summary.ComplexityLevel != tt.want {
				t.Errorf("ComplexityLevel: got %q, want %q", summary.ComplexityLevel, tt.want)
			}
		})
	}
}

// TestToTaskPlanSummary_StrategyMapping verifies all OrchestrationStrategy values map correctly.
func TestToTaskPlanSummary_StrategyMapping(t *testing.T) {
	tests := []struct {
		name     string
		strategy biz.OrchestrationStrategy
		want     string
	}{
		{"direct", biz.StrategyDirect, "direct"},
		{"single_agent", biz.StrategySingleAgent, "single_agent"},
		{"parallel", biz.StrategyParallel, "parallel"},
		{"dag", biz.StrategyDAG, "dag"},
		{"coordinator", biz.StrategyCoordinator, "coordinator"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			plan := &biz.TaskPlan{
				ID:        "p",
				Strategy:  tt.strategy,
				CreatedAt: time.Now(),
				UpdatedAt: time.Now(),
			}
			summary := toTaskPlanSummary(plan)
			if summary.Strategy != tt.want {
				t.Errorf("Strategy: got %q, want %q", summary.Strategy, tt.want)
			}
		})
	}
}
