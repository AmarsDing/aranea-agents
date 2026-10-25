package service

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// fakeTeamRunReader 是 biz.TeamRunV2Reader 的进程内桩（D6 轮询钉板用）。
type fakeTeamRunReader struct {
	mu   sync.Mutex
	runs map[string][]biz.TeamRun // teamStageID → runs
}

func (f *fakeTeamRunReader) set(stageID string, runs ...biz.TeamRun) {
	f.mu.Lock()
	f.runs[stageID] = runs
	f.mu.Unlock()
}

func (f *fakeTeamRunReader) GetTeamRun(_ context.Context, id string) (biz.TeamRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for _, rs := range f.runs {
		for _, r := range rs {
			if r.ID == id {
				return r, nil
			}
		}
	}
	return biz.TeamRun{}, errors.New("not found")
}

func (f *fakeTeamRunReader) ListTeamRunsByStage(_ context.Context, stageID string) ([]biz.TeamRun, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]biz.TeamRun, len(f.runs[stageID]))
	copy(out, f.runs[stageID])
	return out, nil
}

// D6 核心行为钉：waiter 副本的 300ms 轮询消费「别副本只写进决议表」的
// playbook_confirm 决议——本进程 noted 图与确认卡均无该决议，唯一事实源是
// 共享决议面（mem 档端口即跨进程共享面的等价物）。断言 ≤2s 推进（验收
// ≤1s 留足调度余量）、waiter 登记清空、确认卡按批准定案、决议收敛进本
// 进程缓存。
func TestHoldPlaybookConfirmPollConsumesCrossReplicaDecision(t *testing.T) {
	t.Parallel()
	seq := &fakeSeq{}
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), seq, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1", TaskID: "task-1", TurnID: "turn-1"})
	step := &biz.PlanStep{ID: "st-confirm", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusPending}

	type result struct{ approved, held bool }
	done := make(chan result, 1)
	go func() {
		approved, held := r.holdPlaybookConfirm(context.Background(), step)
		done <- result{approved, held}
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 副本 B 决议：只写共享决议面。
	if _, err := port.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
			Kind:      biz.WaiterDecisionKindPlaybookConfirm,
			SessionID: "sp-1",
			ScopeID:   "st-confirm",
		},
		Approved:  true,
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record cross-replica decision: %v", err)
	}
	select {
	case got := <-done:
		if !got.held || !got.approved {
			t.Fatalf("held=%v approved=%v", got.held, got.approved)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("poll did not consume cross-replica decision within budget")
	}
	if pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		t.Fatal("settle must clear waiter registrations")
	}
	// D6-f：直查进程内缓存（不经 lookupPlaybookConfirmDecision——该函数端口
	// 读径无论收敛 Store 是否存在都返回同一决议，经它复查=同义反复假绿）。
	if v, ok := pe.playbookConfirmDecided.Load(playbookConfirmKey("sp-1", "st-confirm")); !ok || v != true {
		t.Fatalf("decision must converge into local cache: ok=%v v=%v", ok, v)
	}
	closed := false
	for _, ev := range seq.snapshot() {
		u, ok := ev.(*biz.StepUpdatedEvent)
		if ok && u.Step.ID == biz.PlaybookConfirmActivityID("sp-1", "st-confirm") && u.Step.Status == biz.StepStatusCompleted {
			closed = true
		}
	}
	if !closed {
		t.Fatal("poll settle must finalize the confirm card as completed")
	}
}

// D6 验收钉（堵 plan_executor_dag_dispatch.go ctx 超时→abortPlaybookConfirm
// 误杀链）：waiter 挂起期间别副本批准 + 本进程 ctx 死亡 → 绝不落 abort 取消
// 路径，按批准推进（Orchestrate 被调用），确认卡绝不定为 cancelled。
func TestDispatchStepCtxDeathWithApprovedDecisionDoesNotAbort(t *testing.T) {
	t.Parallel()
	seq := &fakeSeq{}
	repos := newFakeReposForExecutor()
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, seq, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	board := biz.PlanBoard{ID: "pb-ctx", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	// dispatchStep 只消费预标记 Running（发现#15 断言），hold 在其之前。
	step := &biz.PlanStep{ID: "st-confirm", PlanID: board.ID, TaskID: "task-1", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusRunning, Version: 1}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.dispatchStep(ctx, step)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	// 别副本批准：只写共享决议面（本进程 chan 无人 signal）。
	if _, err := port.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
			Kind:      biz.WaiterDecisionKindPlaybookConfirm,
			SessionID: "sp-1",
			ScopeID:   "st-confirm",
		},
		Approved:  true,
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record approved decision: %v", err)
	}
	cancel()
	// 按批准推进：Orchestrate 必须被调用（不是 abort 取消）。
	if !orch.waitForCall("st-confirm", 2*time.Second) {
		t.Fatal("approved decision at ctx death must proceed, not abort")
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatchStep did not return after ctx death")
	}
	// 确认卡绝不因 ctx 死亡被误定为 cancelled（settle 按批准定案 completed）。
	for _, ev := range seq.snapshot() {
		if u, ok := ev.(*biz.StepUpdatedEvent); ok &&
			u.Step.ID == biz.PlaybookConfirmActivityID("sp-1", "st-confirm") &&
			u.Step.Status == biz.StepStatusCancelled {
			t.Fatal("approved decision must never finalize the card as cancelled")
		}
	}
}

