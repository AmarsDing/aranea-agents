package agent

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// fakeParallelAssembler implements tools.SpiritTeamAssemblerPort for testing
// orchestrateParallelTeams. It supports per-key failure injection, artificial
// delay (to verify concurrency), and an atomic call counter.
type fakeParallelAssembler struct {
	mu            sync.Mutex
	teamIDCounter int
	failOnKeys    map[string]bool
	delay         time.Duration
	callCount     int32 // atomic
}

func (f *fakeParallelAssembler) AssembleTeam(ctx context.Context, params biz.SpiritTeamParams) (biz.Team, biz.Session, map[string]string, error) {
	atomic.AddInt32(&f.callCount, 1)
	if f.delay > 0 {
		select {
		case <-time.After(f.delay):
		case <-ctx.Done():
			return biz.Team{}, biz.Session{}, nil, ctx.Err()
		}
	}
	key := ""
	if len(params.AgentKeys) > 0 {
		key = params.AgentKeys[0]
	}
	if f.failOnKeys[key] {
		return biz.Team{}, biz.Session{}, nil, fmt.Errorf("simulated assembly failure for %s", key)
	}
	f.mu.Lock()
	f.teamIDCounter++
	id := fmt.Sprintf("team_%d", f.teamIDCounter)
	f.mu.Unlock()
	return biz.Team{ID: id, TaskDescription: params.TaskDescription}, biz.Session{}, nil, nil
}

func (f *fakeParallelAssembler) SuggestTopology(_ context.Context, _ string) (string, bool) {
	return "parallel", true
}

// Compile-time check: fakeParallelAssembler implements the port interface.
var _ tools.SpiritTeamAssemblerPort = (*fakeParallelAssembler)(nil)

func newParallelOrchestrator(assembler tools.SpiritTeamAssemblerPort) *TaskOrchestratorImpl {
	return &TaskOrchestratorImpl{
		assembler: assembler,
		lg:        loggateway.NewNoop(),
	}
}

type stubOrchRepo struct {
	created *biz.OrchestrationHandle
}

func (s *stubOrchRepo) Create(_ context.Context, h *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	s.created = h
	return h, nil
}
func (s *stubOrchRepo) GetByID(context.Context, string) (*biz.OrchestrationHandle, error) {
	return nil, nil
}
func (s *stubOrchRepo) Update(context.Context, *biz.OrchestrationHandle) (*biz.OrchestrationHandle, error) {
	return nil, nil
}
func (s *stubOrchRepo) ListBySpiritSessionID(context.Context, string) ([]*biz.OrchestrationHandle, error) {
	return nil, nil
}
func (s *stubOrchRepo) ListByStatus(context.Context, biz.OrchestrationStatus) ([]*biz.OrchestrationHandle, error) {
	return nil, nil
}

func TestOrchestrate_RetiredTeamStrategies(t *testing.T) {
	o := &TaskOrchestratorImpl{lg: loggateway.NewNoop()}
	plan := &biz.TaskPlan{ID: "tp1", SpiritSessionID: "sp1"}
	alloc := &biz.AllocationPlan{ID: "ap1"}
	retired := []biz.OrchestrationStrategy{
		biz.StrategySingleAgent, biz.StrategyParallel, biz.StrategyDAG, biz.StrategyCoordinator,
	}
	for _, s := range retired {
		plan.Strategy = s
		_, err := o.Orchestrate(context.Background(), plan, alloc)
		if err == nil {
			t.Fatalf("strategy %s: expected retired error", s)
		}
		if !apierror.IsCode(err, apierror.CodeFailedPrecondition) {
			t.Fatalf("strategy %s: want FAILED_PRECONDITION, got %v", s, err)
		}
	}
}

func TestOrchestrate_DirectStillPersistsHandle(t *testing.T) {
	repo := &stubOrchRepo{}
	o := &TaskOrchestratorImpl{lg: loggateway.NewNoop(), repo: repo}
	handle, err := o.Orchestrate(context.Background(), &biz.TaskPlan{
		ID: "tp-direct", SpiritSessionID: "sp1", Strategy: biz.StrategyDirect,
	}, &biz.AllocationPlan{ID: "ap1"})
	if err != nil {
		t.Fatalf("direct: %v", err)
	}
	if handle == nil || repo.created == nil {
		t.Fatal("direct must persist a handle")
	}
	if handle.Status != biz.OrchestrationStatusCompleted {
		t.Fatalf("status=%q want completed", handle.Status)
	}
}

func makeParallelAllocPlan(keys ...string) *biz.AllocationPlan {
	allocs := make([]biz.TaskAllocation, 0, len(keys))
	for i, k := range keys {
		allocs = append(allocs, biz.TaskAllocation{
			SubTaskID:   fmt.Sprintf("sub_%d", i+1),
			SubTaskName: fmt.Sprintf("Subtask %d", i+1),
			AssignedKey: k,
		})
	}
	return &biz.AllocationPlan{Allocations: allocs}
}

