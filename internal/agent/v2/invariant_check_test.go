package v2

import (
	"context"
	"strconv"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// P1-2 事件溯源不变量（DSH "model-visible means logged"）：
// Task→Turn→Step 链路上的每个事件必须能从已发布的事件流重建其谱系。
// 断言为开发态（ARANEA_ORCH_INVARIANT=1）仅日志模式，以下用例直接驱动
// invariantChecker 构造可重建/不可重建场景验证检测能力。

func newTestInvariantChecker() *invariantChecker {
	return newInvariantChecker(loggateway.NewNoop())
}

func TestInvariantChecker_HappyPath_NoViolations(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	c.check(biz.NewTurnStartedEvent(biz.Turn{ID: "turn1", TaskID: "t1", SpiritSessionID: sid}))
	c.check(biz.NewStepCreatedEvent(biz.Step{ID: "st1", TurnID: "turn1", TaskID: "t1", SpiritSessionID: sid}))
	c.check(biz.NewStepStreamingEvent(sid, "t1", "st1", "content", "hello"))
	c.check(biz.NewStepCompletedEvent(biz.Step{ID: "st1", TurnID: "turn1", TaskID: "t1", SpiritSessionID: sid}))
	c.check(biz.NewTurnCompletedEvent(biz.Turn{ID: "turn1", TaskID: "t1", SpiritSessionID: sid}))
	c.check(biz.NewTaskCompletedEvent(biz.Task{ID: "t1", SessionID: sid}))
	if got := c.violations(); got != 0 {
		t.Fatalf("violations = %d, want 0", got)
	}
}

func TestInvariantChecker_UnknownStep(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	// step.completed 对应的 step 从未在事件流中创建——不可重建。
	c.check(biz.NewStepCompletedEvent(biz.Step{ID: "ghost", TaskID: "t1", SpiritSessionID: sid}))
	if got := c.violations(); got != 1 {
		t.Fatalf("violations = %d, want 1", got)
	}
}

func TestInvariantChecker_UnknownTurn(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	// step 挂在一个从未 turn.started 的 turn 上。
	c.check(biz.NewStepCreatedEvent(biz.Step{ID: "st1", TurnID: "ghost-turn", TaskID: "t1", SpiritSessionID: sid}))
	if got := c.violations(); got != 1 {
		t.Fatalf("violations = %d, want 1", got)
	}
}

func TestInvariantChecker_UnknownTask(t *testing.T) {
	c := newTestInvariantChecker()
	// turn.started 挂在从未 task.created 的 task 上。
	c.check(biz.NewTurnStartedEvent(biz.Turn{ID: "turn1", TaskID: "ghost-task", SpiritSessionID: "s1"}))
	if got := c.violations(); got != 1 {
		t.Fatalf("violations = %d, want 1", got)
	}
}

func TestInvariantChecker_DuplicateTerminal(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	step := biz.Step{ID: "st1", TaskID: "t1", SpiritSessionID: sid}
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	c.check(biz.NewStepCreatedEvent(step))
	c.check(biz.NewStepCompletedEvent(step))
	// 第二次终态事件——事件流无法区分哪个才是真终态。
	c.check(biz.NewStepCompletedEvent(step))
	if got := c.violations(); got != 1 {
		t.Fatalf("violations = %d, want 1", got)
	}
}

func TestInvariantChecker_EventAfterTerminal(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	step := biz.Step{ID: "st1", TaskID: "t1", SpiritSessionID: sid}
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	c.check(biz.NewStepCreatedEvent(step))
	c.check(biz.NewStepCompletedEvent(step))
	c.check(biz.NewStepUpdatedEvent(step)) // 终态后更新——重建会得到矛盾状态
	if got := c.violations(); got != 1 {
		t.Fatalf("violations = %d, want 1", got)
	}
}

func TestInvariantChecker_DuplicateCreate(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	step := biz.Step{ID: "st1", TaskID: "t1", SpiritSessionID: sid}
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	c.check(biz.NewStepCreatedEvent(step))
	c.check(biz.NewStepCreatedEvent(step)) // 重复创建——谱系歧义
	if got := c.violations(); got != 1 {
		t.Fatalf("violations = %d, want 1", got)
	}
}

func TestInvariantChecker_StreamingAfterTerminal_Tolerated(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	step := biz.Step{ID: "st1", TaskID: "t1", SpiritSessionID: sid}
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	c.check(biz.NewStepCreatedEvent(step))
	c.check(biz.NewStepCompletedEvent(step))
	// 16ms 批合并窗口内的迟到 delta 属良性竞态，不计违规（防误报）。
	c.check(biz.NewStepStreamingEvent(sid, "t1", "st1", "content", "late"))
	if got := c.violations(); got != 0 {
		t.Fatalf("violations = %d, want 0", got)
	}
}

func TestInvariantChecker_StreamingUnknownStep(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	// 未创建 step 的流式 delta——模型输出无谱系。
	c.check(biz.NewStepStreamingEvent(sid, "t1", "ghost", "content", "x"))
	if got := c.violations(); got != 1 {
		t.Fatalf("violations = %d, want 1", got)
	}
}

func TestInvariantChecker_LogDedup(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	step := biz.Step{ID: "ghost", TaskID: "t1", SpiritSessionID: sid}
	// 同一实体的同类违规重复出现：计数全部累计，日志只发一次（防刷屏）。
	c.check(biz.NewStepCompletedEvent(step))
	c.check(biz.NewStepFailedEvent(step))
	if got := c.violations(); got != 2 {
		t.Fatalf("violations = %d, want 2", got)
	}
	if got := c.logged(); got != 1 {
		t.Fatalf("logged = %d, want 1", got)
	}
}

func TestInvariantChecker_SessionEviction(t *testing.T) {
	c := newTestInvariantChecker()
	// 超过会话容量上限不得 panic/死锁；最老会话被逐出后其后续事件按
	// 未知谱系计违规（开发态可接受噪声），新会话正常工作。
	for i := 0; i < maxLineageSessions+8; i++ {
		sid := "evict-" + strconv.Itoa(i)
		c.check(biz.NewTaskCreatedEvent(biz.Task{ID: "t", SessionID: sid}))
	}
	if got := c.trackedSessions(); got > maxLineageSessions {
		t.Fatalf("tracked sessions = %d, exceeds cap %d", got, maxLineageSessions)
	}
}

func TestInvariantChecker_IgnoresNonChainEvents(t *testing.T) {
	c := newTestInvariantChecker()
	// 非 Task/Turn/Step 链事件（系统事件等）不参与谱系断言。
	c.check(biz.NewHeartbeatEvent("s1", "ping", time.Now()))
	if got := c.violations(); got != 0 {
		t.Fatalf("violations = %d, want 0", got)
	}
}

func TestInvariantChecker_EmptyParentSkipsContainment(t *testing.T) {
	c := newTestInvariantChecker()
	sid := "s1"
	// TurnID/TaskID 为空时跳过包含性检查（如游离 error step），只登记实体。
	c.check(biz.NewTurnStartedEvent(biz.Turn{ID: "turn1", SpiritSessionID: sid}))
	c.check(biz.NewStepCreatedEvent(biz.Step{ID: "st1", TaskID: "t1", SpiritSessionID: sid}))
	if got := c.violations(); got != 0 {
		t.Fatalf("violations = %d, want 0", got)
	}
}

// 端到端接线验证：ARANEA_ORCH_INVARIANT=1 时 Sequencer 必须把流经事件
// 送入断言器；违规事件照常发布（仅日志、不阻断）。
func TestSequencer_InvariantGate_EndToEnd(t *testing.T) {
	orig := invariantCheckEnabled
	invariantCheckEnabled = func() bool { return true }
	t.Cleanup(func() { invariantCheckEnabled = orig })

	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
		WithDeltaBatchInterval(time.Millisecond*4),
	)
	t.Cleanup(func() { _ = s.Close() })

	if s.invariant == nil {
		t.Fatal("invariant checker not wired despite gate enabled")
	}
	ctx := context.Background()
	sid := "s1"
	s.Publish(ctx, biz.NewTaskCreatedEvent(biz.Task{ID: "t1", SessionID: sid}))
	// 违规：step.completed 无对应 step.created。
	s.Publish(ctx, biz.NewStepCompletedEvent(biz.Step{ID: "ghost", TaskID: "t1", SpiritSessionID: sid}))
	if err := s.Flush(ctx); err != nil {
		t.Fatalf("Flush: %v", err)
	}
	if got := s.InvariantViolationCount(); got != 1 {
		t.Fatalf("InvariantViolationCount = %d, want 1", got)
	}
	// 不阻断：违规事件仍应到达 bus。
	found := false
	for _, e := range bus.pub {
		if e.EventKind() == biz.EventKindStepCompleted {
			found = true
		}
	}
	if !found {
		t.Fatal("violating event must still be published (log-only mode)")
	}
}

// 门控默认关闭：不启用时 Sequencer 不挂断言器。
func TestSequencer_InvariantGate_DefaultOff(t *testing.T) {
	orig := invariantCheckEnabled
	invariantCheckEnabled = func() bool { return false }
	t.Cleanup(func() { invariantCheckEnabled = orig })

	rs := &fakeRepoSet{}
	bus := &fakeBus{}
	s := NewSequencer(rs, bus, loggateway.NewNoop(),
		WithPublishBuffer(16),
		WithPersistBuffer(16),
	)
	t.Cleanup(func() { _ = s.Close() })
	if s.invariant != nil {
		t.Fatal("invariant checker must be nil when gate is off")
	}
	if got := s.InvariantViolationCount(); got != 0 {
		t.Fatalf("InvariantViolationCount = %d, want 0", got)
	}
}
