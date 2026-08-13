package agent

// task_planner_reliability_test.go — 2026-08-13 编排审查 P0 修复（F7-F9）的回归测试。
//
// 覆盖：
//   F7a/B2 LLM 产出重复 subtask ID → 重映射后全局 ID 冲突，validateSubTaskDAG
//           必须拒绝（否则 DAG 节点互相遮蔽、依赖悬空/错接，执行器静默跑错图）
//   F7b/B2 流式路径 PublishV2Board 必须补发 GraphNode 更新（截取/清理后的最终
//           DependsOn），否则 DAG 视图永久残留悬挂边
//   F8/Y3  瞬时故障重试必须有上限（默认 5 次），耗尽后报错走 decompose_failed
//           降级——无限重试会让 turn 永远卡在「规划中」并持续烧 LLM 调用
//   F9/Y4  分解失败/为空时，已发布的 PlanBoard 壳（Status=planning）必须收到
//           终态更新（failed）——否则前端计划面板永远停在「规划中」

import (
	"context"
	"errors"
	"testing"
	"time"

	"aranea-agents/internal/agent/llmcompat"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// F7a: 重复 subtask ID 必须被 DAG 校验拒绝。
func TestValidateSubTaskDAG_DuplicateIDRejected(t *testing.T) {
	t.Parallel()
	tasks := []biz.SubTask{
		{ID: "st_dup", Name: "A"},
		{ID: "st_dup", Name: "B"},
		{ID: "st_c", Name: "C", DependsOn: []string{"st_dup"}},
	}
	if err := validateSubTaskDAG(tasks); err == nil {
		t.Fatal("expected duplicate subtask ID error, got nil — duplicated IDs shadow each other and dangling dependents")
	}
}

// F7b: 流式发布路径下，PublishV2Board 必须补发全部 GraphNode 更新事件，
// 携带截取/清理后的最终 DependsOn（修复前只补发 PlanStep，不补发 GraphNode）。
func TestPublishV2Board_StreamPublishedRepublishesGraphNodes(t *testing.T) {
	t.Parallel()
	seq := &fakeSeqPublisher{}
	impl := &taskPlannerImpl{lg: loggateway.NewNoop(), seq: seq}
	plan := &biz.TaskPlan{
		ID:              "tp-stream",
		SpiritSessionID: "spirit-1",
		Strategy:        biz.StrategyDAG,
		StreamPublished: true,
		SubTasks: []biz.SubTask{
			{ID: "st_a", Name: "A"},
			{ID: "st_b", Name: "B", DependsOn: []string{"st_a"}},
		},
	}
	if _, err := impl.PublishV2Board(context.Background(), plan, nil, "chat-1"); err != nil {
		t.Fatalf("PublishV2Board: %v", err)
	}
	var gnUpdates []*biz.GraphNodeUpdatedEvent
	psUpdates := 0
	for _, e := range seq.events {
		switch ev := e.(type) {
		case *biz.GraphNodeUpdatedEvent:
			gnUpdates = append(gnUpdates, ev)
		case *biz.PlanStepUpdatedEvent:
			psUpdates++
		}
	}
	if psUpdates != 2 {
		t.Fatalf("PlanStepUpdated events = %d, want 2", psUpdates)
	}
	if len(gnUpdates) != 2 {
		t.Fatalf("GraphNodeUpdated events = %d, want 2 (streaming path must re-publish final graph nodes)", len(gnUpdates))
	}
	for _, ev := range gnUpdates {
		if ev.GraphNode.ID == "st_b" && (len(ev.GraphNode.DependsOn) != 1 || ev.GraphNode.DependsOn[0] != "st_a") {
			t.Fatalf("st_b final DependsOn = %v, want [st_a]", ev.GraphNode.DependsOn)
		}
	}
}

// F8: 瞬时故障重试耗尽上限后必须返回错误（走 decompose_failed 降级），
// 不得无限重试。
func TestDecomposeTaskStream_TransientFailure_StopsAtMaxAttempts(t *testing.T) {
	t.Parallel()
	attempts := 0
	impl := &taskPlannerImpl{
		lg:                   loggateway.NewNoop(),
		retryBackoffFn:       func(int) time.Duration { return time.Millisecond },
		maxDecomposeAttempts: 4,
	}
	impl.llmAttemptFn = func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _, _ string, _ func(biz.SubTask, int)) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		attempts++
		return nil, nil, retriableDecomposeErr()
	}
	_, _, err := impl.decomposeTaskStream(context.Background(), "msg", nil, 0, "spirit-p0", "tp_p0", nil)
	if err == nil {
		t.Fatal("expected exhaustion error after max attempts, got nil (infinite retry)")
	}
	if attempts != 4 {
		t.Fatalf("attempts = %d, want 4（达到重试上限即熔断）", attempts)
	}
}

