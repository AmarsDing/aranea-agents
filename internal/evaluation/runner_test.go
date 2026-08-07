package evaluation

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	beval "aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/loggateway"

	"trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/event"
	"trpc.group/trpc-go/trpc-agent-go/model"
	"trpc.group/trpc-go/trpc-agent-go/runner"
)

// fakeEvalRepo implements beval.Repo. Every method honors ctx cancellation to
// simulate real database driver behavior (the Postgres driver aborts when the
// caller's context is cancelled).
type fakeEvalRepo struct {
	mu        sync.Mutex
	datasets  map[string]beval.Dataset
	cases     map[string][]beval.Case
	runs      map[string]beval.Run
	results   []beval.CaseResult
	insertErr error // when set, InsertCaseResult fails with this error
}

func newFakeEvalRepo() *fakeEvalRepo {
	return &fakeEvalRepo{
		datasets: make(map[string]beval.Dataset),
		cases:    make(map[string][]beval.Case),
		runs:     make(map[string]beval.Run),
	}
}

func (f *fakeEvalRepo) runSnapshot(id string) (beval.Run, bool) {
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[id]
	return r, ok
}

func (f *fakeEvalRepo) CreateDataset(ctx context.Context, d beval.Dataset) (beval.Dataset, error) {
	if err := ctx.Err(); err != nil {
		return beval.Dataset{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.datasets[d.ID] = d
	return d, nil
}

func (f *fakeEvalRepo) GetDataset(ctx context.Context, id string) (beval.Dataset, error) {
	if err := ctx.Err(); err != nil {
		return beval.Dataset{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	d, ok := f.datasets[id]
	if !ok {
		return beval.Dataset{}, errors.New("dataset not found")
	}
	return d, nil
}

func (f *fakeEvalRepo) ListDatasets(context.Context, string, int, int) ([]beval.Dataset, int, error) {
	return nil, 0, nil
}

func (f *fakeEvalRepo) DeleteDataset(context.Context, string) error { return nil }

func (f *fakeEvalRepo) UpdateDataset(_ context.Context, id, _, _ string) (beval.Dataset, error) {
	return f.datasets[id], nil
}

func (f *fakeEvalRepo) UpdateDatasetCaseCount(context.Context, string, int) error { return nil }

func (f *fakeEvalRepo) InsertCases(context.Context, []beval.Case) error { return nil }

func (f *fakeEvalRepo) InsertCasesWithCountUpdate(context.Context, string, []beval.Case) error {
	return nil
}

func (f *fakeEvalRepo) ListCases(ctx context.Context, datasetID string) ([]beval.Case, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]beval.Case(nil), f.cases[datasetID]...), nil
}

func (f *fakeEvalRepo) CreateRun(ctx context.Context, r beval.Run) (beval.Run, error) {
	if err := ctx.Err(); err != nil {
		return beval.Run{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs[r.ID] = r
	return r, nil
}

func (f *fakeEvalRepo) GetRun(ctx context.Context, id string) (beval.Run, error) {
	if err := ctx.Err(); err != nil {
		return beval.Run{}, err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	r, ok := f.runs[id]
	if !ok {
		return beval.Run{}, errors.New("run not found")
	}
	return r, nil
}

func (f *fakeEvalRepo) UpdateRun(ctx context.Context, r beval.Run) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	f.runs[r.ID] = r
	return nil
}

func (f *fakeEvalRepo) DeleteRun(context.Context, string) error { return nil }

func (f *fakeEvalRepo) ListRuns(context.Context, string, string, int, int) ([]beval.Run, int, error) {
	return nil, 0, nil
}

func (f *fakeEvalRepo) InsertCaseResult(ctx context.Context, r beval.CaseResult) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.insertErr != nil {
		return f.insertErr
	}
	f.results = append(f.results, r)
	return nil
}

func (f *fakeEvalRepo) ListCaseResults(context.Context, string, int, int) ([]beval.CaseResult, int, error) {
	return nil, 0, nil
}

func (f *fakeEvalRepo) GetCaseResult(context.Context, string, string) (beval.CaseResult, error) {
	return beval.CaseResult{}, nil
}

func (f *fakeEvalRepo) UpdateCaseResultAnnotation(context.Context, string, string, beval.CaseResultAnnotation) (beval.CaseResult, error) {
	return beval.CaseResult{}, nil
}

func (f *fakeEvalRepo) ListTrendPoints(context.Context, string, string, int) ([]beval.TrendPoint, error) {
	return nil, nil
}

func (f *fakeEvalRepo) GetRunsByIDs(context.Context, []string) ([]beval.Run, error) {
	return nil, nil
}

// contentSwitchRunner is a framework runner that answers each user message
// with a fixed reply, and fails inputs listed in failOn.
type contentSwitchRunner struct {
	mu     sync.Mutex
	reply  string
	failOn map[string]bool
}

func (c *contentSwitchRunner) Run(_ context.Context, _, _ string, message model.Message, _ ...agent.RunOption) (<-chan *event.Event, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.failOn[message.Content] {
		return nil, errors.New("inference boom for input: " + message.Content)
	}
	ch := make(chan *event.Event, 1)
	ch <- &event.Event{
		InvocationID: "inv-1",
		Response: &model.Response{
			Done: true,
			Choices: []model.Choice{
				{Message: model.Message{Role: model.RoleAssistant, Content: c.reply}},
			},
		},
	}
	close(ch)
	return ch, nil
}

func (c *contentSwitchRunner) Close() error { return nil }

var _ runner.Runner = (*contentSwitchRunner)(nil)

func echoAgent(_ context.Context, _, input string) (string, error) { return input, nil }

func waitRunTerminal(t *testing.T, repo *fakeEvalRepo, runID string) beval.Run {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		r, ok := repo.runSnapshot(runID)
		if ok && (r.Status == "completed" || r.Status == "failed") {
			return r
		}
		if time.Now().After(deadline) {
			status := "missing"
			if ok {
				status = r.Status
			}
			t.Fatalf("run %s did not reach a terminal state (status=%q)", runID, status)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// E2E-discovered (2026-08-08): result IDs generated from a nanosecond time
// format collide within a single clock tick (Windows timer granularity is
// ~0.5ms), so two case results in the same run violated eval_case_results_pkey
// and the second row was silently dropped. IDs must be unique per call.
func TestNewEvalResultIDUnique(t *testing.T) {
	const n = 1000
	seen := make(map[string]struct{}, n)
	for i := 0; i < n; i++ {
		id := newEvalResultID()
		if _, dup := seen[id]; dup {
			t.Fatalf("duplicate result id %q at iteration %d", id, i)
		}
		seen[id] = struct{}{}
	}
}

// E2E-discovered: a persistence failure on InsertCaseResult (e.g. PK conflict)
// must be treated as a case error and fail the run; otherwise the run reports
// "completed" while case rows are silently missing from the database.
func TestRunnerFrameworkInsertErrorMarksRunFailed(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.insertErr = errors.New("pq: duplicate key value violates unique constraint")
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{{ID: "c1", DatasetID: "ds1", Input: "q1", ExpectedOutput: "fine"}}
	uc := beval.NewUsecase(repo, loggateway.NewNoop())
	bridge := NewFrameworkBridge(
		func(string) (runner.Runner, error) {
			return &contentSwitchRunner{reply: "fine"}, nil
		},
		nil, nil, nil, MultiRunConfig{}, loggateway.NewNoop(),
	)
	r := NewRunner(uc, echoAgent, bridge, loggateway.NewNoop())

	r.Start(context.Background(), biz.EvalRun{ID: "r5", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)

	final := waitRunTerminal(t, repo, "r5")
	if final.Status != "failed" {
		t.Fatalf("expected failed when result persistence errors, got %q", final.Status)
	}
	if !strings.Contains(final.ErrorMessage, "persist") {
		t.Fatalf("expected error summary to mention persistence failure, got %q", final.ErrorMessage)
	}
}

// ISSUE-002: the async goroutine must be isolated from the HTTP request ctx.
// Kratos cancels the request ctx as soon as RunEvaluation returns; without
// isolation the run dies immediately and stays "pending" forever.
func TestRunnerStartRequestCtxCancelledStillCompletes(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{{ID: "c1", DatasetID: "ds1", Input: "hi", ExpectedOutput: "hi"}}
	uc := beval.NewUsecase(repo, loggateway.NewNoop())
	r := NewRunner(uc, echoAgent, nil, loggateway.NewNoop())

	if _, err := uc.CreateRun(context.Background(), beval.Run{ID: "r1", DatasetID: "ds1", AgentID: "a1", Status: "pending"}); err != nil {
		t.Fatalf("create run: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Kratos cancels the request ctx right after the handler returns
	r.Start(ctx, biz.EvalRun{ID: "r1", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)

	final := waitRunTerminal(t, repo, "r1")
	if final.Status != "completed" {
		t.Fatalf("expected completed, got %q (err=%q) — request ctx leaked into async execution", final.Status, final.ErrorMessage)
	}
	if final.ExactMatchScore != 1 {
		t.Fatalf("expected exact_match=1, got %v", final.ExactMatchScore)
	}
}

// ISSUE-006 (legacy path): a case-level agent error must mark the run failed,
// not completed.
func TestRunnerLegacyCaseErrorMarksRunFailed(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "ok", ExpectedOutput: "ok"},
		{ID: "c2", DatasetID: "ds1", Input: "boom", ExpectedOutput: "x"},
	}
	uc := beval.NewUsecase(repo, loggateway.NewNoop())
	agentFn := func(_ context.Context, _, input string) (string, error) {
		if input == "boom" {
			return "", errors.New("agent exploded")
		}
		return input, nil
	}
	r := NewRunner(uc, agentFn, nil, loggateway.NewNoop())

	r.Start(context.Background(), biz.EvalRun{ID: "r2", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)

	final := waitRunTerminal(t, repo, "r2")
	if final.Status != "failed" {
		t.Fatalf("expected failed when a case errors, got %q", final.Status)
	}
	if !strings.Contains(final.ErrorMessage, "agent exploded") {
		t.Fatalf("expected error summary to include case error, got %q", final.ErrorMessage)
	}
}

// ISSUE-006 (framework path): partial case failure must fail the run even
// though the framework itself returns no global error.
func TestRunnerFrameworkCaseErrorMarksRunFailed(t *testing.T) {
	repo := newFakeEvalRepo()
	repo.datasets["ds1"] = beval.Dataset{ID: "ds1"}
	repo.cases["ds1"] = []beval.Case{
		{ID: "c1", DatasetID: "ds1", Input: "ok", ExpectedOutput: "fine"},
		{ID: "c2", DatasetID: "ds1", Input: "bad", ExpectedOutput: "never"},
	}
	uc := beval.NewUsecase(repo, loggateway.NewNoop())
	bridge := NewFrameworkBridge(
		func(string) (runner.Runner, error) {
			return &contentSwitchRunner{reply: "fine", failOn: map[string]bool{"bad": true}}, nil
		},
		nil, nil, nil, MultiRunConfig{}, loggateway.NewNoop(),
	)
	r := NewRunner(uc, echoAgent, bridge, loggateway.NewNoop())

	r.Start(context.Background(), biz.EvalRun{ID: "r3", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)

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
	uc := beval.NewUsecase(repo, loggateway.NewNoop())
	bridge := NewFrameworkBridge(
		func(string) (runner.Runner, error) {
			return &contentSwitchRunner{reply: "fine"}, nil
		},
		nil, nil, nil, MultiRunConfig{}, loggateway.NewNoop(),
	)
	r := NewRunner(uc, echoAgent, bridge, loggateway.NewNoop())

	r.Start(context.Background(), biz.EvalRun{ID: "r4", DatasetID: "ds1", AgentID: "a1"}, "exact_match", 1, false)

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
