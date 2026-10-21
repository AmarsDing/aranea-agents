package biz

import (
	"context"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// --- P1 FR-8：GraphTask 四路径 CAS 迁移（禁全行覆写回退并发终态） ---

// fr8TransitionCall 捕获一次谓词迁移调用的入参。
type fr8TransitionCall struct {
	taskID string
	from   []GraphTaskStatus
	to     GraphTaskStatus
	patch  *TaskTransitionPatch
}

// fr8TaskRepo 捕获式桩：谓词迁移可脚本化输赢序列，全行 UpdateTask/事件各计数，
// 断言四路径不再经过全行覆写。
type fr8TaskRepo struct {
	stubTaskRepo
	transitionCalls []fr8TransitionCall
	transitionFn    func(taskID string, from []GraphTaskStatus, to GraphTaskStatus, patch *TaskTransitionPatch) (*GraphTask, bool, error)
	updateTaskCalls int
	savedEvents     []string
}

func (s *fr8TaskRepo) TransitionTaskStatusWhere(_ context.Context, taskID string, fromStatuses []GraphTaskStatus, toStatus GraphTaskStatus, patches ...TaskTransitionPatch) (*GraphTask, bool, error) {
	var patch *TaskTransitionPatch
	if len(patches) > 0 {
		p := patches[0]
		patch = &p
	}
	s.transitionCalls = append(s.transitionCalls, fr8TransitionCall{taskID: taskID, from: fromStatuses, to: toStatus, patch: patch})
	if s.transitionFn != nil {
		return s.transitionFn(taskID, fromStatuses, toStatus, patch)
	}
	return nil, false, nil
}

func (s *fr8TaskRepo) UpdateTask(context.Context, *GraphTask) error {
	s.updateTaskCalls++
	return nil
}

func (s *fr8TaskRepo) SaveTaskEvent(_ context.Context, e *TaskEvent) error {
	s.savedEvents = append(s.savedEvents, e.EventType)
	return nil
}

type fr8StatusPublisher struct{ published []*GraphTask }

func (p *fr8StatusPublisher) PublishGraphTaskStatus(_ context.Context, t *GraphTask, _ map[string]any) {
	p.published = append(p.published, t)
}

func newFR8Usecase(repo *fr8TaskRepo) (*TaskUsecase, *fr8StatusPublisher) {
	pub := &fr8StatusPublisher{}
	uc := &TaskUsecase{
		reader:        repo,
		writer:        repo,
		comments:      repo,
		logs:          repo,
		runs:          repo,
		events:        repo,
		heartbeats:    map[string]time.Time{},
		leaseDeadline: map[string]time.Time{},
		lg:            loggateway.NewNoop(),
	}
	uc.SetStatusPublisher(pub)
	return uc, pub
}

// TestReportBlocked_PredicateTransitionNoFullRowWrite：ReportBlocked 必须走
// 谓词迁移（from=claimed→blocked，metadata 随迁移原子写入），禁止 Get→全行
// UpdateTask——后者会把 read 后并发落账的 Output/Summary 用旧值覆写回去。
func TestReportBlocked_PredicateTransitionNoFullRowWrite(t *testing.T) {
	repo := &fr8TaskRepo{stubTaskRepo: stubTaskRepo{tasks: map[string]*GraphTask{
		"t1": {TaskID: "t1", NodeID: "n1", Status: GraphTaskStatusClaimed, Assignee: "a1"},
	}}}
	repo.transitionFn = func(taskID string, _ []GraphTaskStatus, to GraphTaskStatus, patch *TaskTransitionPatch) (*GraphTask, bool, error) {
		task := repo.tasks[taskID]
		task.Status = to
		if patch != nil && patch.Metadata != nil {
			task.Metadata = *patch.Metadata
		}
		return task, true, nil
	}
	uc, _ := newFR8Usecase(repo)

	task, err := uc.ReportBlocked(context.Background(), "t1", "waiting on upstream", "m-x")
	if err != nil {
		t.Fatalf("ReportBlocked: %v", err)
	}
	if task.Status != GraphTaskStatusBlocked || task.Metadata != "m-x" {
		t.Fatalf("expected blocked+m-x via predicate patch, got %+v", task)
	}
	if repo.updateTaskCalls != 0 {
		t.Fatalf("P1 FR-8: ReportBlocked must not full-row UpdateTask, got %d calls", repo.updateTaskCalls)
	}
	if len(repo.transitionCalls) != 1 {
		t.Fatalf("expected exactly 1 predicate transition, got %d", len(repo.transitionCalls))
	}
	call := repo.transitionCalls[0]
	if len(call.from) != 1 || call.from[0] != GraphTaskStatusClaimed || call.to != GraphTaskStatusBlocked {
		t.Fatalf("expected claimed→blocked predicate, got from=%v to=%v", call.from, call.to)
	}
	if call.patch == nil || call.patch.Metadata == nil || *call.patch.Metadata != "m-x" {
		t.Fatalf("expected metadata patch m-x, got %+v", call.patch)
	}
	if len(repo.savedEvents) != 1 || repo.savedEvents[0] != "task_blocked" {
		t.Fatalf("expected task_blocked event, got %v", repo.savedEvents)
	}
}

// TestReportBlocked_LostRaceReturnsError：并发先完成（claimed 已不在）时谓词
// 0 命中，必须返回错误而非静默全行覆写。
func TestReportBlocked_LostRaceReturnsError(t *testing.T) {
	repo := &fr8TaskRepo{stubTaskRepo: stubTaskRepo{tasks: map[string]*GraphTask{
		"t1": {TaskID: "t1", Status: GraphTaskStatusComplete},
	}}}
	repo.transitionFn = func(string, []GraphTaskStatus, GraphTaskStatus, *TaskTransitionPatch) (*GraphTask, bool, error) {
		return nil, false, nil
	}
	uc, _ := newFR8Usecase(repo)

	if _, err := uc.ReportBlocked(context.Background(), "t1", "r", "m"); err == nil {
		t.Fatal("P1 FR-8: ReportBlocked on concurrently-completed task must error, not overwrite")
	}
	if repo.updateTaskCalls != 0 {
		t.Fatalf("no full-row write allowed on lost race, got %d", repo.updateTaskCalls)
	}
	if len(repo.savedEvents) != 0 {
		t.Fatalf("no task_blocked event on lost race, got %v", repo.savedEvents)
	}
}

// TestUnblockTask_ClearsClaimViaPatch：Unblock 必须经谓词迁移并同 UPDATE 清空
// assignee/claimed_at（旧实现 claimed_at=nil 走 UpdateTask 根本无法落库清
// 除——builder 跳过 nil 字段，陈旧 claimed_at 残留）。
func TestUnblockTask_ClearsClaimViaPatch(t *testing.T) {
	claimedAt := time.Now()
	repo := &fr8TaskRepo{stubTaskRepo: stubTaskRepo{tasks: map[string]*GraphTask{
		"t2": {TaskID: "t2", NodeID: "n2", Status: GraphTaskStatusBlocked, Assignee: "a1", ClaimedAt: &claimedAt},
	}}}
	repo.transitionFn = func(taskID string, _ []GraphTaskStatus, to GraphTaskStatus, patch *TaskTransitionPatch) (*GraphTask, bool, error) {
		task := repo.tasks[taskID]
		task.Status = to
		if patch != nil {
			if patch.ClearAssignee {
				task.Assignee = ""
			}
			if patch.ClearClaimedAt {
				task.ClaimedAt = nil
			}
		}
		return task, true, nil
	}
	uc, _ := newFR8Usecase(repo)

	task, err := uc.UnblockTask(context.Background(), "t2", "upstream ready")
	if err != nil {
		t.Fatalf("UnblockTask: %v", err)
	}
	if task.Status != GraphTaskStatusPending || task.Assignee != "" || task.ClaimedAt != nil {
		t.Fatalf("expected pending+claim cleared, got %+v", task)
	}
	if repo.updateTaskCalls != 0 {
		t.Fatalf("P1 FR-8: UnblockTask must not full-row UpdateTask, got %d calls", repo.updateTaskCalls)
	}
	if len(repo.transitionCalls) != 1 {
		t.Fatalf("expected 1 transition, got %d", len(repo.transitionCalls))
	}
	call := repo.transitionCalls[0]
	if len(call.from) != 1 || call.from[0] != GraphTaskStatusBlocked || call.to != GraphTaskStatusPending {
		t.Fatalf("expected blocked→pending predicate, got from=%v to=%v", call.from, call.to)
	}
	if call.patch == nil || !call.patch.ClearAssignee || !call.patch.ClearClaimedAt {
		t.Fatalf("expected clear-claim patch, got %+v", call.patch)
	}
	if len(repo.savedEvents) != 1 || repo.savedEvents[0] != "task_unblocked" {
		t.Fatalf("expected task_unblocked event, got %v", repo.savedEvents)
	}
}

// TestUnblockTask_NotBlockedRaceReturnsError：并发解阻/复位后谓词 0 命中 →
// 报错且不产生事件。
func TestUnblockTask_NotBlockedRaceReturnsError(t *testing.T) {
	repo := &fr8TaskRepo{stubTaskRepo: stubTaskRepo{tasks: map[string]*GraphTask{
		"t2": {TaskID: "t2", Status: GraphTaskStatusClaimed},
	}}}
	repo.transitionFn = func(string, []GraphTaskStatus, GraphTaskStatus, *TaskTransitionPatch) (*GraphTask, bool, error) {
		return nil, false, nil
	}
	uc, _ := newFR8Usecase(repo)

	if _, err := uc.UnblockTask(context.Background(), "t2", ""); err == nil {
		t.Fatal("P1 FR-8: UnblockTask on non-blocked task must error")
	}
	if len(repo.savedEvents) != 0 {
		t.Fatalf("no task_unblocked event on lost race, got %v", repo.savedEvents)
	}
}

// TestReleaseClaim_ConcurrentCompleteNotRegressed：dispatch 失败释放认领与并
// 发 Complete 竞速——谓词 0 命中时必须静默跳过（任务保持 completed），禁止
// 旧实现的 completed→pending 回退覆写，且不产生释放事件/状态发布。
func TestReleaseClaim_ConcurrentCompleteNotRegressed(t *testing.T) {
	repo := &fr8TaskRepo{stubTaskRepo: stubTaskRepo{tasks: map[string]*GraphTask{
		"t3": {TaskID: "t3", NodeID: "n3", Status: GraphTaskStatusComplete, Assignee: "a1"},
	}}}
	// 桩镜像真实谓词：仅 claimed 命中，completed 不命中。
	repo.transitionFn = func(taskID string, from []GraphTaskStatus, to GraphTaskStatus, patch *TaskTransitionPatch) (*GraphTask, bool, error) {
		task := repo.tasks[taskID]
		for _, f := range from {
			if task.Status == f {
				task.Status = to
				if patch != nil {
					if patch.ClearAssignee {
						task.Assignee = ""
					}
					if patch.ClearClaimedAt {
						task.ClaimedAt = nil
					}
				}
				return task, true, nil
			}
		}
		return nil, false, nil
	}
	uc, pub := newFR8Usecase(repo)
	uc.leaseDeadline["t3"] = time.Now().Add(time.Minute)
	uc.heartbeats["t3"] = time.Now()

	uc.ReleaseClaim(context.Background(), "t3")

	if repo.tasks["t3"].Status != GraphTaskStatusComplete {
		t.Fatalf("P1 FR-8: concurrently completed task regressed to %v", repo.tasks["t3"].Status)
	}
	if repo.updateTaskCalls != 0 {
		t.Fatalf("no full-row write allowed, got %d", repo.updateTaskCalls)
	}
	if len(repo.savedEvents) != 0 {
		t.Fatalf("no claim-released event on lost race, got %v", repo.savedEvents)
	}
	if len(pub.published) != 0 {
		t.Fatalf("no status publish on lost race, got %d", len(pub.published))
	}
	if _, ok := uc.leaseDeadline["t3"]; ok {
		t.Fatal("lease bookkeeping must be cleared regardless of race outcome")
	}
}

// TestReleaseClaim_WinnerClearsClaimViaPatch：正常路径经谓词迁移释放并清空
// 认领字段，事件/发布各一次。
func TestReleaseClaim_WinnerClearsClaimViaPatch(t *testing.T) {
	claimedAt := time.Now()
	repo := &fr8TaskRepo{stubTaskRepo: stubTaskRepo{tasks: map[string]*GraphTask{
		"t4": {TaskID: "t4", NodeID: "n4", Status: GraphTaskStatusClaimed, Assignee: "a1", ClaimedAt: &claimedAt},
	}}}
	repo.transitionFn = func(taskID string, from []GraphTaskStatus, to GraphTaskStatus, patch *TaskTransitionPatch) (*GraphTask, bool, error) {
		task := repo.tasks[taskID]
		for _, f := range from {
			if task.Status == f {
				task.Status = to
				if patch != nil {
					if patch.ClearAssignee {
						task.Assignee = ""
					}
					if patch.ClearClaimedAt {
						task.ClaimedAt = nil
					}
				}
				return task, true, nil
			}
		}
		return nil, false, nil
	}
	uc, pub := newFR8Usecase(repo)
	uc.leaseDeadline["t4"] = time.Now().Add(time.Minute)

	uc.ReleaseClaim(context.Background(), "t4")

	task := repo.tasks["t4"]
	if task.Status != GraphTaskStatusPending || task.Assignee != "" || task.ClaimedAt != nil {
		t.Fatalf("expected pending+claim cleared, got %+v", task)
	}
	if len(repo.transitionCalls) != 1 || len(repo.transitionCalls[0].from) != 1 ||
		repo.transitionCalls[0].from[0] != GraphTaskStatusClaimed || repo.transitionCalls[0].to != GraphTaskStatusPending {
		t.Fatalf("expected claimed→pending predicate, got %+v", repo.transitionCalls)
	}
	if len(repo.savedEvents) != 1 || repo.savedEvents[0] != "task_claim_released" {
		t.Fatalf("expected task_claim_released event, got %v", repo.savedEvents)
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 status publish, got %d", len(pub.published))
	}
}

// TestCheckTimeouts_SkipsConcurrentlyCompletedTasks：批量超时逐 id 谓词迁移
//（claimed→timed_out）；与并发 Complete 交错时输家跳过（debug 日志、无事件
// 无发布），赢家正常终态化——断言无 complete→timed_out 回退。
func TestCheckTimeouts_SkipsConcurrentlyCompletedTasks(t *testing.T) {
	repo := &fr8TaskRepo{stubTaskRepo: stubTaskRepo{tasks: map[string]*GraphTask{
		"t5": {TaskID: "t5", NodeID: "n5", Status: GraphTaskStatusClaimed, Assignee: "a1"},
		"t6": {TaskID: "t6", NodeID: "n6", Status: GraphTaskStatusClaimed, Assignee: "a2"},
	}}}
	// 交错模拟：批量读到两 claimed 后，t6 被并发 Complete 抢先。t6 的谓词
	// 0 命中（桩内状态仍 claimed，但 transitionFn 按"并发已落 completed"演
	// 出）——现实场景由 DB 谓词保证，这里脚本化输赢。
	repo.transitionFn = func(taskID string, _ []GraphTaskStatus, to GraphTaskStatus, _ *TaskTransitionPatch) (*GraphTask, bool, error) {
		if taskID == "t6" {
			repo.tasks["t6"].Status = GraphTaskStatusComplete // 并发完成已落地
			return nil, false, nil
		}
		task := repo.tasks[taskID]
		task.Status = to
		return task, true, nil
	}
	uc, pub := newFR8Usecase(repo)
	past := time.Now().Add(-time.Minute)
	uc.leaseDeadline["t5"] = past
	uc.leaseDeadline["t6"] = past

	if err := uc.CheckTimeouts(context.Background()); err != nil {
		t.Fatalf("CheckTimeouts: %v", err)
	}
	if repo.tasks["t5"].Status != GraphTaskStatusTimedOut {
		t.Fatalf("t5 must be timed_out, got %v", repo.tasks["t5"].Status)
	}
	if repo.tasks["t6"].Status != GraphTaskStatusComplete {
		t.Fatalf("P1 FR-8: t6 regressed — expected complete preserved, got %v", repo.tasks["t6"].Status)
	}
	timedOutEvents := 0
	for _, et := range repo.savedEvents {
		if et == "task_timed_out" {
			timedOutEvents++
		}
	}
	if timedOutEvents != 1 {
		t.Fatalf("expected exactly 1 task_timed_out event (winner only), got %v", repo.savedEvents)
	}
	if len(pub.published) != 1 {
		t.Fatalf("expected 1 status publish (winner only), got %d", len(pub.published))
	}
	for _, call := range repo.transitionCalls {
		if len(call.from) != 1 || call.from[0] != GraphTaskStatusClaimed || call.to != GraphTaskStatusTimedOut {
			t.Fatalf("expected claimed→timed_out predicate, got %+v", call)
		}
		if call.patch != nil {
			t.Fatalf("timeout transition must not patch fields, got %+v", call.patch)
		}
	}
}
