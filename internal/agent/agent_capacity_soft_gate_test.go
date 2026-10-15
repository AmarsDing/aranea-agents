package agent

import (
	"context"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestAgentLoadTracker_AcquireRelease(t *testing.T) {
	tr := NewAgentLoadTracker()
	tr.Acquire("a", "a", "b")
	if tr.Load("a") != 2 || tr.Load("b") != 1 {
		t.Fatalf("loads a=%d b=%d", tr.Load("a"), tr.Load("b"))
	}
	if !tr.AtCapacity("a", 2) {
		t.Fatal("expected at capacity")
	}
	if tr.AtCapacity("b", 2) {
		t.Fatal("b should have spare capacity")
	}
	tr.Release("a", "b")
	if tr.Load("a") != 1 || tr.Load("b") != 0 {
		t.Fatalf("after release a=%d b=%d", tr.Load("a"), tr.Load("b"))
	}
}

func TestApplyCapacitySoftGate_ReassignOrAnnotate(t *testing.T) {
	prev := GlobalAgentLoadTracker
	GlobalAgentLoadTracker = NewAgentLoadTracker()
	t.Cleanup(func() { GlobalAgentLoadTracker = prev })

	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "writer_a", DisplayName: "A", Status: "active", DomainPath: "content/write", PositionKey: "writer_a", ConfigJSON: `{"capacity":1}`},
		{AgentKey: "writer_b", DisplayName: "B", Status: "active", DomainPath: "content/write", PositionKey: "writer_b", ConfigJSON: `{"capacity":1}`},
	}}
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:          loggateway.NewNoop(),
	}
	caps, err := impl.capBuilder.BuildAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assignable := filterHeuristicAssignable(caps)
	if len(assignable) < 2 {
		t.Fatalf("assignable=%d", len(assignable))
	}
	for _, c := range assignable {
		if c.Capacity != 1 {
			t.Fatalf("cap %s=%d", c.AgentKey, c.Capacity)
		}
	}

	GlobalAgentLoadTracker.Acquire("writer_a")

	plan := &biz.TaskPlan{
		ID: "tp-cap",
		SubTasks: []biz.SubTask{
			{ID: "st1", Name: "draft", DomainPath: "content/write"},
		},
	}
	allocs := []biz.TaskAllocation{
		{SubTaskID: "st1", AssignedKey: "writer_a", AssignedType: "agent", MatchReason: "exact"},
	}
	out := impl.applyCapacitySoftGate(context.Background(), plan, allocs, assignable, "trace")
	got := out[0]
	if got.AssignedKey == "writer_a" {
		if !strings.Contains(got.MatchReason, "capacity_soft") {
			t.Fatalf("kept busy lead without annotation: %+v", got)
		}
		return
	}
	if got.AssignedKey != "writer_b" {
		t.Fatalf("AssignedKey=%q", got.AssignedKey)
	}
	if !strings.Contains(got.MatchReason, "capacity_soft_reassign") {
		t.Fatalf("missing capacity_soft_reassign in %q", got.MatchReason)
	}
}

func TestApplyCapacitySoftGate_WithinPlan(t *testing.T) {
	prev := GlobalAgentLoadTracker
	GlobalAgentLoadTracker = NewAgentLoadTracker()
	t.Cleanup(func() { GlobalAgentLoadTracker = prev })

	reader := &stubAgentReader{agents: []biz.Agent{
		{AgentKey: "writer_a", DisplayName: "A", Status: "active", DomainPath: "content/write", PositionKey: "writer_a", ConfigJSON: `{"capacity":1}`},
		{AgentKey: "writer_b", DisplayName: "B", Status: "active", DomainPath: "content/write", PositionKey: "writer_b", ConfigJSON: `{"capacity":1}`},
	}}
	impl := &agentAllocatorImpl{
		repo:        &fakeAllocatorRepo{},
		agentReader: reader,
		capBuilder:  NewAgentCapabilityBuilder(reader, loggateway.NewNoop()),
		lg:          loggateway.NewNoop(),
	}
	caps, err := impl.capBuilder.BuildAll(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	assignable := filterHeuristicAssignable(caps)
	plan := &biz.TaskPlan{
		ID: "tp-intra",
		SubTasks: []biz.SubTask{
			{ID: "st1", Name: "draft A", DomainPath: "content/write"},
			{ID: "st2", Name: "draft B", DomainPath: "content/write"},
		},
	}
	allocs := []biz.TaskAllocation{
		{SubTaskID: "st1", AssignedKey: "writer_a", AssignedType: "agent", MatchReason: "exact"},
		{SubTaskID: "st2", AssignedKey: "writer_a", AssignedType: "agent", MatchReason: "exact"},
	}
	out := impl.applyCapacitySoftGate(context.Background(), plan, allocs, assignable, "trace")
	if out[0].AssignedKey != "writer_a" {
		t.Fatalf("first=%q", out[0].AssignedKey)
	}
	if out[1].AssignedKey == "writer_a" && !strings.Contains(out[1].MatchReason, "capacity_soft") {
		t.Fatalf("second kept busy lead without annotation: %+v", out[1])
	}
	if out[1].AssignedKey != "writer_a" && out[1].AssignedKey != "writer_b" {
		t.Fatalf("unexpected second=%q", out[1].AssignedKey)
	}
}