package service

import (
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
)

// plan_executor_lease_port_test.go —— 对抗评审 #7/#2/#12：BoardRunLeasePort
// 接线后 PlanExecutor 的四项边界。分批 C Q4 时 24 项评审挂的零覆盖缺口。

// scriptedLeasePort 脚本化端口：认领放行与否、释放记录、session 活跃度
// 全部钉死可断言。
type scriptedLeasePort struct {
	mu         sync.Mutex
	claimAllow bool
	claims     []string
	releases   []string
	active     map[string]bool
	onRelease  func(boardID string)
}

func (p *scriptedLeasePort) TryClaim(_ context.Context, lease biz.BoardRunLease) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.claims = append(p.claims, lease.BoardID)
	return p.claimAllow
}

func (p *scriptedLeasePort) Renew(_ context.Context, _ string) bool { return true }

func (p *scriptedLeasePort) Release(boardID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.releases = append(p.releases, boardID)
	if p.onRelease != nil {
		p.onRelease(boardID)
	}
}

func (p *scriptedLeasePort) HasActiveForSession(sessionID string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.active[sessionID]
}

func (p *scriptedLeasePort) releasedCount(id string) int {
	p.mu.Lock()
	defer p.mu.Unlock()
	n := 0
	for _, r := range p.releases {
		if r == id {
			n++
		}
	}
	return n
}

type scriptedCompletionNotifier struct {
	onNotify func(ctx context.Context, spiritSessionID string)
}

func (n *scriptedCompletionNotifier) NotifyAllTeamsCompleted(ctx context.Context, spiritSessionID string) {
	if n.onNotify != nil {
		n.onNotify(ctx, spiritSessionID)
	}
}

func leaseTestBoard(id string) biz.PlanBoard {
	return biz.PlanBoard{
		ID:        id,
		TaskID:    "task-" + id,
		SessionID: "sess-" + id,
		Status:    biz.PlanStatusPlanning,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: id, TaskID: "task-" + id, Label: "step1", Status: biz.PlanStepStatusPending, Version: 1},
		},
	}
}

// #7(a)：端口认领被拒 → 本地 running 不登记、不派发、cancel 不泄漏
// （拿不到跨副本租约绝不能动——fail-closed 矩阵起点）。
func TestPlanExecutorLeasePort_ClaimDeniedSkipsBoard(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	port := &scriptedLeasePort{claimAllow: false}
	pe.SetBoardRunLeasePort(port)

	board := leaseTestBoard("board-deny")
	pe.startBoardDAG(board)

	port.mu.Lock()
	claims := len(port.claims)
	port.mu.Unlock()
	if claims != 1 {
		t.Fatalf("TryClaim calls = %d, want 1", claims)
	}
	if _, ok := pe.running.Load(board.ID); ok {
		t.Fatal("denied claim must not register local running entry")
	}
	if orch.waitForCall("s1", 150*time.Millisecond) {
		t.Fatal("denied claim must not dispatch any step")
	}
	if pe.HasActiveRunForSession("sess-board-deny") {
		t.Fatal("denied claim must not read as active")
	}
}

// #7(b)：本地落册撞车（已在执行）→ 端口认领必须回滚 Release，存量 lease
// 行保持原位（先入者不受惊动）。
func TestPlanExecutorLeasePort_LocalCollisionRollsBackClaim(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	port := &scriptedLeasePort{claimAllow: true}
	pe.SetBoardRunLeasePort(port)

	existing := &boardRunLease{cancel: func() {}, sessionID: "sess-first"}
	pe.running.Store("board-dup", existing)

	pe.startBoardDAG(leaseTestBoard("board-dup"))

	if got := port.releasedCount("board-dup"); got != 1 {
		t.Fatalf("rollback releases = %d, want 1", got)
	}
	if got, _ := pe.running.Load("board-dup"); got != existing {
		t.Fatal("pre-existing lease must stay untouched on collision")
	}
	if orch.waitForCall("s1", 150*time.Millisecond) {
		t.Fatal("collision path must not dispatch")
	}
}

