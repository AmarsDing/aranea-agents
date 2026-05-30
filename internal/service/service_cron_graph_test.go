package service_test

import (
	"database/sql"
	"errors"
	"testing"
	"time"

	cronv1 "aranea-agents/api/kratos/cron/v1"
	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/internal/service"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestToProtoCronTask(t *testing.T) {
	in := biz.CronTask{
		ID: "ct1", TaskKey: "daily_report", Name: "Daily Report",
		Description: "runs daily", Status: "active", Enabled: true,
		SortOrder: 2, AgentID: "a1",
		ConfigJSON: `{"cron":"0 9 * * *"}`, MetadataJSON: `{"x":1}`,
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02", DeletedAt: "",
	}
	got := service.ToProtoCronTask(in)
	if got.Id != "ct1" {
		t.Errorf("Id = %q, want %q", got.Id, "ct1")
	}
	if got.TaskKey != "daily_report" {
		t.Errorf("TaskKey = %q, want %q", got.TaskKey, "daily_report")
	}
	if got.Name != "Daily Report" {
		t.Errorf("Name = %q, want %q", got.Name, "Daily Report")
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true")
	}
	if got.SortOrder != 2 {
		t.Errorf("SortOrder = %d, want 2", got.SortOrder)
	}
	if got.AgentId != "a1" {
		t.Errorf("AgentId = %q, want %q", got.AgentId, "a1")
	}
}

func TestPatchFromProtoCronTask(t *testing.T) {
	tests := []struct {
		name string
		in   *cronv1.CronTask
	}{
		{
			name: "full",
			in: &cronv1.CronTask{
				TaskKey: "tk", Name: "n", Description: "d", Status: "active",
				Enabled: true, SortOrder: 5, AgentId: "a1",
				ConfigJson: `{"c":1}`, MetadataJson: `{"m":2}`,
			},
		},
		{
			name: "nil",
			in:   nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.PatchFromProtoCronTask(tt.in)
			if tt.in == nil {
				if got.TaskKey != nil || got.Name != nil {
					t.Errorf("expected zero patch for nil input")
				}
				return
			}
			if got.TaskKey == nil || *got.TaskKey != "tk" {
				t.Errorf("TaskKey mismatch")
			}
			if got.Name == nil || *got.Name != "n" {
				t.Errorf("Name mismatch")
			}
			if got.Enabled == nil || !*got.Enabled {
				t.Errorf("Enabled mismatch")
			}
			if got.SortOrder == nil || *got.SortOrder != 5 {
				t.Errorf("SortOrder mismatch")
			}
			if got.AgentID == nil || *got.AgentID != "a1" {
				t.Errorf("AgentID mismatch")
			}
			if got.ConfigJSON == nil || *got.ConfigJSON != `{"c":1}` {
				t.Errorf("ConfigJSON mismatch")
			}
		})
	}
}

func TestToProtoCronTaskRun(t *testing.T) {
	in := biz.CronTaskRun{
		ID: "tr1", TaskID: "ct1", TaskName: "Daily",
		Status: "completed", StartedAt: "2024-01-01", FinishedAt: "2024-01-01",
		Trigger: "manual", RunID: "run1",
		OutputJSON: `{"ok":true}`, ErrorMessage: "",
		CreatedAt: "2024-01-01",
	}
	got := service.ToProtoCronTaskRun(in)
	if got.Id != "tr1" {
		t.Errorf("Id = %q, want %q", got.Id, "tr1")
	}
	if got.TaskId != "ct1" {
		t.Errorf("TaskId = %q, want %q", got.TaskId, "ct1")
	}
	if got.Trigger != "manual" {
		t.Errorf("Trigger = %q, want %q", got.Trigger, "manual")
	}
	if got.RunId != "run1" {
		t.Errorf("RunId = %q, want %q", got.RunId, "run1")
	}
}

