package team

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
)

type stubTaskCreator struct {
	calls []string
	err   error
}

func (s *stubTaskCreator) CreateGraphTask(_ context.Context, _, _ string, node biz.NodeDef) error {
	s.calls = append(s.calls, node.ID)
	return s.err
}

type stubExecRegistry struct {
	interrupts []string
}

func (s *stubExecRegistry) RegisterTeamGraphExecution(context.Context, string, string, string, string, biz.GraphBuildConfig) error {
	return nil
}

func (s *stubExecRegistry) MarkTeamGraphInterrupt(_ context.Context, execID, nodeID, _ string) error {
	s.interrupts = append(s.interrupts, execID+":"+nodeID)
	return nil
}

func TestStartTeamGraphTaskBridge_createsTaskOnce(t *testing.T) {
	bus := event.NewBus()
	ctx := context.Background()
	creator := &stubTaskCreator{}
	nodes := map[string]biz.NodeDef{"review-1": {ID: "review-1", Type: "review"}}
	stop := StartTeamGraphTaskBridge(ctx, bus, TeamGraphTaskBridgeConfig{
		SessionID: "sess-1", GraphExecutionID: "exec-1", Nodes: nodes, Creator: creator,
	})
	defer stop()

	env := event.NewEnvelope(event.EnvelopeTypeGraphNodeStart, "system", "sess-1")
	env.Metadata = map[string]any{"execution_id": "exec-1", "node_id": "review-1"}
	bus.Publish(ctx, env)
	bus.Publish(ctx, env)
	time.Sleep(30 * time.Millisecond)
	if len(creator.calls) != 1 || creator.calls[0] != "review-1" {
		t.Fatalf("calls=%v", creator.calls)
	}
}

func TestStartTeamGraphExecutionTracker_marksInterrupt(t *testing.T) {
	bus := event.NewBus()
	ctx := context.Background()
	reg := &stubExecRegistry{}
	stop := StartTeamGraphExecutionTracker(ctx, bus, TeamGraphExecutionTrackerConfig{
		SessionID: "sess-1", GraphExecutionID: "exec-1", Registry: reg,
	})
	defer stop()

	env := event.NewEnvelope(event.EnvelopeTypeCheckpoint, "system", "sess-1")
	env.Metadata = map[string]any{
		"execution_id": "exec-1",
		"node_id":      "review-1",
		"lineage_id":   "lineage-1",
	}
	bus.Publish(ctx, env)
	time.Sleep(30 * time.Millisecond)
	if len(reg.interrupts) != 1 || reg.interrupts[0] != "exec-1:review-1" {
		t.Fatalf("interrupts=%v", reg.interrupts)
	}
}

func TestStartTeamGraphTaskBridge_logsCreateError(t *testing.T) {
	bus := event.NewBus()
	ctx := context.Background()
	creator := &stubTaskCreator{err: errors.New("boom")}
	stop := StartTeamGraphTaskBridge(ctx, bus, TeamGraphTaskBridgeConfig{
		SessionID: "sess-1", GraphExecutionID: "exec-1",
		Nodes: map[string]biz.NodeDef{"t1": {ID: "t1", Type: "task"}}, Creator: creator,
	})
	defer stop()
	env := event.NewEnvelope(event.EnvelopeTypeGraphNodeStart, "system", "sess-1")
	env.Metadata = map[string]any{"execution_id": "exec-1", "node_id": "t1"}
	bus.Publish(ctx, env)
	time.Sleep(20 * time.Millisecond)
	if len(creator.calls) != 1 {
		t.Fatalf("calls=%v", creator.calls)
	}
}
