package jobs

import (
	"context"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/cron"
	"aranea-agents/pkg/loggateway"
)

// ─── Test stubs ───────────────────────────────────────────────────────────────

type sweeperJobRepo struct {
	jobs        map[string]biz.ChannelTurnJob
	listStaleFn func(status, beforeUpdatedAt string) []biz.ChannelTurnJob
	updates     []sweeperJobUpdate
}

type sweeperJobUpdate struct {
	id             string
	status         string
	errMsg         string
	previewMsgID   string
	contentPreview string
}

func (s *sweeperJobRepo) Create(_ context.Context, job biz.ChannelTurnJob) (string, error) {
	if job.ID == "" {
		job.ID = "job-auto"
	}
	s.jobs[job.ID] = job
	return job.ID, nil
}

func (s *sweeperJobRepo) UpdateStatus(_ context.Context, id, status, errMsg, previewMsgID, contentPreview string) error {
	s.updates = append(s.updates, sweeperJobUpdate{id, status, errMsg, previewMsgID, contentPreview})
	if j, ok := s.jobs[id]; ok {
		j.Status = status
		j.ErrorMessage = errMsg
		s.jobs[id] = j
	}
	return nil
}

func (s *sweeperJobRepo) UpdateAsyncTarget(_ context.Context, id, targetType, targetID string) error {
	return nil
}

func (s *sweeperJobRepo) GetByIdempotency(_ context.Context, _, _ string) (biz.ChannelTurnJob, error) {
	return biz.ChannelTurnJob{}, nil
}

func (s *sweeperJobRepo) GetByID(_ context.Context, id string) (biz.ChannelTurnJob, error) {
	return s.jobs[id], nil
}

func (s *sweeperJobRepo) ListByChannel(_ context.Context, _ string, _ int) ([]biz.ChannelTurnJob, error) {
	return nil, nil
}

func (s *sweeperJobRepo) ListFiltered(_ context.Context, _ biz.ChannelTurnJobListQuery) ([]biz.ChannelTurnJob, error) {
	return nil, nil
}

func (s *sweeperJobRepo) ListActiveBySession(_ context.Context, _, _ string) ([]biz.ChannelTurnJob, error) {
	return nil, nil
}

func (s *sweeperJobRepo) ListStaleByStatus(_ context.Context, status, beforeUpdatedAt string, _ int) ([]biz.ChannelTurnJob, error) {
	if s.listStaleFn != nil {
		return s.listStaleFn(status, beforeUpdatedAt), nil
	}
	return nil, nil
}

type sweeperGraphExec struct {
	exec *biz.GraphExecution
	err  error
}

func (g *sweeperGraphExec) GetExecution(_ context.Context, _ string) (*biz.GraphExecution, error) {
	if g.err != nil {
		return nil, g.err
	}
	return g.exec, nil
}

type sweeperCron struct {
	run cron.TaskRun
	err  error
}

func (c *sweeperCron) TriggerCronTask(_ context.Context, _ string) (cron.TaskRun, error) {
	return c.run, c.err
}