// TestOrchestrateParallelTeams_AllSuccess verifies that when all allocations
// assemble successfully, all team IDs are returned without error.
func TestOrchestrateParallelTeams_AllSuccess(t *testing.T) {
	asm := &fakeParallelAssembler{}
	orch := newParallelOrchestrator(asm)
	taskPlan := &biz.TaskPlan{SpiritSessionID: "sess_1", UserMessage: "do work"}
	allocPlan := makeParallelAllocPlan("agent_a", "agent_b", "agent_c")

	teamIDs, err := orch.orchestrateParallelTeams(context.Background(), taskPlan, allocPlan)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(teamIDs) != 3 {
		t.Fatalf("expected 3 team IDs, got %d: %v", len(teamIDs), teamIDs)
	}
	seen := make(map[string]bool)
	for _, id := range teamIDs {
		if id == "" {
			t.Error("expected non-empty team ID")
		}
		if seen[id] {
			t.Errorf("duplicate team ID: %s", id)
		}
		seen[id] = true
	}
	if got := atomic.LoadInt32(&asm.callCount); got != 3 {
		t.Errorf("expected 3 assembler calls, got %d", got)
	}
}

// TestOrchestrateParallelTeams_PartialFailure verifies that a single team
// assembly failure does not block other teams; successful team IDs are
// returned without error.
func TestOrchestrateParallelTeams_PartialFailure(t *testing.T) {
	asm := &fakeParallelAssembler{
		failOnKeys: map[string]bool{"agent_b": true},
	}
	orch := newParallelOrchestrator(asm)
	taskPlan := &biz.TaskPlan{SpiritSessionID: "sess_1", UserMessage: "do work"}
	allocPlan := makeParallelAllocPlan("agent_a", "agent_b", "agent_c")

	teamIDs, err := orch.orchestrateParallelTeams(context.Background(), taskPlan, allocPlan)
	if err != nil {
		t.Fatalf("expected no error (partial failure tolerated), got %v", err)
	}
	if len(teamIDs) != 2 {
		t.Fatalf("expected 2 team IDs (agent_b failed), got %d: %v", len(teamIDs), teamIDs)
	}
	if got := atomic.LoadInt32(&asm.callCount); got != 3 {
		t.Errorf("expected 3 assembler calls (all attempted), got %d", got)
	}
}

// TestOrchestrateParallelTeams_AllFailed verifies that when all teams fail to
// assemble, an error is returned and no team IDs are produced.
func TestOrchestrateParallelTeams_AllFailed(t *testing.T) {
	asm := &fakeParallelAssembler{
		failOnKeys: map[string]bool{"agent_a": true, "agent_b": true},
	}
	orch := newParallelOrchestrator(asm)
	taskPlan := &biz.TaskPlan{SpiritSessionID: "sess_1", UserMessage: "do work"}
	allocPlan := makeParallelAllocPlan("agent_a", "agent_b")

	teamIDs, err := orch.orchestrateParallelTeams(context.Background(), taskPlan, allocPlan)
	if err == nil {
		t.Fatal("expected error when all teams fail, got nil")
	}
	if len(teamIDs) != 0 {
		t.Fatalf("expected 0 team IDs, got %d: %v", len(teamIDs), teamIDs)
	}
	if !strings.Contains(err.Error(), "all parallel teams failed to assemble") {
		t.Errorf("expected 'all parallel teams failed to assemble' error, got: %v", err)
	}
}

// TestOrchestrateParallelTeams_EmptyAllocations verifies that an empty
// allocation plan returns nil team IDs and no error.
func TestOrchestrateParallelTeams_EmptyAllocations(t *testing.T) {
	asm := &fakeParallelAssembler{}
	orch := newParallelOrchestrator(asm)
	taskPlan := &biz.TaskPlan{SpiritSessionID: "sess_1", UserMessage: "do work"}
	allocPlan := &biz.AllocationPlan{} // no allocations

	teamIDs, err := orch.orchestrateParallelTeams(context.Background(), taskPlan, allocPlan)
	if err != nil {
		t.Fatalf("expected no error for empty allocations, got %v", err)
	}
	if teamIDs != nil {
		t.Errorf("expected nil team IDs, got %v", teamIDs)
	}
	if got := atomic.LoadInt32(&asm.callCount); got != 0 {
		t.Errorf("expected 0 assembler calls, got %d", got)
	}
}

// TestOrchestrateParallelTeams_ConcurrentSafety verifies that teams are
// assembled concurrently (not serially) by checking that the total elapsed
// time is well below the serial-equivalent duration. Run with -race to also
// detect data races in the results slice handling.
func TestOrchestrateParallelTeams_ConcurrentSafety(t *testing.T) {
	// Each assembly sleeps for `delay`. Serial execution of 3 allocations
	// would take >= 3*delay; concurrent execution should be close to 1*delay.
	delay := 50 * time.Millisecond
	asm := &fakeParallelAssembler{delay: delay}
	orch := newParallelOrchestrator(asm)
	taskPlan := &biz.TaskPlan{SpiritSessionID: "sess_1", UserMessage: "do work"}
	allocPlan := makeParallelAllocPlan("agent_a", "agent_b", "agent_c")

	start := time.Now()
	teamIDs, err := orch.orchestrateParallelTeams(context.Background(), taskPlan, allocPlan)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if len(teamIDs) != 3 {
		t.Fatalf("expected 3 team IDs, got %d", len(teamIDs))
	}
	// Serial would be >= 150ms; concurrent should be well under. Use a
	// generous threshold (2*delay) to avoid CI flakiness while still
	// proving concurrency.
	if elapsed >= 2*delay {
		t.Errorf("expected concurrent execution (< %v), got %v (serial would be %v)", 2*delay, elapsed, 3*delay)
	}
	if got := atomic.LoadInt32(&asm.callCount); got != 3 {
		t.Errorf("expected 3 assembler calls, got %d", got)
	}
}
