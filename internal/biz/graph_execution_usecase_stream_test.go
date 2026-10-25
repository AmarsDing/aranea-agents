package biz

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/logpipeline"
)

// ---------------------------------------------------------------------------
// D6（87-orchestration-chain-hardening 批次 A3）：graph 运行时一致性测试。
//
// F1: interrupt 暂停流不再被当作 done——observer.onComplete 改由 FSM 终态判定。
// F2: streamGen 防护对称补全——GraphDone/NodeStart/NodeEnd 同代际守卫。
// F3: 终态「先落库后外发」重排 + 落库有界重试与死信日志（AS-EVT-01）。
// ---------------------------------------------------------------------------

// streamCallLog 记录跨 fake（repo/sink/onComplete/taskCoord）的全局调用序，
// 用于断言 D6-F3 的「先落库后外发」顺序。
type streamCallLog struct {
	mu     sync.Mutex
	events []string
}

func (l *streamCallLog) add(ev string) {
	l.mu.Lock()
	l.events = append(l.events, ev)
	l.mu.Unlock()
}

func (l *streamCallLog) count(prefix string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	n := 0
	for _, ev := range l.events {
		if ev == prefix {
			n++
		}
	}
	return n
}

func (l *streamCallLog) indexOf(prefix string) int {
	l.mu.Lock()
	defer l.mu.Unlock()
	for i, ev := range l.events {
		if ev == prefix {
			return i
		}
	}
	return -1
}

func (l *streamCallLog) has(prefix string) bool { return l.indexOf(prefix) >= 0 }

// orderedRunRepo 记录每次 UpdateRun 的 status（写入共享 callLog），
// 可配置前 failRemaining 次 UpdateRun 失败（D6-F3 有界重试用例）。
type orderedRunRepo struct {
	log           *streamCallLog
	failRemaining int
	updateCalls   int
}

func (r *orderedRunRepo) SaveRun(context.Context, *GraphExecution) error { return nil }

func (r *orderedRunRepo) GetRun(context.Context, string) (*GraphExecution, error) {
	return nil, ErrNotFound
}

func (r *orderedRunRepo) ListRunsByGraph(context.Context, string, int, string, ...GraphRunListOption) ([]*GraphExecution, string, error) {
	return nil, "", nil
}

func (r *orderedRunRepo) UpdateRun(_ context.Context, exec *GraphExecution) error {
	r.updateCalls++
	r.log.add("update:" + exec.Status)
	if r.failRemaining > 0 {
		r.failRemaining--
		return errors.New("simulated UpdateRun failure")
	}
	return nil
}

// orderedSink 同时实现 GraphRunEventSink 与 GraphRunEventSinkOutput，
// 全部回调写入共享 callLog。
type orderedSink struct{ log *streamCallLog }

var (
	_ GraphRunEventSink       = (*orderedSink)(nil)
	_ GraphRunEventSinkOutput = (*orderedSink)(nil)
)

func (s *orderedSink) OnNodeStarted(context.Context, string, string, string, int) {
	s.log.add("sink:OnNodeStarted")
}

func (s *orderedSink) OnNodeCompleted(context.Context, string, string, string, int, string, string) {
	s.log.add("sink:OnNodeCompleted")
}

func (s *orderedSink) OnRunWaitingApproval(context.Context, string, string, string) {
	s.log.add("sink:OnRunWaitingApproval")
}

func (s *orderedSink) OnRunCompleted(context.Context, string, string, int64) {
	s.log.add("sink:OnRunCompleted")
}

func (s *orderedSink) OnRunFailed(context.Context, string, string, string) {
	s.log.add("sink:OnRunFailed")
}

func (s *orderedSink) OnRunCancelled(context.Context, string, string) {
	s.log.add("sink:OnRunCancelled")
}

func (s *orderedSink) OnRunOutput(context.Context, string, string, string, map[string]string) {
	s.log.add("sink:OnRunOutput")
}