// D6 验收钉（deny 侧）：ctx 死亡瞬间已有拒绝决议 → 走 skipPlaybookConfirmDenied
// （ConfirmDenied 标记阻断 resume 复活），而非 abort 的无标记取消。
func TestDispatchStepCtxDeathWithDeniedDecisionSkipsNotAbort(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	pe := NewPlanExecutor(repos, newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	board := biz.PlanBoard{ID: "pb-ctx-deny", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	step := &biz.PlanStep{ID: "st-confirm", PlanID: board.ID, TaskID: "task-1", Label: "发布", ConfirmBefore: true, Status: biz.PlanStepStatusRunning, Version: 1}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		r.dispatchStep(ctx, step)
		close(done)
	}()
	deadline := time.Now().Add(2 * time.Second)
	for !pe.HasPlaybookStageConfirm("sp-1", "st-confirm") {
		if time.Now().After(deadline) {
			t.Fatal("waiter not registered")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if _, err := port.TryRecordDecision(context.Background(), biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
			Kind:      biz.WaiterDecisionKindPlaybookConfirm,
			SessionID: "sp-1",
			ScopeID:   "st-confirm",
		},
		Approved:  false,
		Reason:    "no",
		Actor:     "replica-b",
		DecidedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record denied decision: %v", err)
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("dispatchStep did not return after ctx death")
	}
	if step.Status != biz.PlanStepStatusSkipped {
		t.Fatalf("denied decision at ctx death must skip: status=%v", step.Status)
	}
	if step.Error == nil || step.Error.Code != biz.StepErrCodeConfirmDenied {
		t.Fatalf("deny provenance marker missing: err=%+v", step.Error)
	}
}

// D6-3 钉：team 完成通知永不落到本进程 chan（跨副本）时，team_runs_v2
// 终态行轮询推进 step——Completed 行 → step 完成。
func TestDispatchStepTeamCompletionPollerAdvances(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, &fakeSeq{}, loggateway.NewNoop())
	reader := &fakeTeamRunReader{runs: map[string][]biz.TeamRun{}}
	pe.SetTeamRunReader(reader)
	board := biz.PlanBoard{ID: "pb-poll", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	step := &biz.PlanStep{ID: "st-poll", PlanID: board.ID, TaskID: "task-1", Label: "执行", Status: biz.PlanStepStatusRunning, Version: 1}

	done := make(chan struct{})
	go func() {
		r.dispatchStep(context.Background(), step)
		close(done)
	}()
	if !orch.waitForCall("st-poll", 2*time.Second) {
		t.Fatal("dispatch did not reach orchestrator")
	}
	// 跨副本场景：完成通知落在别副本；本进程只靠 team_runs_v2 终态行。
	reader.set("fake-team-stage-st-poll", biz.TeamRun{
		ID: "run-1", TeamStageID: "fake-team-stage-st-poll", Seq: 1,
		Status: biz.TeamRunV2StatusCompleted,
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("poller did not advance the step")
	}
	if step.Status != biz.PlanStepStatusCompleted {
		t.Fatalf("step status=%v, want completed", step.Status)
	}
}

// D6-3 钉（cancel 侧）：Cancelled 终态行经轮询合成的事件必须让 L4 分类器
// 命中 cancel 类——不消耗自动重试预算（不重跑用户已取消的团队）。
func TestDispatchStepTeamCompletionPollerCancelledNoRetry(t *testing.T) {
	t.Parallel()
	repos := newFakeReposForExecutor()
	orch := newFakeOrchestrator()
	pe := NewPlanExecutor(repos, orch, &fakeSeq{}, loggateway.NewNoop())
	// D6-f：关退避（同全部重试语义钉板先例 plan_executor_dag_cancel_test.go
	// 等）——默认 5s 退避下 300ms 观测窗永远等不到第 2 次调用，断言空转；
	// backoff=0 时若锚缺失（Cancelled 落入 transient 重试）300ms 内必见
	// 第 2 次 Orchestrate，本断言才真钉得住「不耗重试预算」。
	pe.stepRetryBackoff = 0
	reader := &fakeTeamRunReader{runs: map[string][]biz.TeamRun{}}
	pe.SetTeamRunReader(reader)
	board := biz.PlanBoard{ID: "pb-poll-cancel", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	step := &biz.PlanStep{ID: "st-cancel", PlanID: board.ID, TaskID: "task-1", Label: "执行", Status: biz.PlanStepStatusRunning, Version: 1}

	done := make(chan struct{})
	go func() {
		r.dispatchStep(context.Background(), step)
		close(done)
	}()
	if !orch.waitForCall("st-cancel", 2*time.Second) {
		t.Fatal("dispatch did not reach orchestrator")
	}
	reader.set("fake-team-stage-st-cancel", biz.TeamRun{
		ID: "run-1", TeamStageID: "fake-team-stage-st-cancel", Seq: 1,
		Status: biz.TeamRunV2StatusCancelled, // Error 为空——补 team_cancelled 分类锚
	})
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("poller did not settle the cancelled run")
	}
	if step.Status != biz.PlanStepStatusFailed {
		t.Fatalf("cancelled run must fail the step: status=%v", step.Status)
	}
	if orch.waitForCallCount("st-cancel", 2, 300*time.Millisecond) {
		t.Fatal("cancelled run must not consume the retry budget")
	}
}

// D6 钉：多 run 同 stage（重试/假启动对账）时只有 Seq 最大者权威——旧 run
// 终态不得推进 step。
func TestPollTeamRunTerminalLatestSeqWins(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	reader := &fakeTeamRunReader{runs: map[string][]biz.TeamRun{}}
	pe.SetTeamRunReader(reader)
	r := newDagRun(pe, biz.PlanBoard{SessionID: "sp-1", ID: "pb-1"})
	reader.set("ts-1",
		biz.TeamRun{ID: "run-old", TeamStageID: "ts-1", Seq: 1, Status: biz.TeamRunV2StatusFailed, Error: "boom"},
		biz.TeamRun{ID: "run-new", TeamStageID: "ts-1", Seq: 2, Status: biz.TeamRunV2StatusRunning},
	)
	if _, done := r.pollTeamRunTerminal(context.Background(), "ts-1", "st-1"); done {
		t.Fatal("stale terminal run must not advance while a newer run is active")
	}
	reader.set("ts-1",
		biz.TeamRun{ID: "run-old", TeamStageID: "ts-1", Seq: 1, Status: biz.TeamRunV2StatusFailed, Error: "boom"},
		biz.TeamRun{ID: "run-new", TeamStageID: "ts-1", Seq: 2, Status: biz.TeamRunV2StatusCompleted},
	)
	ev, done := r.pollTeamRunTerminal(context.Background(), "ts-1", "st-1")
	if !done || !ev.Success || ev.TeamRunID != "run-new" {
		t.Fatalf("latest terminal run must advance: done=%v ev=%+v", done, ev)
	}
}

// D6 钉：WaitBoardStartup 慢路径——本进程无 chan 登记、无看板行时，从决议
// 表对账别副本的启动决议（失败带原因/成功放行）。
func TestWaitBoardStartupDecisionTableSlowPath(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	ctx := context.Background()

	if _, err := port.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: biz.WaiterDecisionKindBoardStartup, ScopeID: "pb-x"},
		Approved:                  false,
		Reason:                    "assembly failed",
		Actor:                     "replica-b",
		DecidedAt:                 time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record startup decision: %v", err)
	}
	ok, reason := pe.WaitBoardStartup(ctx, "pb-x", time.Second)
	if ok || reason != "assembly failed" {
		t.Fatalf("failed startup decision: ok=%v reason=%q", ok, reason)
	}

	if _, err := port.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
		ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{Kind: biz.WaiterDecisionKindBoardStartup, ScopeID: "pb-ok"},
		Approved:                  true,
		Actor:                     "replica-b",
		DecidedAt:                 time.Now().UTC(),
	}); err != nil {
		t.Fatalf("record startup decision: %v", err)
	}
	ok, reason = pe.WaitBoardStartup(ctx, "pb-ok", time.Second)
	if !ok || reason != "" {
		t.Fatalf("approved startup decision: ok=%v reason=%q", ok, reason)
	}
}

