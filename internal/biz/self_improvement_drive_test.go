package biz

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── drive-usecase fakes ──────────────────────────────────────────────────────

type siDrivePipelineFake struct {
	mu      sync.Mutex
	calls   []string
	err     error
	entered chan string // 非 nil 时 Execute 阻塞至 release 关闭（活跃集测试）
	release chan struct{}
}

func (p *siDrivePipelineFake) Execute(_ context.Context, runID string) error {
	p.mu.Lock()
	p.calls = append(p.calls, runID)
	p.mu.Unlock()
	if p.entered != nil {
		p.entered <- runID
		<-p.release
	}
	return p.err
}

func (p *siDrivePipelineFake) callCount() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.calls)
}

type siDriveRouterFake struct {
	mu      sync.Mutex
	calls   []string
	channel string
	err     error
}

func (r *siDriveRouterFake) Route(_ context.Context, runID string) (string, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls = append(r.calls, runID)
	if r.err != nil {
		return "", r.err
	}
	if r.channel == "" {
		return "auto", nil
	}
	return r.channel, nil
}

func (r *siDriveRouterFake) callCount() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.calls)
}

type siDriveApplyFake struct {
	mu           sync.Mutex
	applyCalls   []string
	promoteCalls int
	err          error
}

func (a *siDriveApplyFake) Apply(_ context.Context, runID string) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.applyCalls = append(a.applyCalls, runID)
	return a.err
}

func (a *siDriveApplyFake) PromoteEligible(_ context.Context) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.promoteCalls++
	return nil
}

func (a *siDriveApplyFake) applyCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.applyCalls)
}

// ── helpers ──────────────────────────────────────────────────────────────────

func siDriveRun(id string, status SelfImprovementRunStatus) SelfImprovementRun {
	return SelfImprovementRun{
		ID: id, SuggestionID: "sug-" + id, Status: status,
		TriggerSource: TriggerSourceErrorCluster,
		UpdatedAt:     time.Now(),
	}
}

func siDriveFixture(t *testing.T, runs []SelfImprovementRun, mutate func(*SelfImprovementDriveDeps)) (*SelfImprovementDriveUsecase, *siRunStore, *siDrivePipelineFake, *siDriveRouterFake, *siDriveApplyFake) {
	t.Helper()
	store := &siRunStore{others: runs}
	pipe := &siDrivePipelineFake{}
	router := &siDriveRouterFake{}
	apply := &siDriveApplyFake{}
	deps := SelfImprovementDriveDeps{
		RunReader: store, RunWriter: store,
		Pipeline: pipe, Router: router, Applier: apply,
		Lg: loggateway.NewNoop(),
	}
	if mutate != nil {
		mutate(&deps)
	}
	uc, err := NewSelfImprovementDriveUsecase(deps)
	if err != nil {
		t.Fatalf("NewSelfImprovementDriveUsecase: %v", err)
	}
	return uc, store, pipe, router, apply
}

// siDriveEventually polls cond until true or the deadline expires.
func siDriveEventually(t *testing.T, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("等待超时: %s", what)
}

// ── tests ────────────────────────────────────────────────────────────────────

func TestSIDrive_DetectedRunsPipeline(t *testing.T) {
	uc, _, pipe, _, apply := siDriveFixture(t, []SelfImprovementRun{siDriveRun("run-1", RunStatusDetected)}, nil)
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce: %v", err)
	}
	siDriveEventually(t, "pipeline 应被异步调用", func() bool { return pipe.callCount() == 1 })
	if apply.applyCount() != 0 {
		t.Fatalf("detected 不应触发 Apply")
	}
}

func TestSIDrive_AwaitingGovernanceRouted(t *testing.T) {
	uc, _, _, router, _ := siDriveFixture(t, []SelfImprovementRun{siDriveRun("run-1", RunStatusAwaitingGovernance)}, nil)
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce: %v", err)
	}
	if router.callCount() != 1 {
		t.Fatalf("awaiting_governance 应路由一次, 实际 %d", router.callCount())
	}
}