// F8 默认值：未配置时上限为 5 次。
func TestDecomposeTaskStream_DefaultMaxAttemptsIs5(t *testing.T) {
	t.Parallel()
	attempts := 0
	impl := &taskPlannerImpl{
		lg:             loggateway.NewNoop(),
		retryBackoffFn: func(int) time.Duration { return time.Millisecond },
	}
	impl.llmAttemptFn = func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _, _ string, _ func(biz.SubTask, int)) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		attempts++
		return nil, nil, retriableDecomposeErr()
	}
	_, _, _ = impl.decomposeTaskStream(context.Background(), "msg", nil, 0, "spirit-p0", "tp_p0", nil)
	if attempts != 5 {
		t.Fatalf("attempts = %d, want default 5", attempts)
	}
}

// retriableDecomposeErr 与既有重试测试同源：StreamIdleError 包装的瞬时故障。
func retriableDecomposeErr() error {
	return apierror.Internal(apierror.DomainSpirit, "LLM stream call failed").
		WithCause(&llmcompat.StreamIdleError{Timeout: time.Second})
}

// F9: 流式壳已发布后分解失败，PlanBoard/GraphStage 必须收到终态事件。
func TestPlan_DecomposeFailure_PublishesBoardTerminal(t *testing.T) {
	t.Parallel()
	seq := &fakeSeqPublisher{}
	bus := &captureNoticeBus{}
	repo := &stubTaskPlanRepo{}
	impl := &taskPlannerImpl{
		repo:                 repo,
		eventBus:             bus,
		lg:                   loggateway.NewNoop(),
		seq:                  seq,
		retryBackoffFn:       func(int) time.Duration { return time.Millisecond },
		maxDecomposeAttempts: 1, // 首次即熔断（永久性错误）
	}
	impl.llmAttemptFn = func(_ context.Context, _ string, _ *biz.IntentArtifact, _ int, _, _ string, _ func(biz.SubTask, int)) ([]biz.SubTask, *biz.PlanTaskDAG, error) {
		return nil, nil, &decomposeConfigError{err: errors.New("no provider/model configured")}
	}

	plan, err := impl.Plan(context.Background(), biz.PlanInput{
		SpiritSessionID: "spirit-1",
		ChatSessionID:   "chat-1",
		UserMessage:     "组建两个团队并行分析这份日志",
		Mode:            "dag",
	})
	if err != nil {
		t.Fatalf("Plan should downgrade to direct instead of failing: %v", err)
	}
	if plan.Strategy != biz.StrategyDirect {
		t.Fatalf("strategy = %q, want direct (decompose failed)", plan.Strategy)
	}

	var boardTerminal *biz.PlanBoardUpdatedEvent
	stageFailed := false
	shellCreated := false
	for _, e := range seq.events {
		switch ev := e.(type) {
		case *biz.PlanBoardCreatedEvent:
			shellCreated = true
		case *biz.PlanBoardUpdatedEvent:
			if ev.PlanBoard.Status == biz.PlanStatusFailed {
				boardTerminal = ev
			}
		case *biz.GraphStageFailedEvent:
			stageFailed = true
		}
	}
	if !shellCreated {
		t.Fatal("expected PlanBoard shell created event (streaming path)")
	}
	if boardTerminal == nil {
		t.Fatal("PlanBoard never received terminal (failed) update — frontend stuck at 规划中")
	}
	if !stageFailed {
		t.Fatal("GraphStage never received failed event — DAG block stuck at running")
	}
}