// recordingTaskCoord 记录 OnGraphNodeStart 调用（D6-F2 stale 守卫断言用）。
type recordingTaskCoord struct{ starts int }

var _ GraphTaskCoordinator = (*recordingTaskCoord)(nil)

func (c *recordingTaskCoord) PublishGraphTaskStatus(context.Context, *GraphTask, map[string]any) {}
func (c *recordingTaskCoord) OnGraphNodeStart(context.Context, *GraphExecution, *NodeDef, NodeTaskMeta, string) error {
	c.starts++
	return nil
}
func (c *recordingTaskCoord) OnTaskCompleted(context.Context, *GraphTask) error { return nil }

// capturePipeline 捕获 loggateway 输出（D6-F3 死信日志断言用）。
type capturePipeline struct {
	mu      sync.Mutex
	entries []logpipeline.LogEntry
}

var _ logpipeline.Pipeline = (*capturePipeline)(nil)

func (p *capturePipeline) Emit(e logpipeline.LogEntry) {
	p.mu.Lock()
	p.entries = append(p.entries, e)
	p.mu.Unlock()
}
func (p *capturePipeline) AddSink(logpipeline.Sink)                    {}
func (p *capturePipeline) Close() error                                { return nil }
func (p *capturePipeline) Dropped() uint64                             { return 0 }
func (p *capturePipeline) Throttled() uint64                           { return 0 }
func (p *capturePipeline) Stats() logpipeline.PipelineStats            { return logpipeline.PipelineStats{} }
func (p *capturePipeline) SetThrottleRules([]logpipeline.ThrottleRule) {}

func (p *capturePipeline) hasStepAtLevel(stepID, level string) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, e := range p.entries {
		if e.StepID == stepID && e.Level == level {
			return true
		}
	}
	return false
}

func newStreamTestUsecase(repo GraphRunRepo, log *streamCallLog) (*GraphExecutionUsecase, *orderedSink) {
	uc := newTestGraphExecUsecase(repo)
	sink := &orderedSink{log: log}
	uc.runEventSink = sink
	return uc, sink
}

// ---------------------------------------------------------------------------
// D6-F1：observer.onComplete 由 FSM 终态判定
// ---------------------------------------------------------------------------

// F1: interrupt 使 exec 进入 waiting_human（非终态），消费循环退出后不得触发
// onComplete——投影器/遥测不在 interrupt 时提前收口，等真实终局一次性收口。
// 同时 waiting_human 快照必须已落库。
func TestConsumeRuntimeEvents_InterruptSkipsOnComplete(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f1-int", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 1)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphInterrupt, NodeID: "review-1", StepNumber: 1}
	close(eventCh)

	onCompleteCalls := 0
	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, func() { onCompleteCalls++ })

	if got := exec.GetStatus(); got != string(GraphExecWaitingHuman) {
		t.Fatalf("interrupt 后状态 = %q, want %q", got, GraphExecWaitingHuman)
	}
	if onCompleteCalls != 0 {
		t.Fatalf("waiting_human 非终态，onComplete 不得触发, got %d calls", onCompleteCalls)
	}
	if !log.has("update:" + string(GraphExecWaitingHuman)) {
		t.Fatal("UpdateRun 必须已持久化 waiting_human 快照")
	}
}

