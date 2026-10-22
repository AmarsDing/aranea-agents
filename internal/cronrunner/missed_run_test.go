package cronrunner

import (
	"context"
	"encoding/json"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// countingSyncAgent 统计 dispatch 真实发生次数（model_registry_sync 目标不需要 Session/Chat，利于断言「跳过=零分发」）。
type countingSyncAgent struct {
	calls int
	err   error
}

func (c *countingSyncAgent) RunSync(context.Context) error {
	c.calls++
	return c.err
}

// missedFakeRepo 内存版 CronRepo：支持 ListCronTaskRuns 按 task_id 过滤+最新在前，
// 供 skipped/skipped_missed 折叠路径测试使用。
type missedFakeRepo struct {
	mu    sync.Mutex
	tasks map[string]biz.CronTask
	runs  []biz.CronTaskRun // append 序 = 创建序；列表按最新在前返回
}

func newMissedFakeRepo() *missedFakeRepo {
	return &missedFakeRepo{tasks: map[string]biz.CronTask{}}
}

func (m *missedFakeRepo) ListCronTasks(context.Context) ([]biz.CronTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([]biz.CronTask, 0, len(m.tasks))
	for _, t := range m.tasks {
		out = append(out, t)
	}
	return out, nil
}
func (m *missedFakeRepo) GetCronTask(_ context.Context, id string) (biz.CronTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	t, ok := m.tasks[id]
	if !ok {
		return biz.CronTask{}, biz.ErrCronTaskDeleted
	}
	return t, nil
}
func (m *missedFakeRepo) CreateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[t.ID] = t
	return t, nil
}
func (m *missedFakeRepo) UpdateCronTask(_ context.Context, t biz.CronTask) (biz.CronTask, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.tasks[t.ID] = t
	return t, nil
}
func (m *missedFakeRepo) DeleteCronTask(context.Context, string) error { return nil }
func (m *missedFakeRepo) GetCronTaskRun(_ context.Context, id string) (biz.CronTaskRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, r := range m.runs {
		if r.ID == id {
			return r, nil
		}
	}
	return biz.CronTaskRun{}, nil
}
func (m *missedFakeRepo) ListCronTaskRuns(_ context.Context, q biz.CronTaskRunQuery) ([]biz.CronTaskRun, int, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var out []biz.CronTaskRun
	for i := len(m.runs) - 1; i >= 0; i-- { // 最新在前
		r := m.runs[i]
		if q.TaskID != "" && r.TaskID != q.TaskID {
			continue
		}
		if q.Status != "" && r.Status != q.Status {
			continue
		}
		out = append(out, r)
		if q.Limit > 0 && len(out) >= q.Limit {
			break
		}
	}
	return out, len(out), nil
}
func (m *missedFakeRepo) InsertCronTaskRun(_ context.Context, in biz.CronTaskRunInput) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs = append(m.runs, biz.CronTaskRun{
		ID: in.ID, TaskID: in.TaskID, Status: in.Status, StartedAt: in.StartedAt,
		OutputJSON: in.OutputJSON, CreatedAt: in.CreatedAt,
	})
	return nil
}
func (m *missedFakeRepo) UpdateCronTaskRun(_ context.Context, id, status, finishedAt, outputJSON, errorMessage string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.runs {
		if m.runs[i].ID == id {
			m.runs[i].Status = status
			m.runs[i].FinishedAt = finishedAt
			m.runs[i].OutputJSON = outputJSON
			m.runs[i].ErrorMessage = errorMessage
		}
	}
	return nil
}

func missedTaskMeta(t *testing.T, meta cronTaskMetadata) string {
	t.Helper()
	raw, err := json.Marshal(meta)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func readMissedMeta(t *testing.T, repo *missedFakeRepo, taskID string) map[string]any {
	t.Helper()
	repo.mu.Lock()
	raw := repo.tasks[taskID].MetadataJSON
	repo.mu.Unlock()
	var out map[string]any
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		t.Fatalf("task %s metadata unmarshal: %v", taskID, err)
	}
	return out
}

func intPtr(v int) *int { return &v }

func TestMissedBeyondGrace_Matrix(t *testing.T) {
	now := time.Date(2026, 10, 22, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name     string
		graceSec *int
		late     time.Duration
		tick     time.Duration
		beyond   bool
	}{
		{"未配置=特性关闭一律补跑", nil, 24 * time.Hour, time.Minute, false},
		{"宽限内仍补跑", intPtr(300), 6 * time.Minute, time.Minute, false},
		{"超宽限跳过", intPtr(300), 8 * time.Minute, time.Minute, true},
		{"显式0宽限:超过2tick即跳过", intPtr(0), 3 * time.Minute, time.Minute, true},
		{"显式0宽限:2tick内仍补跑", intPtr(0), 2 * time.Minute, time.Minute, false},
		{"负宽限按0处理", intPtr(-5), 3 * time.Minute, time.Minute, true},
		{"未到期不判滞后", intPtr(300), -time.Minute, time.Minute, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cfg := cronTaskConfig{MissedRunGraceSeconds: tc.graceSec}
			_, beyond := missedBeyondGrace(cfg, now.Add(-tc.late), now, tc.tick)
			if beyond != tc.beyond {
				t.Fatalf("grace=%v late=%v: expected beyond=%v, got %v", tc.graceSec, tc.late, tc.beyond, beyond)
			}
		})
	}
}

