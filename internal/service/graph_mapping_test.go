package service

import (
	"testing"
	"time"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"
	graphtrpc "aranea-agents/internal/graph/trpc"
	"aranea-agents/pkg/loggateway"

	"google.golang.org/protobuf/types/known/structpb"
)

func TestBizTaskStatusToProto(t *testing.T) {
	tests := []struct {
		name   string
		status biz.GraphTaskStatus
		want   graphv1.TaskStatus
	}{
		{"pending", biz.GraphTaskStatusPending, graphv1.TaskStatus_TASK_PENDING},
		{"claimed", biz.GraphTaskStatusClaimed, graphv1.TaskStatus_TASK_CLAIMED},
		{"complete", biz.GraphTaskStatusComplete, graphv1.TaskStatus_TASK_COMPLETE},
		{"blocked", biz.GraphTaskStatusBlocked, graphv1.TaskStatus_TASK_BLOCKED},
		{"review_required", biz.GraphTaskStatusReviewRequired, graphv1.TaskStatus_TASK_REVIEW_REQUIRED},
		{"failed", biz.GraphTaskStatusFailed, graphv1.TaskStatus_TASK_FAILED},
		{"timed_out", biz.GraphTaskStatusTimedOut, graphv1.TaskStatus_TASK_TIMED_OUT},
		{"cancelled", biz.GraphTaskStatusCancelled, graphv1.TaskStatus_TASK_CANCELLED},
		{"crashed", biz.GraphTaskStatusCrashed, graphv1.TaskStatus_TASK_CRASHED},
		{"pending_assignment", biz.GraphTaskStatusPendingAssignment, graphv1.TaskStatus_TASK_PENDING_ASSIGNMENT},
		{"unknown", biz.GraphTaskStatus("unknown"), graphv1.TaskStatus_TASK_PENDING},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := bizTaskStatusToProto(tt.status)
			if got != tt.want {
				t.Errorf("bizTaskStatusToProto(%q) = %v, want %v", tt.status, got, tt.want)
			}
		})
	}
}

func TestFromProtoStateField(t *testing.T) {
	t.Run("without_default", func(t *testing.T) {
		sf := &graphv1.StateFieldDef{
			Name:     "count",
			Type:     "int",
			Reducer:  "default",
			Required: true,
		}
		got := fromProtoStateField(sf)
		if got.Name != "count" {
			t.Errorf("Name = %q", got.Name)
		}
		if got.Type != "int" {
			t.Errorf("Type = %q", got.Type)
		}
		if got.Reducer != biz.ReducerDefault {
			t.Errorf("Reducer = %q", got.Reducer)
		}
		if !got.Required {
			t.Error("Required = false")
		}
		if got.DefaultValue != nil {
			t.Errorf("DefaultValue = %v, want nil", got.DefaultValue)
		}
	})

	t.Run("with_default", func(t *testing.T) {
		dv := structpb.NewStringValue("hello")
		sf := &graphv1.StateFieldDef{
			Name:         "msg",
			Type:         "string",
			Reducer:      "cover",
			DefaultValue: dv,
		}
		got := fromProtoStateField(sf)
		if got.DefaultValue == nil {
			t.Fatal("DefaultValue = nil, want non-nil")
		}
		if got.DefaultValue != "hello" {
			t.Errorf("DefaultValue = %v, want hello", got.DefaultValue)
		}
		if got.Reducer != biz.ReducerCover {
			t.Errorf("Reducer = %q, want cover", got.Reducer)
		}
	})
}