func TestSIDrive_ApprovalChannelSubmittedOnce(t *testing.T) {
	approvalRouter := &siDriveRouterFake{channel: "approval"}
	uc, _, _, _, _ := siDriveFixture(t, []SelfImprovementRun{siDriveRun("run-1", RunStatusAwaitingGovernance)},
		func(d *SelfImprovementDriveDeps) { d.Router = approvalRouter })
	// approval 通道的审批提交每 run 每进程只路由一次（防 per-tick 重复提交；
	// W6 适配器侧幂等兜底进程重启场景）。
	for i := 0; i < 3; i++ {
		if err := uc.DriveOnce(context.Background()); err != nil {
			t.Fatalf("DriveOnce #%d: %v", i, err)
		}
	}
	if n := approvalRouter.callCount(); n != 1 {
		t.Fatalf("approval 通道 3 tick 应只路由一次, 实际 %d", n)
	}
}

func TestSIDrive_ApplyingDriven(t *testing.T) {
	uc, _, _, _, apply := siDriveFixture(t, []SelfImprovementRun{siDriveRun("run-1", RunStatusApplying)}, nil)
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce: %v", err)
	}
	if apply.applyCount() != 1 {
		t.Fatalf("applying 应驱动 Apply 一次, 实际 %d", apply.applyCount())
	}
}

func TestSIDrive_StaleMidPipelineRecovered(t *testing.T) {
	stale := siDriveRun("run-1", RunStatusPatching)
	stale.UpdatedAt = time.Now().Add(-2 * time.Hour)
	uc, store, pipe, _, _ := siDriveFixture(t, []SelfImprovementRun{stale}, nil)

	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce: %v", err)
	}
	got, _ := store.GetByID(context.Background(), "run-1")
	if got.Status != RunStatusDetected {
		t.Fatalf("陈旧 patching 应 recover 回 detected, 实际 %s", got.Status)
	}
	if pipe.callCount() != 0 {
		t.Fatalf("recover 与执行不在同 tick")
	}
	// 下一 tick 重驱动。
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce #2: %v", err)
	}
	siDriveEventually(t, "recover 后应重驱动 pipeline", func() bool { return pipe.callCount() == 1 })
}

func TestSIDrive_FreshMidPipelineSkipped(t *testing.T) {
	fresh := siDriveRun("run-1", RunStatusVerifying) // UpdatedAt = now
	uc, store, pipe, _, _ := siDriveFixture(t, []SelfImprovementRun{fresh}, nil)
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce: %v", err)
	}
	got, _ := store.GetByID(context.Background(), "run-1")
	if got.Status != RunStatusVerifying {
		t.Fatalf("活跃中途态不应 recover, 实际 %s", got.Status)
	}
	if pipe.callCount() != 0 {
		t.Fatalf("活跃中途态不应触发 pipeline")
	}
}

func TestSIDrive_ActiveRunNotDoubleDriven(t *testing.T) {
	pipe := &siDrivePipelineFake{entered: make(chan string, 1), release: make(chan struct{})}
	uc, _, _, _, _ := siDriveFixture(t, []SelfImprovementRun{siDriveRun("run-1", RunStatusDetected)},
		func(d *SelfImprovementDriveDeps) { d.Pipeline = pipe })

	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce #1: %v", err)
	}
	select {
	case <-pipe.entered:
	case <-time.After(2 * time.Second):
		t.Fatal("pipeline 未进入执行")
	}
	// run-1 正在执行：第二 tick 不得重入。
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce #2: %v", err)
	}
	if n := pipe.callCount(); n != 1 {
		t.Fatalf("活跃 run 被重入, Execute 调用 %d 次", n)
	}
	close(pipe.release)
	siDriveEventually(t, "pipeline 退出后活跃集应释放", func() bool {
		if err := uc.DriveOnce(context.Background()); err != nil {
			t.Fatalf("DriveOnce #3: %v", err)
		}
		return pipe.callCount() == 2
	})
}