// H10 核心路径：宕机后到期时刻已超宽限 → 记 skipped_missed 并翻页，零分发。
func TestRunDue_SkipsMissedBeyondGrace(t *testing.T) {
	repo := newMissedFakeRepo()
	syncAgent := &countingSyncAgent{}
	now := time.Now().UTC()
	dueAt := now.Add(-10 * time.Minute)
	repo.tasks["t1"] = biz.CronTask{
		ID: "t1", TaskKey: "job", Name: "Job", Status: "active", Enabled: true,
		ConfigJSON: `{"target_type":"model_registry_sync","message":"sync","schedule_type":"interval","interval_seconds":60,"missed_run_grace_seconds":300}`,
		MetadataJSON: missedTaskMeta(t, cronTaskMetadata{
			NextRunAt: dueAt.Format(time.RFC3339),
		}),
	}
	r := NewRunner(Deps{Cron: repo, RegistrySyncAgent: syncAgent}, loggateway.NewNoop()).WithTickInterval(time.Minute)

	r.runDue(context.Background())

	if syncAgent.calls != 0 {
		t.Fatalf("missed run must not dispatch, got %d dispatches", syncAgent.calls)
	}
	if len(repo.runs) != 1 {
		t.Fatalf("expected 1 cron_task_run, got %d", len(repo.runs))
	}
	run := repo.runs[0]
	if run.Status != cronRunStatusSkippedMissed {
		t.Fatalf("expected status %s, got %s", cronRunStatusSkippedMissed, run.Status)
	}
	if run.ErrorMessage != errCronSkippedMissed.Error() {
		t.Fatalf("unexpected skip reason: %q", run.ErrorMessage)
	}
	meta := readMissedMeta(t, repo, "t1")
	if meta["skip_count"].(float64) != 1 {
		t.Fatalf("expected skip_count=1, got %v", meta["skip_count"])
	}
	if v := meta["failure_count"]; v != nil && v.(float64) != 0 {
		t.Fatalf("skipped_missed must not increment failure_count, got %v", v)
	}
	if meta["last_run_status"] != cronRunStatusSkippedMissed {
		t.Fatalf("expected last_run_status=%s, got %v", cronRunStatusSkippedMissed, meta["last_run_status"])
	}
	next, err := time.Parse(time.RFC3339, meta["next_run_at"].(string))
	if err != nil || !next.After(now) {
		t.Fatalf("expected next_run_at advanced into the future, got %v", meta["next_run_at"])
	}
}

// 宽限内的滞后期照常补跑（正向证据：宽限只挡「过度滞后」，不挡「迟到一点」）。
func TestRunDue_MissedWithinGraceExecutes(t *testing.T) {
	repo := newMissedFakeRepo()
	syncAgent := &countingSyncAgent{}
	now := time.Now().UTC()
	repo.tasks["t1"] = biz.CronTask{
		ID: "t1", TaskKey: "job", Name: "Job", Status: "active", Enabled: true,
		ConfigJSON: `{"target_type":"model_registry_sync","message":"sync","schedule_type":"interval","interval_seconds":60,"missed_run_grace_seconds":300}`,
		MetadataJSON: missedTaskMeta(t, cronTaskMetadata{
			NextRunAt: now.Add(-time.Minute).Format(time.RFC3339),
		}),
	}
	r := NewRunner(Deps{Cron: repo, RegistrySyncAgent: syncAgent}, loggateway.NewNoop()).WithTickInterval(time.Minute)

	r.runDue(context.Background())

	if syncAgent.calls != 1 {
		t.Fatalf("within-grace run must dispatch exactly once, got %d", syncAgent.calls)
	}
	if len(repo.runs) != 1 || repo.runs[0].Status != "success" {
		t.Fatalf("expected 1 success run, got %+v", repo.runs)
	}
}

// once 任务豁免宽限判定：一次性任务错过窗口仍应补跑。
func TestRunDue_OnceTaskExemptFromGrace(t *testing.T) {
	repo := newMissedFakeRepo()
	syncAgent := &countingSyncAgent{}
	now := time.Now().UTC()
	repo.tasks["t1"] = biz.CronTask{
		ID: "t1", TaskKey: "job", Name: "Job", Status: "active", Enabled: true,
		ConfigJSON: `{"target_type":"model_registry_sync","message":"sync","schedule_type":"once","run_at":"` +
			now.Add(-30*time.Minute).Format(time.RFC3339) + `","missed_run_grace_seconds":0}`,
	}
	r := NewRunner(Deps{Cron: repo, RegistrySyncAgent: syncAgent}, loggateway.NewNoop()).WithTickInterval(time.Minute)

	r.runDue(context.Background())

	if syncAgent.calls != 1 {
		t.Fatalf("once task must execute even when overdue, got %d dispatches", syncAgent.calls)
	}
	if len(repo.runs) != 1 || repo.runs[0].Status != "success" {
		t.Fatalf("expected 1 success run, got %+v", repo.runs)
	}
}