func TestFromProtoNode(t *testing.T) {
	in := &graphv1.NodeDef{
		Id:                    "node-1",
		FuncRef:               "fn1",
		Type:                  "llm",
		Description:           "test node",
		Instruction:           "say hi",
		ModelName:             "gpt-4o",
		ToolNames:             []string{"search"},
		AgentName:             "helper",
		InterruptBefore:       true,
		InterruptAfter:        false,
		Destinations:          []string{"next"},
		RetryMaxAttempts:      3,
		FailureAction:         "fallback",
		FallbackAgent:         "backup",
		InputMapperJson:       `{"x":"y"}`,
		OutputMapperJson:      `{"a":"b"}`,
		IsolatedMessages:      true,
		InputFromLastResponse: false,
		CacheEnabled:          true,
		CacheTtlSeconds:       300,
	}
	got := fromProtoNode(in)
	if got.ID != "node-1" {
		t.Errorf("ID = %q", got.ID)
	}
	if got.FuncRef != "fn1" {
		t.Errorf("FuncRef = %q", got.FuncRef)
	}
	if got.Type != "llm" {
		t.Errorf("Type = %q", got.Type)
	}
	if got.ModelName != "gpt-4o" {
		t.Errorf("ModelName = %q", got.ModelName)
	}
	if len(got.ToolNames) != 1 || got.ToolNames[0] != "search" {
		t.Errorf("ToolNames = %v", got.ToolNames)
	}
	if got.RetryMaxAttempts != 3 {
		t.Errorf("RetryMaxAttempts = %d", got.RetryMaxAttempts)
	}
	if !got.InterruptBefore {
		t.Error("InterruptBefore = false")
	}
	if !got.CacheEnabled {
		t.Error("CacheEnabled = false")
	}
	if got.CacheTTLSeconds != 300 {
		t.Errorf("CacheTTLSeconds = %d", got.CacheTTLSeconds)
	}
}

func TestFromProtoCondEdge(t *testing.T) {
	in := &graphv1.ConditionalEdgeDef{
		From:        "router",
		CondFuncRef: "route_fn",
		PathMap:     map[string]string{"yes": "step_a", "no": "step_b"},
	}
	got := fromProtoCondEdge(in)
	if got.From != "router" {
		t.Errorf("From = %q", got.From)
	}
	if got.CondFuncRef != "route_fn" {
		t.Errorf("CondFuncRef = %q", got.CondFuncRef)
	}
	if got.PathMap["yes"] != "step_a" {
		t.Errorf("PathMap[yes] = %q", got.PathMap["yes"])
	}
}

func TestFromProtoSubgraph(t *testing.T) {
	in := &graphv1.SubgraphDef{
		Id:              "sub-1",
		InterruptBefore: true,
		InterruptAfter:  false,
	}
	got := fromProtoSubgraph(in)
	if got.ID != "sub-1" {
		t.Errorf("ID = %q", got.ID)
	}
	if !got.InterruptBefore {
		t.Error("InterruptBefore = false")
	}
}

func TestToProtoStateField(t *testing.T) {
	t.Run("without_default", func(t *testing.T) {
		sf := biz.StateFieldDef{
			Name:     "count",
			Type:     "int",
			Reducer:  biz.ReducerDefault,
			Required: true,
		}
		got := toProtoStateField(sf, loggateway.NewNoop())
		if got.Name != "count" {
			t.Errorf("Name = %q", got.Name)
		}
		if got.Reducer != "default" {
			t.Errorf("Reducer = %q", got.Reducer)
		}
		if got.DefaultValue != nil {
			t.Error("DefaultValue should be nil")
		}
	})

	t.Run("with_default", func(t *testing.T) {
		sf := biz.StateFieldDef{
			Name:         "msg",
			Type:         "string",
			Reducer:      biz.ReducerAppend,
			DefaultValue: "hello",
		}
		got := toProtoStateField(sf, loggateway.NewNoop())
		if got.DefaultValue == nil {
			t.Fatal("DefaultValue should not be nil")
		}
		if got.Reducer != "append" {
			t.Errorf("Reducer = %q", got.Reducer)
		}
	})
}

