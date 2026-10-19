package service

import (
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"
)

// collectSystemNotices subscribes to the bus and buffers SystemNoticeEvents
// until the returned stop func is called.
//
// V2Bus 契约（R-1 修复后）：cancel 只摘除订阅者，永不 close channel——
// 接收循环必须通过自己的 done channel 退出（红线 #23），禁止 range ch。
func collectSystemNotices(bus biz.EventBus) (notices func() []*biz.SystemNoticeEvent, stop func()) {
	ch, cancel := bus.Subscribe(biz.EventSubscribeOptions{})
	var mu sync.Mutex
	var buf []*biz.SystemNoticeEvent
	done := make(chan struct{})
	finished := make(chan struct{})
	go func() {
		defer close(finished)
		for {
			select {
			case <-done:
				return
			case ev := <-ch:
				if n, ok := ev.(*biz.SystemNoticeEvent); ok {
					mu.Lock()
					buf = append(buf, n)
					mu.Unlock()
				}
			}
		}
	}()
	return func() []*biz.SystemNoticeEvent {
			mu.Lock()
			defer mu.Unlock()
			out := make([]*biz.SystemNoticeEvent, len(buf))
			copy(out, buf)
			return out
		}, func() {
			cancel()
			close(done)
			<-finished
		}
}

func TestDagRun_ContractMismatch_PublishesSystemNotice(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-contract-mismatch",
		TaskID:    "task-contract",
		SessionID: "sess-contract",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-contract-mismatch", TaskID: "task-contract", Label: "upstream", Status: biz.PlanStepStatusPending, Version: 1,
				Deliverables: []biz.DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
			{ID: "s2", PlanID: "board-contract-mismatch", TaskID: "task-contract", Label: "downstream", DependsOn: []string{"s1"}, Status: biz.PlanStepStatusPending, Version: 1,
				InputContract: []biz.DeliverableContract{{Name: "data", Type: "data", Format: "json"}}},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	notices, stop := collectSystemNotices(bus)
	defer stop()

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched")
	}
	orch.completeStep("s1", true, "")
	if !orch.waitForCall("s2", 2*time.Second) {
		t.Fatal("s2 was not dispatched")
	}
	orch.completeStep("s2", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	var mismatch *biz.SystemNoticeEvent
	for _, n := range notices() {
		if n.NoticeType == "contract_mismatch" {
			mismatch = n
			break
		}
	}
	if mismatch == nil {
		t.Fatal("expected a contract_mismatch SystemNoticeEvent, got none")
	}
	if mismatch.SpiritSessionID() != "sess-contract" {
		t.Errorf("notice session = %q, want sess-contract", mismatch.SpiritSessionID())
	}
	warnings, ok := mismatch.Meta["warnings"].([]string)
	if !ok || len(warnings) == 0 {
		t.Fatalf("notice meta warnings missing or empty: %v", mismatch.Meta)
	}
	found := false
	for _, w := range warnings {
		if strings.Contains(w, `"data"`) {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("warnings should mention unmatched contract %q: %v", "data", warnings)
	}
}

// stubTaskPlanUpdater implements taskPlanStatusUpdater in memory (TS9-BUG-1).
type stubTaskPlanUpdater struct {
	mu   sync.Mutex
	plan *biz.TaskPlan
}

func (s *stubTaskPlanUpdater) GetByID(_ context.Context, id string) (*biz.TaskPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.plan != nil && s.plan.ID == id {
		cp := *s.plan
		return &cp, nil
	}
	return nil, errors.New("not found")
}

func (s *stubTaskPlanUpdater) Update(_ context.Context, plan *biz.TaskPlan) (*biz.TaskPlan, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	cp := *plan
	s.plan = &cp
	return plan, nil
}

func (s *stubTaskPlanUpdater) status() biz.TaskPlanStatus {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.plan.Status
}

func waitPlanStatus(t *testing.T, s *stubTaskPlanUpdater, want biz.TaskPlanStatus) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if s.status() == want {
			return
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatalf("plan status = %s, want %s (timed out waiting)", s.status(), want)
}

// TestPlanExecutor_TaskPlanStatusPropagation (TS9-BUG-1) verifies the TaskPlan
// lifecycle follows the PlanBoard: confirmed → executing → completed. Before
// the fix, plans stayed "confirmed" forever.
func TestPlanExecutor_TaskPlanStatusPropagation(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "pb_plan-1",
		TaskID:    "task-1",
		SessionID: "sess-1",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "pb_plan-1", TaskID: "task-1", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	updater := &stubTaskPlanUpdater{plan: &biz.TaskPlan{ID: "plan-1", Status: biz.TaskPlanStatusConfirmed}}
	pe.SetTaskPlanUpdater(updater)

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched in time")
	}

	waitPlanStatus(t, updater, biz.TaskPlanStatusExecuting)

	orch.completeStep("s1", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
	if got := updater.status(); got != biz.TaskPlanStatusCompleted {
		t.Fatalf("plan status after DAG completed = %s, want completed", got)
	}
}

// TestPlanExecutor_TaskPlanStatusPropagation_Failed (TS9-BUG-1) verifies a
// failed DAG run propagates failed to the TaskPlan.
// F2（2026-09-03）：首次失败触发自动重试，重试耗尽（第二次失败）后才收敛
// board/plan 终态。
func TestPlanExecutor_TaskPlanStatusPropagation_Failed(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "pb_plan-2",
		TaskID:    "task-2",
		SessionID: "sess-2",
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "pb_plan-2", TaskID: "task-2", Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	pe.stepRetryBackoff = 0
	updater := &stubTaskPlanUpdater{plan: &biz.TaskPlan{ID: "plan-2", Status: biz.TaskPlanStatusConfirmed}}
	pe.SetTaskPlanUpdater(updater)

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched in time")
	}
	waitPlanStatus(t, updater, biz.TaskPlanStatusExecuting)

	orch.completeStep("s1", false, "boom")

	if !orch.waitForCallCount("s1", 2, 2*time.Second) {
		t.Fatal("s1 was not auto-retried after first failure")
	}
	orch.completeStep("s1", false, "boom again")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe returned error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}
	if got := updater.status(); got != biz.TaskPlanStatusFailed {
		t.Fatalf("plan status after DAG failed = %s, want failed", got)
	}
}