// D6 钉：signalStartup 双写决议表（CAS 与 buffered-1 chan 首发有效语义对齐）。
func TestSignalStartupRecordsDecision(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	pe.signalStartup("pb-s", true, "")
	dec, found, err := port.LoadDecision(context.Background(), biz.ExecutorWaiterDecisionKey{
		Kind:    biz.WaiterDecisionKindBoardStartup,
		ScopeID: "pb-s",
	})
	if err != nil || !found {
		t.Fatalf("signalStartup must record the decision: found=%v err=%v", found, err)
	}
	if !dec.Approved || dec.Actor != "plan_executor.startup" {
		t.Fatalf("decision=%+v", dec)
	}
}

// ============================================================================
// D6-f 评审确认项钉（wf_7b336566-b11）
// ============================================================================

// failingWaiterDecisions 是恒失败的决议端口桩（写失败降级钉用——InMem 不
// 会失败，无法覆盖 err 分支）。
type failingWaiterDecisions struct{}

func (failingWaiterDecisions) TryRecordDecision(context.Context, biz.ExecutorWaiterDecision) (bool, error) {
	return false, errors.New("table unavailable")
}

func (failingWaiterDecisions) LoadDecision(context.Context, biz.ExecutorWaiterDecisionKey) (biz.ExecutorWaiterDecision, bool, error) {
	return biz.ExecutorWaiterDecision{}, false, errors.New("table unavailable")
}