// 未配置 grace 时保持历史行为：滞后多久都补跑。
func TestRunDue_GraceAbsentCatchesUp(t *testing.T) {
	repo := newMissedFakeRepo()
	syncAgent := &countingSyncAgent{}
	now := time.Now().UTC()
	repo.tasks["t1"] = biz.CronTask{
		ID: "t1", TaskKey: "job", Name: "Job", Status: "active", Enabled: true,
		ConfigJSON: `{"target_type":"model_registry_sync","message":"sync","schedule_type":"interval","interval_seconds":60}`,
		MetadataJSON: missedTaskMeta(t, cronTaskMetadata{
			NextRunAt: now.Add(-10 * time.Minute).Format(time.RFC3339),
		}),
	}
	r := NewRunner(Deps{Cron: repo, RegistrySyncAgent: syncAgent}, loggateway.NewNoop()).WithTickInterval(time.Minute)

	r.runDue(context.Background())

	if syncAgent.calls != 1 {
		t.Fatalf("legacy behavior must catch up when grace unset, got %d dispatches", syncAgent.calls)
	}
}

// H11 折叠纪律：连续同字节原因折叠进同一行（fold_count 递增），原因漂移促生新行。
func TestRecordPreExecuteOutcome_FoldsByteIdenticalSkips(t *testing.T) {
	repo := newMissedFakeRepo()
	repo.tasks["t1"] = biz.CronTask{
		ID: "t1", TaskKey: "job", Name: "Job", Status: "active", Enabled: true,
		ConfigJSON: `{"target_type":"model_registry_sync","message":"","schedule_type":"interval","interval_seconds":60}`,
	}
	r := NewRunner(Deps{Cron: repo}, loggateway.NewNoop())
	cfg := cronTaskConfig{ScheduleType: "interval", IntervalSeconds: 60}
	now := time.Now().UTC()

	for i := 0; i < 3; i++ {
		r.recordPreExecuteOutcome(context.Background(), repo.tasks["t1"], cfg, now, "skipped", "cron message is required")
	}
	if len(repo.runs) != 1 {
		t.Fatalf("byte-identical skips must fold into one row, got %d rows", len(repo.runs))
	}
	var out struct {
		FoldCount int `json:"fold_count"`
	}
	if err := json.Unmarshal([]byte(repo.runs[0].OutputJSON), &out); err != nil {
		t.Fatal(err)
	}
	if out.FoldCount != 3 {
		t.Fatalf("expected fold_count=3, got %d", out.FoldCount)
	}

	r.recordPreExecuteOutcome(context.Background(), repo.tasks["t1"], cfg, now, "skipped", "another reason")
	if len(repo.runs) != 2 {
		t.Fatalf("reason drift must start a new row, got %d rows", len(repo.runs))
	}
	meta := readMissedMeta(t, repo, "t1")
	if meta["skip_count"].(float64) != 4 {
		t.Fatalf("expected skip_count=4 across folded occurrences, got %v", meta["skip_count"])
	}
}

// H11 回归守卫：配置不可求值每 tick 触发一次，折叠+不致死（旧行为 3 tick 即 dead）。
func TestRunDue_UnevaluableConfigFoldsAndNeverDies(t *testing.T) {
	repo := newMissedFakeRepo()
	repo.tasks["t1"] = biz.CronTask{
		ID: "t1", TaskKey: "job", Name: "Job", Status: "active", Enabled: true,
		ConfigJSON: `{"target_type":"model_registry_sync","message":"","schedule_type":"interval","interval_seconds":60}`,
	}
	r := NewRunner(Deps{Cron: repo}, loggateway.NewNoop()).WithTickInterval(time.Minute)

	for i := 0; i < maxDeadFailures+2; i++ {
		r.runDue(context.Background())
	}

	if len(repo.runs) != 1 {
		t.Fatalf("unevaluable config must fold across ticks, got %d rows", len(repo.runs))
	}
	var out struct {
		FoldCount int `json:"fold_count"`
	}
	if err := json.Unmarshal([]byte(repo.runs[0].OutputJSON), &out); err != nil {
		t.Fatal(err)
	}
	if out.FoldCount != maxDeadFailures+2 {
		t.Fatalf("expected fold_count=%d, got %d", maxDeadFailures+2, out.FoldCount)
	}
	repo.mu.Lock()
	task := repo.tasks["t1"]
	repo.mu.Unlock()
	if task.Status == "dead" || !task.Enabled {
		t.Fatalf("unevaluable config must wait in place for fix, got status=%s enabled=%v", task.Status, task.Enabled)
	}
	meta := readMissedMeta(t, repo, "t1")
	if meta["failure_count"] != nil && meta["failure_count"].(float64) != 0 {
		t.Fatalf("unevaluable must not increment failure_count, got %v", meta["failure_count"])
	}
}