// F1+F3: done → completed 时 onComplete 恰好一次，且发生在终态 UpdateRun
// 与 OnRunCompleted sink 之后（先落库后外发，observer 最后收口）。
func TestConsumeRuntimeEvents_DoneFiresOnCompleteAfterPersist(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f1-done", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 2)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphNodeEnd, NodeID: "node-1", StepNumber: 1}
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphDone}
	close(eventCh)

	onCompleteCalls := 0
	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, func() {
		onCompleteCalls++
		log.add("onComplete")
	})

	if got := exec.GetStatus(); got != string(GraphExecCompleted) {
		t.Fatalf("done 后状态 = %q, want %q", got, GraphExecCompleted)
	}
	if onCompleteCalls != 1 {
		t.Fatalf("onComplete 必须恰好一次, got %d", onCompleteCalls)
	}
	updIdx := log.indexOf("update:" + string(GraphExecCompleted))
	sinkIdx := log.indexOf("sink:OnRunCompleted")
	completeIdx := log.indexOf("onComplete")
	if updIdx < 0 || sinkIdx < 0 {
		t.Fatalf("缺少终态 UpdateRun 或 OnRunCompleted: events=%v", log.events)
	}
	if !(updIdx < sinkIdx && sinkIdx < completeIdx) {
		t.Fatalf("顺序必须为 UpdateRun → OnRunCompleted → onComplete: events=%v", log.events)
	}
}

// F1: cancel 路径（Cancelled 是终态）流关闭后仍触发 onComplete 恰好一次，
// 保证 observer（projector/telemetry）解绑不泄漏。
func TestConsumeRuntimeEvents_CancelledStreamStillFiresOnComplete(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f1-cancel", "graph-1", "sess-1", string(GraphExecRunning))
	// 模拟 CancelExecution 已完成的终态收敛：Cancelled + FinishedAt。
	exec.execMu.Lock()
	if err := uc.applyExecTransition(exec, GraphExecEventCancel); err != nil {
		exec.execMu.Unlock()
		t.Fatalf("applyExecTransition(cancel): %v", err)
	}
	now := time.Now()
	exec.FinishedAt = &now
	exec.execMu.Unlock()

	// 被 cancel 的 runtime 关闭事件流（无事件）。
	eventCh := make(chan GraphRuntimeEvent)
	close(eventCh)

	onCompleteCalls := 0
	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, func() {
		onCompleteCalls++
		log.add("onComplete")
	})

	if onCompleteCalls != 1 {
		t.Fatalf("cancelled 是终态，onComplete 必须触发恰好一次, got %d", onCompleteCalls)
	}
	if log.has("sink:OnRunCompleted") || log.has("sink:OnRunFailed") {
		t.Fatalf("cancelled 不得外发完成/失败 sink: events=%v", log.events)
	}
}

// ---------------------------------------------------------------------------
// D6-F2：streamGen 防护对称补全（GraphDone/NodeStart/NodeEnd）
// ---------------------------------------------------------------------------

// F2: gen 失配的 NodeStart 不得改 CurrentNode，不得触发 taskCoord 看板与
// runEventSink 副作用；同窗（gen 相同）照常处理。
func TestUpdateExecutionFromRuntimeEvent_StaleStream_IgnoresNodeStart(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	coord := &recordingTaskCoord{}
	uc.taskCoord = coord
	exec := NewGraphExecution(context.Background(), "exec-f2-ns", "graph-1", "sess-1", string(GraphExecRunning))
	exec.streamGen = 2
	exec.execMu.Lock()
	exec.CurrentNode = "old-node"
	exec.execMu.Unlock()

	uc.updateExecutionFromRuntimeEvent(exec, 1, GraphRuntimeEvent{
		Type: DomainEventGraphNodeStart, NodeID: "node-1", StepNumber: 3,
	})

	if got := exec.CurrentNode; got != "old-node" {
		t.Fatalf("stale NodeStart 不得改写 CurrentNode, got %q", got)
	}
	if coord.starts != 0 {
		t.Fatalf("stale NodeStart 不得触发 taskCoord, got %d calls", coord.starts)
	}
	if log.has("sink:OnNodeStarted") {
		t.Fatal("stale NodeStart 不得外发 OnNodeStarted")
	}
}

