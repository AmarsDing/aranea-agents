package evaluation

import (
	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/loggateway"
	"context"
	"sync"
	"testing"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// ISSUE-006 (framework path): partial case failure must fail the run even
// though the framework itself returns no global error.
func TestRunnerFrameworkCaseErrorMarksRunFailed(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "ok", ExpectedOutput: "fine"},
		{ID: "c2", DatasetID: "ds1", Input: "bad", ExpectedOutput: "never"},
	}
	uc := beval.NewUsecase(beval.StoresFrom(repo), loggateway.NewNoop())
	bridge := NewFrameworkBridge(
		func(string) (runner.Runner, error) {
			return &contentSwitchRunner{reply: "fine", failOn: map[string]bool{"bad": true}}, nil
		},
		nil, nil, nil, MultiRunConfig{}, loggateway.NewNoop(),
	)
	r := NewRunner(uc, bridge, loggateway.NewNoop())

	_ = r.Start(context.Background(), biz.EvalRun{ID: "r3", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)

	final := waitRunTerminal(t, repo, "r3")
	if final.Status != "failed" {
		t.Fatalf("expected failed when a case errors, got %q (completed=%d/%d)", final.Status, final.CompletedCases, final.TotalCases)
	}
	if final.ErrorMessage == "" {
		t.Fatal("expected aggregated error summary on the run")
	}
}

// Happy path through the real framework: proves ISSUE-005 (SessionInput) and
// ISSUE-006 fixes together — a fully successful evaluation must complete with
// a non-zero score.
func TestRunnerFrameworkHappyPathCompletes(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "q1", ExpectedOutput: "fine"},
		{ID: "c2", DatasetID: "ds1", Input: "q2", ExpectedOutput: "fine"},
	}
	uc := beval.NewUsecase(beval.StoresFrom(repo), loggateway.NewNoop())
	bridge := NewFrameworkBridge(
		func(string) (runner.Runner, error) {
			return &contentSwitchRunner{reply: "fine"}, nil
		},
		nil, nil, nil, MultiRunConfig{}, loggateway.NewNoop(),
	)
	r := NewRunner(uc, bridge, loggateway.NewNoop())

	_ = r.Start(context.Background(), biz.EvalRun{ID: "r4", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)

	final := waitRunTerminal(t, repo, "r4")
	if final.Status != "completed" {
		t.Fatalf("expected completed, got %q (err=%q)", final.Status, final.ErrorMessage)
	}
	if final.CompletedCases != final.TotalCases || final.TotalCases != 2 {
		t.Fatalf("expected 2/2 completed cases, got %d/%d", final.CompletedCases, final.TotalCases)
	}
	if final.ExactMatchScore != 1 {
		t.Fatalf("expected exact_match=1, got %v", final.ExactMatchScore)
	}
}

func TestRunnerCancelMarksCancelled(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{{ID: "c1", DatasetID: "ds1", Input: "q", ExpectedOutput: "a"}}
	uc := beval.NewUsecase(beval.StoresFrom(repo), loggateway.NewNoop())
	started := make(chan struct{})
	bridge := NewFrameworkBridge(
		func(string) (runner.Runner, error) {
			return &blockingEvalRunner{started: started}, nil
		},
		nil, nil, nil, MultiRunConfig{}, loggateway.NewNoop(),
	)
	r := NewRunner(uc, bridge, loggateway.NewNoop())
	if _, err := uc.CreateRun(context.Background(), beval.Run{ID: "r-cancel", DatasetID: "ds1", AgentID: "a1", Status: beval.RunStatusPending}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	_ = r.Start(context.Background(), biz.EvalRun{ID: "r-cancel", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("inference never started")
	}
	if !r.Cancel("r-cancel") {
		t.Fatal("Cancel should find the in-flight run")
	}
	if _, err := uc.CancelRun(context.Background(), "r-cancel"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	final := waitRunTerminal(t, repo, "r-cancel")
	if final.Status != "cancelled" {
		t.Fatalf("expected cancelled, got %q err=%q", final.Status, final.ErrorMessage)
	}
}

func TestRunnerRejectsWhenSlotsFull(t *testing.T) {
	repo := newFakeEvalRepo()
	uc := beval.NewUsecase(beval.StoresFrom(repo), loggateway.NewNoop())
	r := NewRunner(uc, echoBridge(), loggateway.NewNoop())
	for i := 0; i < DefaultMaxInFlight; i++ {
		r.slots <- struct{}{}
	}
	err := r.Start(context.Background(), biz.EvalRun{ID: "busy", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)
	if err == nil {
		t.Fatal("Start must reject when the concurrency cap is full")
	}
}

type blockingEvalRunner struct {
	started chan struct{}
	once    sync.Once
}

func (b *blockingEvalRunner) Run(ctx context.Context, _, _ string, _ model.Message, _ ...agent.RunOption) (<-chan *event.Event, error) {
	b.once.Do(func() { close(b.started) })
	ch := make(chan *event.Event)
	go func() {
		defer close(ch)
		<-ctx.Done()
	}()
	return ch, nil
}

func (b *blockingEvalRunner) Close() error { return nil }

var _ runner.Runner = (*blockingEvalRunner)(nil)