// #7(c)：run 退出（steps 全部终态）→ 延迟释放调用端口 Release（lease-aware
// run 的端口/本地行成对生命周期，final 经 lease 推出方才能被其他副本收养）。
func TestPlanExecutorLeasePort_RunExitReleasesPort(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	port := &scriptedLeasePort{claimAllow: true}
	pe.SetBoardRunLeasePort(port)
	bus := event.NewV2Bus()
	pe.SetEventBus(bus)
	pe.StartSubscription()
	t.Cleanup(pe.Stop)
	time.Sleep(20 * time.Millisecond)

	board := leaseTestBoard("board-exit")
	bus.Publish(context.Background(), biz.NewPlanBoardCreatedEvent(board))
	if !orch.waitForCall("s1", 2*time.Second) {
		t.Fatal("expected dispatch")
	}
	orch.completeStep("s1", true, "")

	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if port.releasedCount("board-exit") >= 1 {
			_, running := pe.running.Load("board-exit")
			if !running {
				return // 端口释放落账 + 本地行清理，成对语义钉死（#2 后释放可多于一次，幂等）
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("port release not called on run exit (releases=%v)", port.releases)
}

// #7(d)：HasActiveRunForSession 放端口决议；nil 端口回落进程内经
// boardRunRegistry 行（字节级旧行为对照组）。
func TestPlanExecutorLeasePort_HasActiveDelegatesToPort(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)

	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	port := &scriptedLeasePort{claimAllow: true, active: map[string]bool{"sess-x": true}}
	pe.SetBoardRunLeasePort(port)
	if !pe.HasActiveRunForSession("sess-x") {
		t.Fatal("port-backed HasActiveForSession(true) must win")
	}
	if pe.HasActiveRunForSession("sess-y") {
		t.Fatal("port-backed HasActiveForSession(false) must win")
	}

	// nil 端口：纯进程内 Range 旧行为。
	peLocal := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	peLocal.running.Store("b-local", &boardRunLease{cancel: func() {}, sessionID: "sess-local"})
	if !peLocal.HasActiveRunForSession("sess-local") {
		t.Fatal("nil-port local registry lookup must find in-flight lease")
	}
	if peLocal.HasActiveRunForSession("sess-other") {
		t.Fatal("nil-port local registry must not cross-report")
	}
}

// #2（H 级）：releaseLeaseAndNotifyCompletion 必须先端口 Release 再
// completionNotifier——否则 final synthesis 触发方读 HasActive 仍 true、
// 被误拦成波次中点（本次批 C 原实现丢了这个次序——dag_run.go 的既有
// 「先释放再通知」不变量在 publish 路径被绕开）。
func TestPlanExecutorLeasePort_ReleaseBeforeNotifyCompletion(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())

	var mu sync.Mutex
	var order []string
	port := &scriptedLeasePort{claimAllow: true, onRelease: func(boardID string) {
		mu.Lock()
		order = append(order, "release:"+boardID)
		mu.Unlock()
	}}
	pe.SetBoardRunLeasePort(port)
	pe.SetCompletionNotifier(&scriptedCompletionNotifier{onNotify: func(_ context.Context, sid string) {
		mu.Lock()
		order = append(order, "notify:"+sid)
		mu.Unlock()
	}})
	pe.running.Store("board-ord", &boardRunLease{cancel: func() {}, sessionID: "sess-ord"})

	r := &dagRun{pe: pe, board: biz.PlanBoard{ID: "board-ord", SessionID: "sess-ord"}}
	r.releaseLeaseAndNotifyCompletion()

	mu.Lock()
	got := append([]string(nil), order...)
	mu.Unlock()
	want := []string{"release:board-ord", "notify:sess-ord"}
	if len(got) != len(want) {
		t.Fatalf("order = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order = %v, want %v", got, want)
		}
	}
	if _, ok := pe.running.Load("board-ord"); ok {
		t.Fatal("running entry must be deleted before notify")
	}
}

// #12（M 级）：resume 必须在做任何共享态变更前完成跨副本认领；认领被拒
// → 原样返回错误且 board/steps 零改动（否则本副本改完态后被拒 = 跨副本
// 楔死：board 停 Executing 无人跑）。
func TestResumePlanBoard_ClaimDeniedLeavesEverythingUntouched(t *testing.T) {
	repos := newFakeReposForExecutor()
	seq := &fakeSeq{repos: repos}
	orch := newFakeOrchestrator().withSeq(seq)
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	port := &scriptedLeasePort{claimAllow: false}
	pe.SetBoardRunLeasePort(port)

	ctx := context.Background()
	board := biz.PlanBoard{
		ID: "board-wedge", TaskID: "task-wedge", SessionID: "sess-wedge",
		Status: biz.PlanStatusFailed, Version: 3,
		Steps: []biz.PlanStep{
			{ID: "s1", PlanID: "board-wedge", TaskID: "task-wedge", Label: "step1", Status: biz.PlanStepStatusFailed, Version: 2},
		},
	}
	if _, err := repos.UpsertPlanBoard(ctx, board); err != nil {
		t.Fatalf("seed board: %v", err)
	}
	if _, err := repos.UpsertPlanStep(ctx, board.Steps[0]); err != nil {
		t.Fatalf("seed step: %v", err)
	}

	err := pe.ResumePlanBoard(ctx, "board-wedge", ResumeModeRetry)
	if err == nil {
		t.Fatal("claim-denied resume must error")
	}
	if port.releasedCount("board-wedge") != 0 {
		t.Fatal("denied claim must not trigger release")
	}
	stored, gerr := repos.GetPlanBoard(ctx, "board-wedge")
	if gerr != nil {
		t.Fatalf("board must survive: %v", gerr)
	}
	if stored.Status != biz.PlanStatusFailed || stored.Version != 3 {
		t.Fatalf("board mutated on denied resume: status=%s version=%d", stored.Status, stored.Version)
	}
	step, serr := repos.GetPlanStep(ctx, "s1")
	if serr != nil || step.Status != biz.PlanStepStatusFailed {
		t.Fatalf("step mutated on denied resume: %v %v", step.Status, serr)
	}
	if _, ok := pe.running.Load("board-wedge"); ok {
		t.Fatal("denied resume must not register running entry")
	}
	if !strings.Contains(err.Error(), "租约") && !strings.Contains(err.Error(), "lease") {
		t.Fatalf("error must name the lease cause: %v", err)
	}
}