// TestPlanExecutor_TaskPlanStatusPropagation_TerminalNotOverwritten
// (TS9-BUG-1) verifies an already-terminal TaskPlan is not overwritten by a
// late board terminal event (e.g. replay).
func TestPlanExecutor_TaskPlanStatusPropagation_TerminalNotOverwritten(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	updater := &stubTaskPlanUpdater{plan: &biz.TaskPlan{ID: "plan-3", Status: biz.TaskPlanStatusCompleted}}
	pe.SetTaskPlanUpdater(updater)

	pe.propagateTaskPlanExecuting(context.Background(), "pb_plan-3")
	if got := updater.status(); got != biz.TaskPlanStatusCompleted {
		t.Fatalf("executing propagation overwrote terminal plan: %s", got)
	}
	pe.propagateTaskPlanTerminal(context.Background(), "pb_plan-3", biz.PlanStatusFailed)
	if got := updater.status(); got != biz.TaskPlanStatusCompleted {
		t.Fatalf("terminal propagation overwrote terminal plan: %s", got)
	}
}

func TestDagRun_ContractMatch_NoMismatchNotice(t *testing.T) {
	t.Parallel()
	board := biz.PlanBoard{
		ID:        "board-contract-match",
		TaskID:    "task-contract-ok",
		SessionID: "sess-contract-ok",
		Status:    biz.PlanStatusExecuting,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-contract-match", TaskID: "task-contract-ok", Label: "upstream", Status: biz.PlanStepStatusPending, Version: 1,
				Deliverables: []biz.DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
			{ID: "s2", PlanID: "board-contract-match", TaskID: "task-contract-ok", Label: "downstream", DependsOn: []string{"s1"}, Status: biz.PlanStepStatusPending, Version: 1,
				InputContract: []biz.DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
		},
	}
	seq := &fakeSeq{}
	orch := newFakeOrchestrator().withSeq(seq)
	repos := newFakeReposForExecutor()
	seq.repos = repos
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	notices, stop := collectSystemNotices(bus)
	defer stop()

	done := make(chan error, 1)
	go func() { done <- pe.Subscribe(context.Background(), board) }()

	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("s1 was not dispatched")
	}
	orch.completeStep("s1", true, "")
	if !orch.waitForCall("s2", 2*time.Second) {
		t.Fatal("s2 was not dispatched")
	}
	orch.completeStep("s2", true, "")
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Subscribe: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Subscribe timed out")
	}

	for _, n := range notices() {
		if n.NoticeType == "contract_mismatch" {
			t.Fatalf("matching contracts should not emit contract_mismatch, got %+v", n)
		}
	}
}