func TestToProtoGraph(t *testing.T) {
	now := time.Now()
	def := &biz.GraphDefinition{
		ID:               "g1",
		Name:             "demo",
		Description:      "test graph",
		EntryPoint:       "start",
		FinishPoint:      "end",
		EnableCheckpoint: true,
		ExecutionEngine:  biz.EngineDAG,
		InterruptBefore:  []string{"start"},
		InterruptAfter:   []string{"end"},
		Version:          3,
		CreatedAt:        now,
		UpdatedAt:        now,
		Nodes: []biz.NodeDef{
			{ID: "start", Type: "llm", ModelName: "gpt-4o"},
			{ID: "end", Type: "llm"},
		},
		Edges: []biz.EdgeDef{
			{From: "start", To: "end"},
		},
		ConditionalEdges: []biz.ConditionalEdgeDef{
			{From: "start", CondFuncRef: "route_fn", PathMap: map[string]string{"yes": "end"}},
		},
		Subgraphs: []biz.SubgraphDef{
			{ID: "sub-1", InterruptBefore: true},
		},
		StateFields: []biz.StateFieldDef{
			{Name: "messages", Type: "[]any", Reducer: biz.ReducerAppend},
		},
	}

	pb, err := toProtoGraph(def, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("toProtoGraph error: %v", err)
	}
	if pb.Id != "g1" {
		t.Errorf("Id = %q", pb.Id)
	}
	if pb.Name != "demo" {
		t.Errorf("Name = %q", pb.Name)
	}
	if pb.EntryPoint != "start" {
		t.Errorf("EntryPoint = %q", pb.EntryPoint)
	}
	if pb.Version != 3 {
		t.Errorf("Version = %d", pb.Version)
	}
	if !pb.EnableCheckpoint {
		t.Error("EnableCheckpoint = false")
	}
	if pb.ExecutionEngine != "dag" {
		t.Errorf("ExecutionEngine = %q", pb.ExecutionEngine)
	}
	if len(pb.Nodes) != 2 {
		t.Fatalf("Nodes len = %d", len(pb.Nodes))
	}
	if pb.Nodes[0].Id != "start" {
		t.Errorf("Node[0].Id = %q", pb.Nodes[0].Id)
	}
	if pb.Nodes[0].ModelName != "gpt-4o" {
		t.Errorf("Node[0].ModelName = %q", pb.Nodes[0].ModelName)
	}
	if len(pb.Edges) != 1 {
		t.Fatalf("Edges len = %d", len(pb.Edges))
	}
	if pb.Edges[0].From != "start" || pb.Edges[0].To != "end" {
		t.Errorf("Edge = %+v", pb.Edges[0])
	}
	if len(pb.ConditionalEdges) != 1 {
		t.Fatalf("ConditionalEdges len = %d", len(pb.ConditionalEdges))
	}
	if pb.ConditionalEdges[0].CondFuncRef != "route_fn" {
		t.Errorf("CondEdge CondFuncRef = %q", pb.ConditionalEdges[0].CondFuncRef)
	}
	if len(pb.Subgraphs) != 1 {
		t.Fatalf("Subgraphs len = %d", len(pb.Subgraphs))
	}
	if pb.Subgraphs[0].Id != "sub-1" {
		t.Errorf("Subgraph Id = %q", pb.Subgraphs[0].Id)
	}
	if len(pb.StateFields) != 1 {
		t.Fatalf("StateFields len = %d", len(pb.StateFields))
	}
	if pb.StateFields[0].Name != "messages" {
		t.Errorf("StateField Name = %q", pb.StateFields[0].Name)
	}
}