func TestMapCronError(t *testing.T) {
	tests := []struct {
		name    string
		in      error
		want404 bool
		want503 bool
		want400 bool
		want409 bool
		wantNil bool
	}{
		{name: "nil", in: nil, wantNil: true},
		{name: "sql_no_rows", in: sql.ErrNoRows, want404: true},
		{name: "runner_disabled", in: biz.ErrCronRunnerDisabled, want503: true},
		{name: "task_deleted", in: biz.ErrCronTaskDeleted, want404: true},
		{name: "session_busy", in: biz.ErrCronSessionBusy, want409: true},
		{name: "kratos_error", in: kerrors.BadRequest("CRON", "bad"), want400: true},
		{name: "required_msg", in: errors.New("name is required"), want400: true},
		{name: "invalid_msg", in: errors.New("invalid config"), want400: true},
		{name: "generic_error", in: errors.New("something went wrong")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.MapCronError(tt.in)
			if tt.wantNil {
				if got != nil {
					t.Errorf("expected nil, got %v", got)
				}
				return
			}
			if got == nil {
				t.Fatalf("expected non-nil error")
			}
			var ke *kerrors.Error
			isKratos := errors.As(got, &ke)
			if tt.want404 {
				if !isKratos || ke.Code != 404 {
					t.Errorf("expected 404 kratos error, got %v", got)
				}
			}
			if tt.want503 {
				if !isKratos || ke.Code != 503 {
					t.Errorf("expected 503 kratos error, got %v", got)
				}
			}
			if tt.want400 {
				if !isKratos || ke.Code != 400 {
					t.Errorf("expected 400 kratos error, got %v", got)
				}
			}
			if tt.want409 {
				if !isKratos || ke.Code != 409 {
					t.Errorf("expected 409 kratos error, got %v", got)
				}
			}
		})
	}
}

func TestToProtoCronTask_RoundTrip(t *testing.T) {
	original := biz.CronTask{
		ID: "rt1", TaskKey: "rtk", Name: "rtn",
		Status: "active", Enabled: true, SortOrder: 3,
		AgentID: "a1", ConfigJSON: `{"c":1}`, MetadataJSON: `{"m":2}`,
	}
	pb := service.ToProtoCronTask(original)
	patch := service.PatchFromProtoCronTask(pb)
	if *patch.TaskKey != original.TaskKey {
		t.Errorf("roundtrip TaskKey = %q, want %q", *patch.TaskKey, original.TaskKey)
	}
	if *patch.Name != original.Name {
		t.Errorf("roundtrip Name = %q, want %q", *patch.Name, original.Name)
	}
	if *patch.Enabled != original.Enabled {
		t.Errorf("roundtrip Enabled = %v, want %v", *patch.Enabled, original.Enabled)
	}
	if *patch.SortOrder != original.SortOrder {
		t.Errorf("roundtrip SortOrder = %d, want %d", *patch.SortOrder, original.SortOrder)
	}
}

func TestFromProtoStateField(t *testing.T) {
	sf := &graphv1.StateFieldDef{
		Name: "count", Type: "int", Reducer: "add",
		Required: true, DisableDeepCopy: false,
	}
	got := service.FromProtoStateField(sf)
	if got.Name != "count" {
		t.Errorf("Name = %q, want %q", got.Name, "count")
	}
	if got.Type != "int" {
		t.Errorf("Type = %q, want %q", got.Type, "int")
	}
	if got.Reducer != biz.ReducerType("add") {
		t.Errorf("Reducer = %v, want add", got.Reducer)
	}
	if !got.Required {
		t.Errorf("Required = false, want true")
	}
}

func TestFromProtoNode(t *testing.T) {
	n := &graphv1.NodeDef{
		Id: "node1", FuncRef: "func1", Type: "llm",
		Description: "test node", Instruction: "do stuff",
		ModelName: "gpt-4", ToolNames: []string{"tool1"},
		AgentName: "agent1", InterruptBefore: true, InterruptAfter: false,
		Destinations: []string{"node2"},
		RequiredRole: "admin", AssignmentMode: "manual",
		AssignmentStrategy: "round_robin", ReviewerAgent: "reviewer1",
		ReviewRules: "must approve", TimeoutSeconds: 60,
		HeartbeatIntervalSeconds: 10, EnableLeaseExtension: true,
		RetryMaxAttempts: 3, FailureAction: "fallback",
		FallbackAgent: "fallback1",
		InputMapperJson: `{"in":"x"}`, OutputMapperJson: `{"out":"y"}`,
		IsolatedMessages: true, InputFromLastResponse: false,
		CacheEnabled: true, CacheTtlSeconds: 300,
	}
	got := service.FromProtoNode(n)
	if got.ID != "node1" {
		t.Errorf("ID = %q, want %q", got.ID, "node1")
	}
	if got.FuncRef != "func1" {
		t.Errorf("FuncRef = %q, want %q", got.FuncRef, "func1")
	}
	if got.Type != "llm" {
		t.Errorf("Type = %q, want %q", got.Type, "llm")
	}
	if got.TimeoutSeconds != 60 {
		t.Errorf("TimeoutSeconds = %d, want 60", got.TimeoutSeconds)
	}
	if got.RetryMaxAttempts != 3 {
		t.Errorf("RetryMaxAttempts = %d, want 3", got.RetryMaxAttempts)
	}
	if !got.CacheEnabled {
		t.Errorf("CacheEnabled = false, want true")
	}
	if got.CacheTTLSeconds != 300 {
		t.Errorf("CacheTTLSeconds = %d, want 300", got.CacheTTLSeconds)
	}
}