func TestSIDrive_PromoteEligibleEveryTick(t *testing.T) {
	uc, _, _, _, apply := siDriveFixture(t, nil, nil)
	for i := 0; i < 2; i++ {
		if err := uc.DriveOnce(context.Background()); err != nil {
			t.Fatalf("DriveOnce #%d: %v", i, err)
		}
	}
	apply.mu.Lock()
	defer apply.mu.Unlock()
	if apply.promoteCalls != 2 {
		t.Fatalf("每 tick 应调用 PromoteEligible, 实际 %d/2", apply.promoteCalls)
	}
}

func TestSIDrive_PausedRunNotAnError(t *testing.T) {
	uc, _, pipe, _, _ := siDriveFixture(t, []SelfImprovementRun{siDriveRun("run-1", RunStatusDetected)},
		func(d *SelfImprovementDriveDeps) {
			d.Pipeline = &siDrivePipelineFake{err: ErrSIRunPaused}
		})
	_ = pipe
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("pause 不应作为错误传播: %v", err)
	}
}

func TestSIDrive_TerminalRunsIgnored(t *testing.T) {
	runs := []SelfImprovementRun{
		siDriveRun("run-closed", RunStatusClosed),
		siDriveRun("run-failed", RunStatusFailed),
		siDriveRun("run-rejected", RunStatusRejected),
		siDriveRun("run-rolled", RunStatusRolledBack),
		siDriveRun("run-vfailed", RunStatusVerifyFailed),
	}
	uc, _, pipe, router, apply := siDriveFixture(t, runs, nil)
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("DriveOnce: %v", err)
	}
	if pipe.callCount()+router.callCount()+apply.applyCount() != 0 {
		t.Fatalf("终态 run 不应被驱动: pipe=%d router=%d apply=%d",
			pipe.callCount(), router.callCount(), apply.applyCount())
	}
}

func TestSIDrive_ConflictMeansSomeoneElseDriving(t *testing.T) {
	// 路由/应用返回 Conflict（run 已被其他入口推进）→ 静默跳过，不报错。
	uc, _, _, _, _ := siDriveFixture(t,
		[]SelfImprovementRun{
			siDriveRun("run-1", RunStatusAwaitingGovernance),
			siDriveRun("run-2", RunStatusApplying),
		},
		func(d *SelfImprovementDriveDeps) {
			d.Router = &siDriveRouterFake{err: apierror.Conflict("SELF_IMPROVEMENT", "moved")}
			d.Applier = &siDriveApplyFake{err: apierror.Conflict("SELF_IMPROVEMENT", "moved")}
		})
	if err := uc.DriveOnce(context.Background()); err != nil {
		t.Fatalf("Conflict 应静默跳过: %v", err)
	}
}

func TestSIDrive_ConstructorGuards(t *testing.T) {
	if _, err := NewSelfImprovementDriveUsecase(SelfImprovementDriveDeps{}); err == nil {
		t.Fatal("缺依赖应报错")
	}
	store := &siRunStore{}
	if _, err := NewSelfImprovementDriveUsecase(SelfImprovementDriveDeps{
		RunReader: store, RunWriter: store, Lg: loggateway.NewNoop(),
	}); err == nil {
		t.Fatal("缺 Applier 应报错（PromoteEligible 为 D10 强制）")
	}
	// Pipeline/Router 可 nil（降级：对应阶段跳过）。
	if _, err := NewSelfImprovementDriveUsecase(SelfImprovementDriveDeps{
		RunReader: store, RunWriter: store, Applier: &siDriveApplyFake{}, Lg: loggateway.NewNoop(),
	}); err != nil {
		t.Fatalf("Pipeline/Router 可 nil 降级: %v", err)
	}
}