func TestToProtoGraph_WithMetadata(t *testing.T) {
	def := &biz.GraphDefinition{
		ID:          "g1",
		Name:        "meta",
		EntryPoint:  "start",
		FinishPoint: "end",
		Metadata:    map[string]any{"key": "value"},
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	pb, err := toProtoGraph(def, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("toProtoGraph error: %v", err)
	}
	if pb.Metadata == nil {
		t.Fatal("Metadata should not be nil")
	}
}

func TestToProtoGraph_NilSlices(t *testing.T) {
	def := &biz.GraphDefinition{
		ID:          "g1",
		Name:        "empty",
		EntryPoint:  "start",
		FinishPoint: "end",
		CreatedAt:   time.Now(),
		UpdatedAt:   time.Now(),
	}
	pb, err := toProtoGraph(def, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("toProtoGraph error: %v", err)
	}
	if pb.Id != "g1" {
		t.Errorf("Id = %q", pb.Id)
	}
}

func TestToProtoStep(t *testing.T) {
	t.Run("without_state", func(t *testing.T) {
		step := biz.GraphStepSnapshot{
			NodeID:    "n1",
			StepIndex: 0,
		}
		got := toProtoStep(step)
		if got.NodeId != "n1" {
			t.Errorf("NodeId = %q", got.NodeId)
		}
		if got.StepIndex != 0 {
			t.Errorf("StepIndex = %d", got.StepIndex)
		}
		if got.InputState != nil {
			t.Error("InputState should be nil")
		}
		if got.OutputState != nil {
			t.Error("OutputState should be nil")
		}
	})

	t.Run("with_state", func(t *testing.T) {
		step := biz.GraphStepSnapshot{
			NodeID:      "n1",
			StepIndex:   1,
			InputState:  map[string]any{"x": 1},
			OutputState: map[string]any{"y": 2},
		}
		got := toProtoStep(step)
		if got.NodeId != "n1" {
			t.Errorf("NodeId = %q", got.NodeId)
		}
		if got.StepIndex != 1 {
			t.Errorf("StepIndex = %d", got.StepIndex)
		}
		if got.InputState == nil {
			t.Error("InputState should not be nil")
		}
		if got.OutputState == nil {
			t.Error("OutputState should not be nil")
		}
	})
}

func TestUserTemplateToProto(t *testing.T) {
	def := &biz.GraphDefinition{
		EntryPoint:  "start",
		FinishPoint: "end",
		Nodes: []biz.NodeDef{
			{ID: "start", Type: "llm", Description: "begin"},
			{ID: "end", Type: "llm", Description: "finish"},
		},
		Edges: []biz.EdgeDef{
			{From: "start", To: "end"},
		},
		StateFields: []biz.StateFieldDef{
			{Name: "messages", Type: "[]any", Reducer: biz.ReducerAppend},
		},
	}
	meta := &biz.UserTemplateMeta{
		TemplateID:  "tmpl-1",
		Name:        "My Template",
		Description: "A test template",
		Category:    "pipeline",
	}

	got := userTemplateToProto(def, meta, loggateway.NewNoop())
	if got.Id != "tmpl-1" {
		t.Errorf("Id = %q", got.Id)
	}
	if got.Name != "My Template" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.Description != "A test template" {
		t.Errorf("Description = %q", got.Description)
	}
	if got.Category != "pipeline" {
		t.Errorf("Category = %q", got.Category)
	}
	if got.EntryPoint != "start" {
		t.Errorf("EntryPoint = %q", got.EntryPoint)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("Nodes len = %d", len(got.Nodes))
	}
	if got.Nodes[0].NodeId != "start" {
		t.Errorf("Node[0].NodeId = %q", got.Nodes[0].NodeId)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("Edges len = %d", len(got.Edges))
	}
	if got.Edges[0].FromNode != "start" || got.Edges[0].ToNode != "end" {
		t.Errorf("Edge = %+v", got.Edges[0])
	}
	if len(got.StateFields) != 1 {
		t.Fatalf("StateFields len = %d", len(got.StateFields))
	}
}

func TestTemplateToProto(t *testing.T) {
	tmpl := graphtrpc.GraphTemplate{
		ID:          "builtin-1",
		Name:        "Pipeline",
		Description: "A pipeline template",
		Category:    "pipeline",
		EntryPoint:  "step_1",
		FinishPoint: "step_3",
		Nodes: []graphtrpc.TemplateNode{
			{NodeID: "step_1", Type: "function", Label: "Step 1", Description: "first"},
			{NodeID: "step_3", Type: "function", Label: "Step 3", Description: "last"},
		},
		Edges: []graphtrpc.TemplateEdge{
			{FromNode: "step_1", ToNode: "step_3", Type: "runtime", Label: "next"},
		},
		StateFields: []graphtrpc.StateFieldDef{
			{Name: "input", Type: "string", Reducer: graphtrpc.ReducerDefault},
		},
	}

	got := templateToProto(tmpl, loggateway.NewNoop())
	if got.Id != "builtin-1" {
		t.Errorf("Id = %q", got.Id)
	}
	if got.Name != "Pipeline" {
		t.Errorf("Name = %q", got.Name)
	}
	if got.EntryPoint != "step_1" {
		t.Errorf("EntryPoint = %q", got.EntryPoint)
	}
	if len(got.Nodes) != 2 {
		t.Fatalf("Nodes len = %d", len(got.Nodes))
	}
	if got.Nodes[0].NodeId != "step_1" {
		t.Errorf("Node[0].NodeId = %q", got.Nodes[0].NodeId)
	}
	if got.Nodes[0].Label != "Step 1" {
		t.Errorf("Node[0].Label = %q", got.Nodes[0].Label)
	}
	if len(got.Edges) != 1 {
		t.Fatalf("Edges len = %d", len(got.Edges))
	}
	if got.Edges[0].FromNode != "step_1" {
		t.Errorf("Edge FromNode = %q", got.Edges[0].FromNode)
	}
	if got.Edges[0].Label != "next" {
		t.Errorf("Edge Label = %q", got.Edges[0].Label)
	}
	if len(got.StateFields) != 1 {
		t.Fatalf("StateFields len = %d", len(got.StateFields))
	}
	if got.StateFields[0].Name != "input" {
		t.Errorf("StateField Name = %q", got.StateFields[0].Name)
	}
}

func TestToProtoTask(t *testing.T) {
	now := time.Now()
	claimedAt := now.Add(-5 * time.Minute)
	completedAt := now

	task := &biz.GraphTask{
		TaskID:         "task-1",
		NodeID:         "node-1",
		ExecutionID:    "exec-1",
		Assignee:       "user-1",
		Status:         biz.GraphTaskStatusComplete,
		Context:        "do something",
		Input:          "input data",
		Output:         "output data",
		Summary:        "done",
		Metadata:       "meta",
		RequiredRole:   "admin",
		AssignmentMode: "static",
		CreatedAt:      now,
		ClaimedAt:      &claimedAt,
		CompletedAt:    &completedAt,
	}

	got := toProtoTask(task)
	if got.TaskId != "task-1" {
		t.Errorf("TaskId = %q", got.TaskId)
	}
	if got.NodeId != "node-1" {
		t.Errorf("NodeId = %q", got.NodeId)
	}
	if got.ExecutionId != "exec-1" {
		t.Errorf("ExecutionId = %q", got.ExecutionId)
	}
	if got.Assignee != "user-1" {
		t.Errorf("Assignee = %q", got.Assignee)
	}
	if got.Status != graphv1.TaskStatus_TASK_COMPLETE {
		t.Errorf("Status = %v", got.Status)
	}
	if got.Context != "do something" {
		t.Errorf("Context = %q", got.Context)
	}
	if got.ClaimedAt == nil {
		t.Error("ClaimedAt should not be nil")
	}
	if got.CompletedAt == nil {
		t.Error("CompletedAt should not be nil")
	}
}

func TestToProtoTask_NilTimestamps(t *testing.T) {
	task := &biz.GraphTask{
		TaskID:    "task-2",
		NodeID:    "n1",
		Status:    biz.GraphTaskStatusPending,
		CreatedAt: time.Now(),
	}

	got := toProtoTask(task)
	if got.TaskId != "task-2" {
		t.Errorf("TaskId = %q", got.TaskId)
	}
	if got.ClaimedAt != nil {
		t.Error("ClaimedAt should be nil")
	}
	if got.CompletedAt != nil {
		t.Error("CompletedAt should be nil")
	}
	if got.Status != graphv1.TaskStatus_TASK_PENDING {
		t.Errorf("Status = %v", got.Status)
	}
}