// ctxBoundWaiterDecisions 是尊重 ctx 的决议端口桩（钉死「快照写不得携已死
// dag ctx」——InMem 实现忽略 ctx，无法区分；PG 实现在 ctx 死亡时 INSERT
// 必败，此桩模拟该语义）。
type ctxBoundWaiterDecisions struct{ inner *InMemExecutorWaiterDecisions }

func (c ctxBoundWaiterDecisions) TryRecordDecision(ctx context.Context, d biz.ExecutorWaiterDecision) (bool, error) {
	if err := ctx.Err(); err != nil {
		return false, err
	}
	return c.inner.TryRecordDecision(ctx, d)
}

func (c ctxBoundWaiterDecisions) LoadDecision(ctx context.Context, k biz.ExecutorWaiterDecisionKey) (biz.ExecutorWaiterDecision, bool, error) {
	if err := ctx.Err(); err != nil {
		return biz.ExecutorWaiterDecision{}, false, err
	}
	return c.inner.LoadDecision(ctx, k)
}

// D6-f F3 钉：NotePlaybookConfirmDecision 决议表双写——recorded=true 路径
// 双键落表 + 进程内图同写 + 返回权威值。整段删除决议表块的历史缺口（30 钉
// 全绿的假绿区）由此封闭。
func TestNotePlaybookConfirmDecisionDoubleWritesTable(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)

	if got := pe.NotePlaybookConfirmDecision("sp-1", "st-confirm", true); !got {
		t.Fatal("recorded decision must return the approved value")
	}
	ids := playbookConfirmLookupIDs("sp-1", "st-confirm")
	if len(ids) != 2 {
		t.Fatalf("double-key addressing expected, got %v", ids)
	}
	for _, id := range ids {
		dec, found, err := port.LoadDecision(context.Background(), biz.ExecutorWaiterDecisionKey{
			Kind: biz.WaiterDecisionKindPlaybookConfirm, SessionID: "sp-1", ScopeID: id,
		})
		if err != nil || !found || !dec.Approved {
			t.Fatalf("table double-write missing for %q: found=%v err=%v dec=%+v", id, found, err, dec)
		}
		if v, ok := pe.playbookConfirmDecided.Load(playbookConfirmKey("sp-1", id)); !ok || v != true {
			t.Fatalf("local map double-write missing for %q: ok=%v v=%v", id, ok, v)
		}
	}
}