func TestFromProtoCondEdge(t *testing.T) {
	ce := &graphv1.ConditionalEdgeDef{
		From: "n1", CondFuncRef: "router1",
		PathMap: map[string]string{"yes": "n2", "no": "n3"},
	}
	got := service.FromProtoCondEdge(ce)
	if got.From != "n1" {
		t.Errorf("From = %q, want %q", got.From, "n1")
	}
	if got.CondFuncRef != "router1" {
		t.Errorf("CondFuncRef = %q, want %q", got.CondFuncRef, "router1")
	}
	if got.PathMap["yes"] != "n2" {
		t.Errorf("PathMap[yes] = %q, want %q", got.PathMap["yes"], "n2")
	}
}

func TestFromProtoSubgraph(t *testing.T) {
	sub := &graphv1.SubgraphDef{
		Id: "sub1", InterruptBefore: true, InterruptAfter: false,
	}
	got := service.FromProtoSubgraph(sub)
	if got.ID != "sub1" {
		t.Errorf("ID = %q, want %q", got.ID, "sub1")
	}
	if !got.InterruptBefore {
		t.Errorf("InterruptBefore = false, want true")
	}
}

func TestToProtoGraph(t *testing.T) {
	now := time.Now()
	def := &biz.GraphDefinition{
		ID: "g1", Name: "test_graph", Description: "desc",
		EntryPoint: "start", FinishPoint: "end",
		EnableCheckpoint: true, ExecutionEngine: "trpc",
		InterruptBefore: []string{"n1"}, InterruptAfter: []string{"n2"},
		SortOrder: 1, CreatedAt: now, UpdatedAt: now,
		StateFields: []biz.StateFieldDef{
			{Name: "count", Type: "int", Reducer: "add"},
		},
		Nodes: []biz.NodeDef{
			{ID: "n1", Type: "llm", FuncRef: "func1"},
		},
		Edges: []biz.EdgeDef{
			{From: "n1", To: "n2"},
		},
		ConditionalEdges: []biz.ConditionalEdgeDef{
			{From: "n1", CondFuncRef: "router", PathMap: map[string]string{"ok": "n2"}},
		},
		Subgraphs: []biz.SubgraphDef{
			{ID: "sub1"},
		},
	}
	got, err := service.ToProtoGraph(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Id != "g1" {
		t.Errorf("Id = %q, want %q", got.Id, "g1")
	}
	if got.Name != "test_graph" {
		t.Errorf("Name = %q, want %q", got.Name, "test_graph")
	}
	if got.EntryPoint != "start" {
		t.Errorf("EntryPoint = %q, want %q", got.EntryPoint, "start")
	}
	if !got.EnableCheckpoint {
		t.Errorf("EnableCheckpoint = false, want true")
	}
	if len(got.StateFields) != 1 {
		t.Errorf("len(StateFields) = %d, want 1", len(got.StateFields))
	}
	if len(got.Nodes) != 1 {
		t.Errorf("len(Nodes) = %d, want 1", len(got.Nodes))
	}
	if len(got.Edges) != 1 {
		t.Errorf("len(Edges) = %d, want 1", len(got.Edges))
	}
	if len(got.ConditionalEdges) != 1 {
		t.Errorf("len(ConditionalEdges) = %d, want 1", len(got.ConditionalEdges))
	}
	if len(got.Subgraphs) != 1 {
		t.Errorf("len(Subgraphs) = %d, want 1", len(got.Subgraphs))
	}
}

func TestToProtoGraph_Minimal(t *testing.T) {
	now := time.Now()
	def := &biz.GraphDefinition{
		ID: "g2", Name: "minimal",
		CreatedAt: now, UpdatedAt: now,
	}
	got, err := service.ToProtoGraph(def)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Id != "g2" {
		t.Errorf("Id = %q, want %q", got.Id, "g2")
	}
	if len(got.StateFields) != 0 {
		t.Errorf("expected no StateFields")
	}
	if len(got.Nodes) != 0 {
		t.Errorf("expected no Nodes")
	}
}

func TestToProtoStateField(t *testing.T) {
	sf := biz.StateFieldDef{
		Name: "items", Type: "list", Reducer: "append",
		Required: true, DisableDeepCopy: true,
	}
	got := service.ToProtoStateField(sf)
	if got.Name != "items" {
		t.Errorf("Name = %q, want %q", got.Name, "items")
	}
	if got.Type != "list" {
		t.Errorf("Type = %q, want %q", got.Type, "list")
	}
	if got.Reducer != "append" {
		t.Errorf("Reducer = %q, want %q", got.Reducer, "append")
	}
	if !got.Required {
		t.Errorf("Required = false, want true")
	}
	if !got.DisableDeepCopy {
		t.Errorf("DisableDeepCopy = false, want true")
	}
}

func TestToProtoStep(t *testing.T) {
	step := biz.GraphStepSnapshot{
		NodeID: "n1", StepIndex: 2,
		InputState:  map[string]any{"x": 1},
		OutputState: map[string]any{"y": 2},
	}
	got := service.ToProtoStep(step)
	if got.NodeId != "n1" {
		t.Errorf("NodeId = %q, want %q", got.NodeId, "n1")
	}
	if got.StepIndex != 2 {
		t.Errorf("StepIndex = %d, want 2", got.StepIndex)
	}
	if got.InputState == nil {
		t.Errorf("InputState should not be nil")
	}
	if got.OutputState == nil {
		t.Errorf("OutputState should not be nil")
	}
}

func TestUserTemplateToProto(t *testing.T) {
	meta := &biz.UserTemplateMeta{
		TemplateID: "tmpl1", Name: "My Template",
		Category: "workflow", Description: "desc",
	}
	def := &biz.GraphDefinition{
		EntryPoint: "start", FinishPoint: "end",
		Nodes: []biz.NodeDef{
			{ID: "n1", Type: "llm", Description: "node1"},
		},
		Edges: []biz.EdgeDef{
			{From: "n1", To: "n2"},
		},
		StateFields: []biz.StateFieldDef{
			{Name: "state", Type: "string"},
		},
	}
	got := service.UserTemplateToProto(def, meta)
	if got.Id != "tmpl1" {
		t.Errorf("Id = %q, want %q", got.Id, "tmpl1")
	}
	if got.Name != "My Template" {
		t.Errorf("Name = %q, want %q", got.Name, "My Template")
	}
	if got.Category != "workflow" {
		t.Errorf("Category = %q, want %q", got.Category, "workflow")
	}
	if got.EntryPoint != "start" {
		t.Errorf("EntryPoint = %q, want %q", got.EntryPoint, "start")
	}
	if len(got.Nodes) != 1 {
		t.Errorf("len(Nodes) = %d, want 1", len(got.Nodes))
	}
	if len(got.Edges) != 1 {
		t.Errorf("len(Edges) = %d, want 1", len(got.Edges))
	}
	if len(got.StateFields) != 1 {
		t.Errorf("len(StateFields) = %d, want 1", len(got.StateFields))
	}
}

func TestTemplateToProto(t *testing.T) {
	tmpl := graphtrpc.GraphTemplate{
		ID: "builtin1", Name: "Built-in", Description: "builtin template",
		Category: "standard", EntryPoint: "start", FinishPoint: "end",
		Nodes: []graphtrpc.TemplateNode{
			{NodeID: "n1", Type: "llm", Label: "LLM Node", Description: "desc"},
		},
		Edges: []graphtrpc.TemplateEdge{
			{FromNode: "n1", ToNode: "n2", Type: "edge", Label: "next"},
		},
		StateFields: []graphtrpc.StateFieldDef{
			{Name: "state", Type: "string", Reducer: graphtrpc.ReducerType("last"), Required: true},
		},
	}
	got := service.TemplateToProto(tmpl)
	if got.Id != "builtin1" {
		t.Errorf("Id = %q, want %q", got.Id, "builtin1")
	}
	if got.Name != "Built-in" {
		t.Errorf("Name = %q, want %q", got.Name, "Built-in")
	}
	if len(got.Nodes) != 1 {
		t.Errorf("len(Nodes) = %d, want 1", len(got.Nodes))
	}
	if len(got.Edges) != 1 {
		t.Errorf("len(Edges) = %d, want 1", len(got.Edges))
	}
	if len(got.StateFields) != 1 {
		t.Errorf("len(StateFields) = %d, want 1", len(got.StateFields))
	}
	if got.StateFields[0].Name != "state" {
		t.Errorf("StateFields[0].Name = %q, want %q", got.StateFields[0].Name, "state")
	}
}

func TestToProtoTask(t *testing.T) {
	now := time.Now()
	claimedAt := now.Add(1 * time.Hour)
	completedAt := now.Add(2 * time.Hour)
	task := &biz.GraphTask{
		TaskID: "task1", NodeID: "n1", ExecutionID: "exec1",
		Assignee: "agent1", Status: biz.TaskStatusComplete,
		Context: "ctx", Input: "in", Output: "out",
		Summary: "done", Metadata: "meta",
		RequiredRole: "admin", AssignmentMode: "auto",
		CreatedAt: now, ClaimedAt: &claimedAt, CompletedAt: &completedAt,
	}
	got := service.ToProtoTask(task)
	if got.TaskId != "task1" {
		t.Errorf("TaskId = %q, want %q", got.TaskId, "task1")
	}
	if got.NodeId != "n1" {
		t.Errorf("NodeId = %q, want %q", got.NodeId, "n1")
	}
	if got.Assignee != "agent1" {
		t.Errorf("Assignee = %q, want %q", got.Assignee, "agent1")
	}
	if got.ClaimedAt == nil {
		t.Errorf("ClaimedAt should not be nil")
	}
	if got.CompletedAt == nil {
		t.Errorf("CompletedAt should not be nil")
	}
}

func TestToProtoTask_NilTimes(t *testing.T) {
	now := time.Now()
	task := &biz.GraphTask{
		TaskID: "task2", NodeID: "n1",
		Status: biz.TaskStatusPending,
		CreatedAt: now,
	}
	got := service.ToProtoTask(task)
	if got.ClaimedAt != nil {
		t.Errorf("ClaimedAt should be nil")
	}
	if got.CompletedAt != nil {
		t.Errorf("CompletedAt should be nil")
	}
}

func TestBizTaskStatusToProto(t *testing.T) {
	tests := []struct {
		name   string
		status biz.TaskStatus
		want   graphv1.TaskStatus
	}{
		{"pending", biz.TaskStatusPending, graphv1.TaskStatus_TASK_PENDING},
		{"claimed", biz.TaskStatusClaimed, graphv1.TaskStatus_TASK_CLAIMED},
		{"complete", biz.TaskStatusComplete, graphv1.TaskStatus_TASK_COMPLETE},
		{"blocked", biz.TaskStatusBlocked, graphv1.TaskStatus_TASK_BLOCKED},
		{"review_required", biz.TaskStatusReviewRequired, graphv1.TaskStatus_TASK_REVIEW_REQUIRED},
		{"failed", biz.TaskStatusFailed, graphv1.TaskStatus_TASK_FAILED},
		{"timed_out", biz.TaskStatusTimedOut, graphv1.TaskStatus_TASK_TIMED_OUT},
		{"cancelled", biz.TaskStatusCancelled, graphv1.TaskStatus_TASK_CANCELLED},
		{"crashed", biz.TaskStatusCrashed, graphv1.TaskStatus_TASK_CRASHED},
		{"pending_assignment", biz.TaskStatusPendingAssignment, graphv1.TaskStatus_TASK_PENDING_ASSIGNMENT},
		{"unknown", biz.TaskStatus("unknown"), graphv1.TaskStatus_TASK_PENDING},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.BizTaskStatusToProto(tt.status)
			if got != tt.want {
				t.Errorf("BizTaskStatusToProto(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}