// F2: gen 失配的 NodeEnd 不得向 Steps 追加幽灵步骤、不得 UpdateRun、不得外发 sink。
func TestUpdateExecutionFromRuntimeEvent_StaleStream_IgnoresNodeEnd(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f2-ne", "graph-1", "sess-1", string(GraphExecRunning))
	exec.streamGen = 2

	uc.updateExecutionFromRuntimeEvent(exec, 1, GraphRuntimeEvent{
		Type: DomainEventGraphNodeEnd, NodeID: "node-1", StepNumber: 1,
	})

	exec.execMu.RLock()
	steps := len(exec.Steps)
	exec.execMu.RUnlock()
	if steps != 0 {
		t.Fatalf("stale NodeEnd 不得追加 Steps, got %d", steps)
	}
	if repo.updateCalls != 0 {
		t.Fatalf("stale NodeEnd 不得 UpdateRun, got %d calls", repo.updateCalls)
	}
	if log.has("sink:OnNodeCompleted") {
		t.Fatal("stale NodeEnd 不得外发 OnNodeCompleted")
	}
}

// F2: gen 失配的 GraphDone 不得用旧流 FinalState 覆写新流 CurrentState。
func TestUpdateExecutionFromRuntimeEvent_StaleStream_IgnoresGraphDoneFinalState(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f2-done", "graph-1", "sess-1", string(GraphExecRunning))
	exec.streamGen = 2
	exec.execMu.Lock()
	exec.CurrentState = map[string]any{"keep": "new-stream"}
	exec.execMu.Unlock()

	uc.updateExecutionFromRuntimeEvent(exec, 1, GraphRuntimeEvent{
		Type:       DomainEventGraphDone,
		FinalState: map[string]any{"keep": "stale-stream", "ghost": true},
	})

	exec.execMu.RLock()
	got := exec.CurrentState
	exec.execMu.RUnlock()
	if got["keep"] != "new-stream" || len(got) != 1 {
		t.Fatalf("stale GraphDone 不得覆写 CurrentState, got %v", got)
	}
}

// F2 行为契约：waiting_human 期间允许同窗（gen 相同）的迟到 NodeEnd 照常落库——
// interrupt 前的已完成步骤不丢。
func TestUpdateExecutionFromRuntimeEvent_SameGenLateNodeEnd_PersistsStep(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f2-hitl", "graph-1", "sess-1", string(GraphExecRunning))
	exec.streamGen = 1
	// interrupt 后状态 waiting_human。
	exec.execMu.Lock()
	if err := uc.applyExecTransition(exec, GraphExecEventInterrupt); err != nil {
		exec.execMu.Unlock()
		t.Fatalf("applyExecTransition(interrupt): %v", err)
	}
	exec.execMu.Unlock()

	uc.updateExecutionFromRuntimeEvent(exec, 1, GraphRuntimeEvent{
		Type: DomainEventGraphNodeEnd, NodeID: "node-before-interrupt", StepNumber: 1,
	})

	exec.execMu.RLock()
	steps := len(exec.Steps)
	exec.execMu.RUnlock()
	if steps != 1 {
		t.Fatalf("同窗迟到 NodeEnd 必须照常落步骤, got %d steps", steps)
	}
	if repo.updateCalls == 0 {
		t.Fatal("同窗迟到 NodeEnd 必须照常 UpdateRun 落库")
	}
}

// ---------------------------------------------------------------------------
// D6-F3：终态「先落库后外发」+ 落库有界重试与死信日志
// ---------------------------------------------------------------------------

// F3: 终态 done 的顺序必须是 [UpdateRun(ok), OnRunCompleted, onComplete]。
func TestPersistBeforeSink_TerminalDone(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f3-done", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 1)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphDone}
	close(eventCh)

	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, func() { log.add("onComplete") })

	updIdx := log.indexOf("update:" + string(GraphExecCompleted))
	sinkIdx := log.indexOf("sink:OnRunCompleted")
	completeIdx := log.indexOf("onComplete")
	if updIdx < 0 {
		t.Fatalf("缺少终态 UpdateRun: events=%v", log.events)
	}
	if !(updIdx < sinkIdx && sinkIdx < completeIdx) {
		t.Fatalf("顺序必须为 UpdateRun → OnRunCompleted → onComplete: events=%v", log.events)
	}
}