// D6-f F1+F3 钉：CAS 输家收敛——别副本先到 approved=true，本进程迟到 deny
// 必须收敛到先到值：返回值=true（调用方据此 Resolve，HITL 先到批准不被静默
// 丢弃）、进程内图收敛=true、表内先到值不变（严禁 UPDATE 翻案）。
func TestNotePlaybookConfirmDecisionCASLoserConvergesToFirstWriter(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	port := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(port)
	ctx := context.Background()

	// 别副本先赢：双键均已落 approved=true。
	for _, id := range playbookConfirmLookupIDs("sp-1", "st-confirm") {
		if _, err := port.TryRecordDecision(ctx, biz.ExecutorWaiterDecision{
			ExecutorWaiterDecisionKey: biz.ExecutorWaiterDecisionKey{
				Kind: biz.WaiterDecisionKindPlaybookConfirm, SessionID: "sp-1", ScopeID: id,
			},
			Approved: true, Actor: "replica-b", DecidedAt: time.Now().UTC(),
		}); err != nil {
			t.Fatalf("record first-writer decision: %v", err)
		}
	}
	// 本副本迟到 deny：CAS 输家。
	if got := pe.NotePlaybookConfirmDecision("sp-1", "st-confirm", false); !got {
		t.Fatal("CAS loser must return the first-writer value (true), not the losing deny")
	}
	if v, ok := pe.lookupPlaybookConfirmDecision("sp-1", "st-confirm"); !ok || !v {
		t.Fatalf("local cache must converge to first-writer: ok=%v v=%v", ok, v)
	}
	dec, found, err := port.LoadDecision(ctx, biz.ExecutorWaiterDecisionKey{
		Kind: biz.WaiterDecisionKindPlaybookConfirm, SessionID: "sp-1", ScopeID: "st-confirm",
	})
	if err != nil || !found || !dec.Approved || dec.Actor != "replica-b" {
		t.Fatalf("first-writer decision must survive (no UPDATE): found=%v err=%v dec=%+v", found, err, dec)
	}
}

// D6-f F3 钉：决议表写失败降级——进程内语义兜底（图存原始值、返回原始值），
// 不堵同进程 waiter 流。
func TestNotePlaybookConfirmDecisionTableFailureDegradesToProcessLocal(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	pe.SetWaiterDecisionPort(failingWaiterDecisions{})

	if got := pe.NotePlaybookConfirmDecision("sp-1", "st-confirm", true); !got {
		t.Fatal("table failure must degrade to process-local semantics with the original value")
	}
	if v, ok := pe.lookupPlaybookConfirmDecision("sp-1", "st-confirm"); !ok || !v {
		t.Fatalf("local map must hold the original decision on table failure: ok=%v v=%v", ok, v)
	}
}

// D6-f F2+F3 钉：team_agent_loads 快照写刻意携 context.Background()——
// dag ctx 已死（Orchestrate 成功后/快照写之前的验收堵口可达窗口）快照也
// 必须落表（孤儿 team 恰是该快照的标称消费场景）。ctxBound 桩模拟 PG 的
// ctx 语义：旧实现携死 ctx 写必败（红），现实现绿。同步调用零 goroutine，
// 完全确定。
func TestDispatchStepTeamAgentLoadsSnapshotSurvivesCtxDeath(t *testing.T) {
	t.Parallel()
	pe := NewPlanExecutor(newFakeReposForExecutor(), newFakeOrchestrator(), &fakeSeq{}, loggateway.NewNoop())
	inner := NewInMemExecutorWaiterDecisions()
	pe.SetWaiterDecisionPort(ctxBoundWaiterDecisions{inner: inner})
	board := biz.PlanBoard{ID: "pb-snap", SessionID: "sp-1", TaskID: "task-1", Status: biz.PlanStatusExecuting}
	r := newDagRun(pe, board)
	step := &biz.PlanStep{ID: "st-snap", PlanID: board.ID, TaskID: "task-1", Label: "执行",
		AgentKeys: []string{"agent-a", "agent-b"}, Status: biz.PlanStepStatusRunning, Version: 1}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // dag ctx 先死——快照写窗口的极端形态
	r.dispatchStep(ctx, step)

	dec, found, err := inner.LoadDecision(context.Background(), biz.ExecutorWaiterDecisionKey{
		Kind: biz.WaiterDecisionKindTeamAgentLoads, SessionID: "sp-1", ScopeID: "fake-team-st-snap",
	})
	if err != nil || !found {
		t.Fatalf("team_agent_loads snapshot must survive dag ctx death: found=%v err=%v", found, err)
	}
	if !dec.Approved || dec.Reason != "agent-a,agent-b" || dec.Actor != "plan_executor.dispatch" {
		t.Fatalf("snapshot=%+v", dec)
	}
}