func (c *sweeperCron) GetTaskRun(_ context.Context, _ string) (cron.TaskRun, error) {
	return c.run, c.err
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

func newSweeperTestRepo(jobs ...biz.ChannelTurnJob) *sweeperJobRepo {
	m := make(map[string]biz.ChannelTurnJob, len(jobs))
	for _, j := range jobs {
		m[j.ID] = j
	}
	return &sweeperJobRepo{jobs: m}
}

func newSweeper(repo *sweeperJobRepo, graphExec ExecutionStatusReader, cronGW biz.CronTriggerGateway) *ChannelTurnJobSweeper {
	return NewChannelTurnJobSweeper(
		1*time.Minute,
		30*time.Minute,
		repo,
		graphExec,
		cronGW,
		loggateway.NewNoop(),
	)
}

func staleJob(id, status, targetType, targetID string, age time.Duration) biz.ChannelTurnJob {
	return biz.ChannelTurnJob{
		ID:              id,
		ChannelID:       "ch1",
		Status:          status,
		AsyncTargetType: targetType,
		AsyncTargetID:   targetID,
		UpdatedAt:       time.Now().UTC().Add(-age).Format(time.RFC3339Nano),
	}
}

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestSweeper_QueuedTimeout verifies that Queued jobs older than the timeout
// are transitioned to Timeout (P0 #8).
func TestSweeper_QueuedTimeout(t *testing.T) {
	staleJob := staleJob("job-1", biz.ChannelTurnJobStatusQueued, "", "", 31*time.Minute)
	repo := newSweeperTestRepo(staleJob)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusQueued {
			return nil
		}
		return []biz.ChannelTurnJob{staleJob}
	}

	w := newSweeper(repo, nil, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	upd := repo.updates[0]
	if upd.id != "job-1" || upd.status != biz.ChannelTurnJobStatusTimeout {
		t.Errorf("expected job-1→timeout, got %s→%s", upd.id, upd.status)
	}
}

// TestSweeper_QueuedTimeout_NoStaleJobs verifies that fresh Queued jobs are not timed out.
func TestSweeper_QueuedTimeout_NoStaleJobs(t *testing.T) {
	repo := newSweeperTestRepo()
	repo.listStaleFn = func(_, _ string) []biz.ChannelTurnJob {
		return nil
	}

	w := newSweeper(repo, nil, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 0 {
		t.Fatalf("expected 0 updates, got %d", len(repo.updates))
	}
}

// TestSweeper_AsyncRecovery_GraphCompleted verifies that AsyncQueued jobs
// with a completed graph target are transitioned to Completed (P0 #9).
func TestSweeper_AsyncRecovery_GraphCompleted(t *testing.T) {
	job := staleJob("job-async-1", biz.ChannelTurnJobStatusAsyncQueued, "graph", "exec-1", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	graphExec := &sweeperGraphExec{
		exec: &biz.GraphExecution{ID: "exec-1", Status: string(biz.GraphExecCompleted)},
	}

	w := newSweeper(repo, graphExec, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	upd := repo.updates[0]
	if upd.status != biz.ChannelTurnJobStatusCompleted {
		t.Errorf("expected status %s, got %s", biz.ChannelTurnJobStatusCompleted, upd.status)
	}
}

// TestSweeper_AsyncRecovery_GraphFailed verifies that AsyncQueued jobs
// with a failed graph target are transitioned to Failed.
func TestSweeper_AsyncRecovery_GraphFailed(t *testing.T) {
	job := staleJob("job-async-2", biz.ChannelTurnJobStatusAsyncQueued, "graph", "exec-2", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	graphExec := &sweeperGraphExec{
		exec: &biz.GraphExecution{ID: "exec-2", Status: string(biz.GraphExecFailed), ErrorMessage: "node X failed"},
	}

	w := newSweeper(repo, graphExec, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	upd := repo.updates[0]
	if upd.status != biz.ChannelTurnJobStatusFailed {
		t.Errorf("expected status %s, got %s", biz.ChannelTurnJobStatusFailed, upd.status)
	}
	if upd.errMsg == "" {
		t.Error("expected non-empty error message")
	}
}

// TestSweeper_AsyncRecovery_GraphRunning verifies that AsyncQueued jobs
// with a still-running graph target are touched (updated_at refreshed), not transitioned.
func TestSweeper_AsyncRecovery_GraphRunning(t *testing.T) {
	job := staleJob("job-async-3", biz.ChannelTurnJobStatusAsyncQueued, "graph", "exec-3", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	graphExec := &sweeperGraphExec{
		exec: &biz.GraphExecution{ID: "exec-3", Status: string(biz.GraphExecRunning)},
	}

	w := newSweeper(repo, graphExec, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 touch update, got %d", len(repo.updates))
	}
	upd := repo.updates[0]
	if upd.status != biz.ChannelTurnJobStatusAsyncQueued {
		t.Errorf("expected status %s (touch), got %s", biz.ChannelTurnJobStatusAsyncQueued, upd.status)
	}
}

// TestSweeper_AsyncRecovery_CronSuccess verifies that AsyncQueued jobs
// with a succeeded cron target are transitioned to Completed.
func TestSweeper_AsyncRecovery_CronSuccess(t *testing.T) {
	job := staleJob("job-cron-1", biz.ChannelTurnJobStatusAsyncQueued, "cron", "run-1", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	cronGW := &sweeperCron{
		run: cron.TaskRun{ID: "run-1", Status: "success"},
	}

	w := newSweeper(repo, nil, cronGW)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	if repo.updates[0].status != biz.ChannelTurnJobStatusCompleted {
		t.Errorf("expected status %s, got %s", biz.ChannelTurnJobStatusCompleted, repo.updates[0].status)
	}
}

// TestSweeper_AsyncRecovery_CronFailure verifies that AsyncQueued jobs
// with a failed cron target are transitioned to Failed.
func TestSweeper_AsyncRecovery_CronFailure(t *testing.T) {
	job := staleJob("job-cron-2", biz.ChannelTurnJobStatusAsyncQueued, "cron", "run-2", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	cronGW := &sweeperCron{
		run: cron.TaskRun{ID: "run-2", Status: "failure", ErrorMessage: "cron task error"},
	}

	w := newSweeper(repo, nil, cronGW)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	if repo.updates[0].status != biz.ChannelTurnJobStatusFailed {
		t.Errorf("expected status %s, got %s", biz.ChannelTurnJobStatusFailed, repo.updates[0].status)
	}
}

// TestSweeper_AsyncRecovery_UnknownTargetType verifies that jobs with an
// unknown async_target_type are force-timed-out.
func TestSweeper_AsyncRecovery_UnknownTargetType(t *testing.T) {
	job := staleJob("job-unknown", biz.ChannelTurnJobStatusAsyncQueued, "webhook", "target-1", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	w := newSweeper(repo, nil, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	if repo.updates[0].status != biz.ChannelTurnJobStatusTimeout {
		t.Errorf("expected status %s, got %s", biz.ChannelTurnJobStatusTimeout, repo.updates[0].status)
	}
}

// TestSweeper_AsyncRecovery_NoTargetID verifies that jobs with no async_target_id
// are force-timed-out.
func TestSweeper_AsyncRecovery_NoTargetID(t *testing.T) {
	job := staleJob("job-notarget", biz.ChannelTurnJobStatusAsyncQueued, "graph", "", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	w := newSweeper(repo, nil, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	if repo.updates[0].status != biz.ChannelTurnJobStatusTimeout {
		t.Errorf("expected status %s, got %s", biz.ChannelTurnJobStatusTimeout, repo.updates[0].status)
	}
}

// TestSweeper_AsyncRecovery_MaxAgeForceTimeout verifies that jobs older than
// turnJobAsyncMaxAge are force-timed-out regardless of target status.
func TestSweeper_AsyncRecovery_MaxAgeForceTimeout(t *testing.T) {
	job := staleJob("job-old", biz.ChannelTurnJobStatusAsyncQueued, "graph", "exec-old", 25*time.Hour)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	// Even though graph says "running", the job is too old and must be force-timed-out.
	graphExec := &sweeperGraphExec{
		exec: &biz.GraphExecution{ID: "exec-old", Status: string(biz.GraphExecRunning)},
	}

	w := newSweeper(repo, graphExec, nil)
	w.RunOnceExposed(context.Background())

	if len(repo.updates) != 1 {
		t.Fatalf("expected 1 update, got %d", len(repo.updates))
	}
	if repo.updates[0].status != biz.ChannelTurnJobStatusTimeout {
		t.Errorf("expected status %s (force timeout), got %s", biz.ChannelTurnJobStatusTimeout, repo.updates[0].status)
	}
}

// TestSweeper_GraphExecNil verifies that the sweeper does not panic when
// graphExec is nil (cron-only deployment).
func TestSweeper_GraphExecNil(t *testing.T) {
	job := staleJob("job-nil-graph", biz.ChannelTurnJobStatusAsyncQueued, "graph", "exec-1", 5*time.Minute)
	repo := newSweeperTestRepo(job)
	repo.listStaleFn = func(status, _ string) []biz.ChannelTurnJob {
		if status != biz.ChannelTurnJobStatusAsyncQueued {
			return nil
		}
		return []biz.ChannelTurnJob{job}
	}

	w := newSweeper(repo, nil, nil)
	// Should not panic; job stays stuck (no recovery for graph target without graphExec)
	w.RunOnceExposed(context.Background())

	// No update because graphExec is nil and the job hasn't exceeded max age
	if len(repo.updates) != 0 {
		t.Fatalf("expected 0 updates (graphExec nil, job not max age), got %d", len(repo.updates))
	}
}