// F3: UpdateRun 首次失败重试成功后，sink 仍精确外发一次。
func TestPersistRunWithRetry_FailThenSuccess(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log, failRemaining: 1}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f3-retry", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 1)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphDone}
	close(eventCh)

	onCompleteCalls := 0
	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, func() { onCompleteCalls++ })

	if repo.updateCalls != 2 {
		t.Fatalf("UpdateRun 必须重试一次成功: calls=%d, want 2", repo.updateCalls)
	}
	if got := log.count("sink:OnRunCompleted"); got != 1 {
		t.Fatalf("OnRunCompleted 必须精确一次, got %d", got)
	}
	if onCompleteCalls != 1 {
		t.Fatalf("onComplete 必须恰好一次, got %d", onCompleteCalls)
	}
}

// F3: UpdateRun 重试耗尽（1+2 次）仍失败 → 跳过终态 sink、记
// graph.record_dead_letter Error 日志，但 onComplete 仍触发（内存终态已收敛，
// observer 解绑属进程内事实）。
func TestTerminatePersistAllFail_SkipsSink(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log, failRemaining: 1 << 30}
	uc, _ := newStreamTestUsecase(repo, log)
	capture := &capturePipeline{}
	lg := loggateway.NewNoop()
	lg.SetPipeline(capture)
	uc.lg = lg
	exec := NewGraphExecution(context.Background(), "exec-f3-dead", "graph-1", "sess-1", string(GraphExecRunning))

	eventCh := make(chan GraphRuntimeEvent, 1)
	eventCh <- GraphRuntimeEvent{Type: DomainEventGraphDone}
	close(eventCh)

	onCompleteCalls := 0
	uc.consumeRuntimeEvents(eventCh, exec, 0, exec.ID, exec.GraphID, exec.SessionID, func() { onCompleteCalls++ })

	if repo.updateCalls != 3 {
		t.Fatalf("UpdateRun 必须有界重试 1+2 次: calls=%d, want 3", repo.updateCalls)
	}
	if log.has("sink:OnRunCompleted") || log.has("sink:OnRunOutput") {
		t.Fatalf("持久化重试耗尽后不得外发终态 sink: events=%v", log.events)
	}
	if !capture.hasStepAtLevel("graph.record_dead_letter", "error") {
		t.Fatal("必须记录 graph.record_dead_letter Error 日志")
	}
	if onCompleteCalls != 1 {
		t.Fatalf("持久化失败不影响进程内 observer 解绑: onComplete=%d, want 1", onCompleteCalls)
	}
	if got := exec.GetStatus(); got != string(GraphExecCompleted) {
		t.Fatalf("内存权威先行：终态已收敛 completed, got %q", got)
	}
}

// F3: interrupt 的 OnRunWaitingApproval 必须在 waiting_human 落库成功之后外发。
func TestInterruptPersistsBeforeWaitingApprovalSink(t *testing.T) {
	log := &streamCallLog{}
	repo := &orderedRunRepo{log: log}
	uc, _ := newStreamTestUsecase(repo, log)
	exec := NewGraphExecution(context.Background(), "exec-f3-hitl", "graph-1", "sess-1", string(GraphExecRunning))

	uc.updateExecutionFromRuntimeEvent(exec, 0, GraphRuntimeEvent{
		Type: DomainEventGraphInterrupt, NodeID: "review-1", StepNumber: 2,
	})

	updIdx := log.indexOf("update:" + string(GraphExecWaitingHuman))
	sinkIdx := log.indexOf("sink:OnRunWaitingApproval")
	if updIdx < 0 || sinkIdx < 0 {
		t.Fatalf("缺少 waiting_human UpdateRun 或 OnRunWaitingApproval: events=%v", log.events)
	}
	if updIdx >= sinkIdx {
		t.Fatalf("顺序必须为 UpdateRun(waiting_human) → OnRunWaitingApproval: events=%v", log.events)
	}
}
