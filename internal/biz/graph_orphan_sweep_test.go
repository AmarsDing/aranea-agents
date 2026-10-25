package biz

import (
	"context"
	"strings"
	"testing"
)

// ---------------------------------------------------------------------------
// D6-F4（87-orchestration-chain-hardening 批次 A3）：standalone 孤儿执行
// 启动终态化扫描（biz 窄端口 GraphOrphanRunScanner + FinalizeOrphanedStandaloneExecutions）。
// ---------------------------------------------------------------------------

// fakeOrphanRunRepo 同时实现 GraphRunRepo 与 GraphOrphanRunScanner 窄接口。
// runs 为行存储（GetRun/UpdateRun 直接操作同一对象，模拟真实 repo 的
// 行级持久化）；orphanIDs 模拟 raw SQL 的 NOT IN team_graph_sessions 过滤
// 结果集（team 行排除语义由 internal/data 的 SQL 用例覆盖）。
type fakeOrphanRunRepo struct {
	runs       map[string]*GraphExecution
	orphanIDs  []string
	updateRuns int
	scans      int
}

var _ GraphRunRepo = (*fakeOrphanRunRepo)(nil)
var _ GraphOrphanRunScanner = (*fakeOrphanRunRepo)(nil)

func (r *fakeOrphanRunRepo) SaveRun(context.Context, *GraphExecution) error { return nil }

func (r *fakeOrphanRunRepo) GetRun(_ context.Context, id string) (*GraphExecution, error) {
	if exec, ok := r.runs[id]; ok {
		return exec, nil
	}
	return nil, ErrNotFound
}

func (r *fakeOrphanRunRepo) ListRunsByGraph(context.Context, string, int, string, ...GraphRunListOption) ([]*GraphExecution, string, error) {
	return nil, "", nil
}

func (r *fakeOrphanRunRepo) UpdateRun(_ context.Context, exec *GraphExecution) error {
	r.updateRuns++
	if stored, ok := r.runs[exec.ID]; ok && stored != exec {
		*stored = *exec
	}
	return nil
}

func (r *fakeOrphanRunRepo) ListOrphanedStandaloneRuns(_ context.Context, status string, limit int) ([]*GraphExecution, error) {
	r.scans++
	status = strings.TrimSpace(status)
	var out []*GraphExecution
	for _, id := range r.orphanIDs {
		exec, ok := r.runs[id]
		if !ok || exec.Status != status {
			continue
		}
		out = append(out, exec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, nil
}

func newOrphanRun(id, status string) *GraphExecution {
	return NewGraphExecution(context.Background(), id, "graph-orphan", "sess-orphan", status)
}

// F4: 混合行集中仅 running-standalone 被收敛为 failed 且 UpdateRun 恰好一次；
// waiting_human / completed（非 running）一律不动。
func TestFinalizeOrphanedStandaloneExecutions_SkipsTeamAndNonRunning(t *testing.T) {
	repo := &fakeOrphanRunRepo{runs: map[string]*GraphExecution{
		"run-standalone": newOrphanRun("run-standalone", string(GraphExecRunning)),
		"run-waiting":    newOrphanRun("run-waiting", string(GraphExecWaitingHuman)),
		"run-completed":  newOrphanRun("run-completed", string(GraphExecCompleted)),
	}, orphanIDs: []string{"run-standalone", "run-waiting", "run-completed"}}
	uc := newTestGraphExecUsecase(repo)

	n, err := uc.FinalizeOrphanedStandaloneExecutions(context.Background())
	if err != nil {
		t.Fatalf("FinalizeOrphanedStandaloneExecutions: %v", err)
	}
	if n != 1 {
		t.Fatalf("仅 run-standalone 应被终态化: finalized=%d, want 1", n)
	}
	if got := repo.runs["run-standalone"].GetStatus(); got != string(GraphExecFailed) {
		t.Fatalf("run-standalone 状态 = %q, want failed", got)
	}
	if repo.updateRuns != 1 {
		t.Fatalf("UpdateRun 恰好一次: got %d", repo.updateRuns)
	}
	if got := repo.runs["run-standalone"].ErrorMessage; got != "server restart: orphaned graph execution finalized" {
		t.Fatalf("终态化原因 = %q", got)
	}
	if got := repo.runs["run-waiting"].GetStatus(); got != string(GraphExecWaitingHuman) {
		t.Fatalf("waiting_human 一律不动, got %q", got)
	}
	if got := repo.runs["run-completed"].GetStatus(); got != string(GraphExecCompleted) {
		t.Fatalf("completed 一律不动, got %q", got)
	}
}

// F4: 二次启动/多副本交叠幂等——首轮已终态化的行在第二轮被 FSM 终态守卫
// 跳过（终态守卫幂等），不再 UpdateRun。
func TestFinalizeOrphanedStandaloneExecutions_IdempotentSecondRunNoOp(t *testing.T) {
	repo := &fakeOrphanRunRepo{runs: map[string]*GraphExecution{
		"run-standalone": newOrphanRun("run-standalone", string(GraphExecRunning)),
	}, orphanIDs: []string{"run-standalone"}}
	uc := newTestGraphExecUsecase(repo)
	ctx := context.Background()

	n1, err := uc.FinalizeOrphanedStandaloneExecutions(ctx)
	if err != nil {
		t.Fatalf("first sweep: %v", err)
	}
	if n1 != 1 || repo.updateRuns != 1 {
		t.Fatalf("first sweep: finalized=%d updateRuns=%d, want 1/1", n1, repo.updateRuns)
	}

	n2, err := uc.FinalizeOrphanedStandaloneExecutions(ctx)
	if err != nil {
		t.Fatalf("second sweep: %v", err)
	}
	if repo.updateRuns != 1 {
		t.Fatalf("second sweep 幂等空转: updateRuns=%d, want 1", repo.updateRuns)
	}
	_ = n2 // 第二轮扫描返回 0 条 running 行（首轮已 failed），不做硬性断言 n2==0 的行数语义
}

// F4: runRepo nil 或未实现 GraphOrphanRunScanner 窄接口 → (0, nil) 幂等空转，
// 不影响启动。
func TestFinalizeOrphanedStandaloneExecutions_NilRepoOrNoScannerNoOp(t *testing.T) {
	// nil repo
	ucNil := newTestGraphExecUsecase(nil)
	if n, err := ucNil.FinalizeOrphanedStandaloneExecutions(context.Background()); n != 0 || err != nil {
		t.Fatalf("nil repo: got (%d, %v), want (0, nil)", n, err)
	}
	// 未实现 scanner 的存量 fake（GraphRunRepo 大接口不受影响）
	ucNoScanner := newTestGraphExecUsecase(&fakeGraphRunRepo{})
	if n, err := ucNoScanner.FinalizeOrphanedStandaloneExecutions(context.Background()); n != 0 || err != nil {
		t.Fatalf("non-scanner repo: got (%d, %v), want (0, nil)", n, err)
	}
}
