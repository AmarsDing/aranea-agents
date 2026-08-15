package v2

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpcevent "trpc.group/trpc-go/trpc-agent-go/event"
	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// SequencerPublisher is the publish sink for v2 events.
// Implemented by *Sequencer and the test capturingSequencer.
type SequencerPublisher interface {
	Publish(ctx context.Context, e biz.Event)
}

// SeqAssigner allocates monotonic Seq values per spirit session.
// Implemented by *defaultSeqAssigner in this package.
type SeqAssigner interface {
	NextSeq(spiritSessionID string) int64
	// RestoreAtLeast raises the counter so the next NextSeq is > minSeq.
	// Used after process restart to avoid reusing Seq values already in DB (B-06).
	RestoreAtLeast(spiritSessionID string, minSeq int64)
}

// ActivityProjector projects runtime callbacks into v2 events.
//
// It owns the BeginStep lifecycle: a step is created via BeginStep, streamed
// via OnReasoningDelta/OnTextDelta, and finalized via OnReasoningDone/OnTextDone
// or OnToolResult. All events are constructed via biz factory functions
// (internal/biz/event_factory.go) — struct literals are forbidden because the
// event fields are unexported.
//
// The projector is single-threaded per turn: callers must not invoke callbacks
// for the same turn concurrently. The mutex only protects the activeStep map
// (which may be read by ProcessEvent from a different goroutine).
type ActivityProjector struct {
	seq         SequencerPublisher
	seqAsg      SeqAssigner
	lg          loggateway.Logger
	mu          sync.Mutex
	activeStep  map[string]*biz.Step // stepID → step
	activeTurn  map[string]*biz.Turn // turnID → turn (for OnTurnEnd)
	activeTask  map[string]*biz.Task // taskID → task (root only, for OnTurnEnd)
	stepCounter atomic.Int64
	// factory 是创建此 projector 的 ProjectorFactory 引用（可能为 nil）。
	// 用于 OnTurnEnd 查询 HasTeamDispatch 决定是否延迟 task.completed。
	// 2026-07-04 问题 P5/D1 修复。
	factory *ProjectorFactory

	// === Per-turn ProcessEvent state ===
	// meta is the ProjectMeta for the current turn, set by OnTurnStart.
	meta ProjectMeta
	// thinkingStepIDs maps agentKey → stepID of the current thinking step
	// (lazily created on the first reasoning delta, finalized on OnReasoningDone).
	//
	// 2026-07-04 问题 1 根因 2 修复：Graph 模式下多个 member agent 共享同一
	// turn，per-turn 单值 thinkingStepID 会导致后续 member 复用首个 member 的
	// thinking step。改为 per-agentKey map 让每个 member agent 拥有独立的
	// thinking step。
	thinkingStepIDs map[string]string
	// replyStepIDs maps agentKey → stepID of the current reply step
	// (lazily created on the first text delta, finalized on OnTextDone).
	// 同 thinkingStepIDs，per-agentKey 隔离避免 member 间复用。
	replyStepIDs map[string]string
	// toolCallSteps maps model tool_call_id → v2 stepID, for correlating
	// tool response events back to the action step created by OnToolCall.
	toolCallSteps map[string]string
	// memberToolCalls tracks per-member tool call counts observed during the
	// turn. Used by stream_consumer for team run step persistence.
	memberToolCalls map[string]int
}

// NewActivityProjector constructs an ActivityProjector.
// seq and seqAsg may be nil for compile-check scenarios (e.g. the Task 13
// skipped test); callbacks no-op when the sequencer is nil.
func NewActivityProjector(seq SequencerPublisher, seqAsg SeqAssigner, lg loggateway.Logger) *ActivityProjector {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ActivityProjector{
		seq:             seq,
		seqAsg:          seqAsg,
		lg:              lg.With(loggateway.Domain("projector_v2")),
		activeStep:      make(map[string]*biz.Step),
		activeTurn:      make(map[string]*biz.Turn),
		activeTask:      make(map[string]*biz.Task),
		thinkingStepIDs: make(map[string]string),
		replyStepIDs:    make(map[string]string),
		toolCallSteps:   make(map[string]string),
		memberToolCalls: make(map[string]int),
	}
}

// ProjectorFactory creates per-turn ActivityProjector instances that share
// the singleton Sequencer + SeqAssigner (so Seq allocation remains globally
// monotonic per spirit session) while isolating per-turn streaming state.
//
// 2026-07-04 问题 4 根因修复：spirit turn 和 team member turn 是并发关系
// （team AutoStart 通过 safego.Go 异步启动），共享单个 ActivityProjector
// 单例时，team member turn 的 Reset()+Configure() 会清空 spirit turn 进行
// 中的活跃状态（activeStep/activeTurn/activeTask/meta/thinkingStepIDs 等），
// 导致 spirit turn 后续事件（reply delta）被错误归因到 member session。
// 改为每次 turn 创建独立 Projector 实例，Sequencer 仍为单例（共享发布管道
// 与全局 SeqAssigner）。
//
// 2026-07-04 问题 P5/D1 修复：新增 teamDispatched 跟踪表，记录哪些 task
// 已派发了 team。OnTurnEnd 据此决定是否延迟 task.completed（等 synthesis
// turn 完成后再发）。
type ProjectorFactory struct {
	seq    SequencerPublisher
	seqAsg SeqAssigner
	lg     loggateway.Logger
	// taskReader 用于 synthesis/cancelled 兜底路径回读父 Task 的
	// CreatedAt/Seq/UserMessage/SessionID（见 terminalTask）。可为 nil
	// （测试/编译检查场景），读取失败时回退最小载荷。
	taskReader biz.TaskV2Reader
	// teamDispatched 跟踪哪些 taskID 已派发 team（system-push 模式）。
	// key=taskID, value=true。由 PlanExecutor.dispatchStep 调用 MarkTeamDispatched。
	// OnTurnEnd 检查此表决定是否延迟 task.completed。
	teamDispatched sync.Map
	// seqRestored tracks spirit sessions whose SeqAssigner was restored from DB
	// in this process (B-06). Avoids re-querying MaxSeq on every turn.
	seqRestored sync.Map
}

// NewProjectorFactory constructs a factory that produces per-turn
// ActivityProjector instances. seq and seqAsg are shared across all turns
// produced by this factory; taskReader is used by OnTurnEnd fallback paths to
// read back immutable parent-task fields; lg may be nil (defaults to Noop).
func NewProjectorFactory(seq SequencerPublisher, seqAsg SeqAssigner, taskReader biz.TaskV2Reader, lg loggateway.Logger) *ProjectorFactory {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &ProjectorFactory{seq: seq, seqAsg: seqAsg, taskReader: taskReader, lg: lg}
}

// MarkTeamDispatched 标记一个 task 已派发 team。
// 由 PlanExecutor.dispatchStep 在 Orchestrate 成功后调用。
// 2026-07-04 问题 P5/D1 修复：让 OnTurnEnd 知道此 task 有 team 在异步执行，
// 不应立即发 task.completed，而应等 synthesis turn 完成后再发。
func (f *ProjectorFactory) MarkTeamDispatched(taskID string) {
	if f == nil || taskID == "" {
		return
	}
	f.teamDispatched.Store(taskID, true)
}

// HasTeamDispatch 检查一个 task 是否已派发 team。
// 由 ActivityProjector.OnTurnEnd 调用决定是否延迟 task.completed。
func (f *ProjectorFactory) HasTeamDispatch(taskID string) bool {
	if f == nil || taskID == "" {
		return false
	}
	v, ok := f.teamDispatched.Load(taskID)
	return ok && v.(bool)
}

// ClearTeamDispatch 清除 task 的 team 派发标记。
// 由 synthesis turn 的 OnTurnEnd 在发出 task.completed 后调用。
func (f *ProjectorFactory) ClearTeamDispatch(taskID string) {
	if f == nil || taskID == "" {
		return
	}
	f.teamDispatched.Delete(taskID)
}

// NewProjector returns a fresh ActivityProjector bound to the factory's
// singleton Sequencer + SeqAssigner. Per-turn state (activeStep/meta/etc)
// is isolated per instance. Returns nil when the factory itself is nil
// (callers must handle nil gracefully).
func (f *ProjectorFactory) NewProjector() *ActivityProjector {
	if f == nil {
		return nil
	}
	p := NewActivityProjector(f.seq, f.seqAsg, f.lg)
	p.factory = f // 2026-07-04 问题 P5/D1: 注入 factory 引用供 OnTurnEnd 查询
	return p
}

// Seq returns the underlying SequencerPublisher (may be nil).
// Allows service-layer structs to publish v2 events via the sequencer
// (persist + WS) instead of bare eventBus.Publish (WS only).
// 2026-07-04 问题 C5 修复：暴露 seq 让 service 层的 Notice step 事件
// 经过 sequencer 持久化，避免刷新后丢失。
func (f *ProjectorFactory) Seq() SequencerPublisher {
	if f == nil {
		return nil
	}
	return f.seq
}

// RestoreSeqIfNeeded raises the shared SeqAssigner so the next NextSeq for
// spiritSessionID is > maxSeqFromDB. Idempotent per process/session (B-06).
func (f *ProjectorFactory) RestoreSeqIfNeeded(spiritSessionID string, maxSeqFromDB int64) {
	if f == nil || f.seqAsg == nil || spiritSessionID == "" || maxSeqFromDB <= 0 {
		return
	}
	if _, loaded := f.seqRestored.LoadOrStore(spiritSessionID, true); loaded {
		return
	}
	f.seqAsg.RestoreAtLeast(spiritSessionID, maxSeqFromDB)
}

// OnTurnStart emits task.created (root turns only) followed by turn.started.
// A "root turn" is one with an empty TeamStageID (i.e. the spirit-level turn,
// not a team member sub-turn). Root turns own the Task entity lifecycle.
//
// OnTurnStart also stores the ProjectMeta for ProcessEvent and resets per-turn
// ProcessEvent state (thinkingStepID, replyStepID, toolCallSteps).
//
// System-push continuation turns (meta.ParentTaskID != "") attach the new Turn
// to the existing Task ID without creating a new Task or emitting task events.
// The existing Task's state machine is owned by the original user-input turn.
func (p *ActivityProjector) OnTurnStart(ctx context.Context, meta ProjectMeta) {
	// Store meta + reset per-turn ProcessEvent state regardless of seq,
	// so that a projector with a nil sequencer (compile-check) still accepts
	// the configuration without panicking on a nil-map write below.
	p.meta = meta
	p.thinkingStepIDs = make(map[string]string)
	p.replyStepIDs = make(map[string]string)
	p.toolCallSteps = make(map[string]string)
	p.memberToolCalls = make(map[string]int)

	if p.seq == nil {
		return
	}
	// System-push continuation: attach the Turn to the existing Task without
	// creating a new Task or emitting task.created. The new Turn will be
	// parented under meta.ParentTaskID (resolved below).
	if meta.ParentTaskID != "" {
		// Point TaskID at the inherited parent so subsequent step/turn
		// emission correctly parents child steps under the existing Task.
		p.meta.TaskID = meta.ParentTaskID
		meta.TaskID = meta.ParentTaskID
	} else if meta.TeamStageID == "" {
		task := meta.newTask(meta.TaskID, biz.TaskStatusRunning, meta.TaskContent)
		p.mu.Lock()
		p.activeTask[meta.TaskID] = &task
		p.mu.Unlock()
		p.seq.Publish(ctx, biz.NewTaskCreatedEvent(task))
	}
	var seq int64
	if p.seqAsg != nil {
		seq = p.seqAsg.NextSeq(meta.SpiritSessionID)
	}
	turn := meta.newTurn(meta.TurnID, seq)
	p.mu.Lock()
	p.activeTurn[meta.TurnID] = &turn
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewTurnStartedEvent(turn))
}

// Configure sets the ProjectMeta for the current turn WITHOUT emitting events.
// Used by chat_orchestrator to pre-configure the projector before LLM invocation
// so that plugins/hooks can emit notice/confirm events during the call.
// OnTurnStart (called later by the stream consumer) will emit task.created and
// turn.started events and reset per-turn streaming state.
func (p *ActivityProjector) Configure(meta ProjectMeta) {
	p.meta = meta
}

// === ActivityEmitter interface (biz.ActivityEmitter) ===
//
// These methods allow plugins/hooks (cost_guard, model_router, tool_confirmation)
// to emit notice/confirm events via the projector without importing the agent
// package. The projector is injected into the context via biz.WithActivityEmitter.

// EmitNotice creates a notice step and immediately completes it. The step
// carries NoticeType metadata for frontend rendering. Implements biz.ActivityEmitter.
func (p *ActivityProjector) EmitNotice(ctx context.Context, content, noticeType string) error {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return nil
	}
	// Inline step creation (instead of BeginStep) so NoticeType is set before
	// the StepCreatedEvent is published — the frontend needs it for rendering.
	// Seq is allocated immediately from SeqAssigner to ensure notice steps get
	// a monotonic Seq (consistent with BeginStep) for correct frontend ordering.
	n := p.stepCounter.Add(1)
	stepID := p.meta.TurnID + "-s" + strconv.Itoa(int(n))
	var seq int64
	if p.seqAsg != nil {
		seq = p.seqAsg.NextSeq(p.meta.SpiritSessionID)
	}
	step := p.meta.newStep(stepID, biz.StepKindNotice, seq)
	step.NoticeType = noticeType
	step.Content = content
	p.mu.Lock()
	p.activeStep[stepID] = &step
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCreatedEvent(step))
	p.completeStep(ctx, stepID, "", nil)
	return nil
}

// EmitConfirmRequest creates a confirm step with status=tool_blocked and
// returns the step ID for later result correlation via EmitConfirmResult.
// The step stays in tool_blocked until the user responds. Implements
// biz.ActivityEmitter.
func (p *ActivityProjector) EmitConfirmRequest(ctx context.Context, params biz.ActivityConfirmParams) (string, error) {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return "", nil
	}
	stepID := p.BeginStep(p.meta, biz.StepKindConfirm)
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok {
		step.ToolName = params.ToolName
		step.ToolArgs = sanitizeRawJSON([]byte(params.ToolArguments))
		step.Content = params.Content
		step.Danger = params.Danger
		step.Status = biz.StepStatusToolBlocked
		// Team mode: attribute the confirm step to the member agent that
		// triggered it (hook passes its own agent key); otherwise the step
		// would inherit the anchor key from the base meta and the frontend
		// could not attach it to the member's activity panel.
		if k := strings.TrimSpace(params.AuthorAgentKey); k != "" {
			step.AuthorAgentKey = k
		}
		step.Version++
	}
	p.mu.Unlock()
	if ok {
		p.seq.Publish(ctx, biz.NewStepUpdatedEvent(*step))
	}
	return stepID, nil
}

// EmitConfirmResult updates a confirm step with the user's response:
// approved → completed, denied → cancelled. Returns an error if the step ID
// is not found or is not a confirm step. Implements biz.ActivityEmitter.
func (p *ActivityProjector) EmitConfirmResult(ctx context.Context, stepID string, approved bool) error {
	if p == nil || p.seq == nil {
		return nil
	}
	now := time.Now()
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("confirm step not found: %s", stepID)
	}
	if step.Kind != biz.StepKindConfirm {
		p.mu.Unlock()
		return fmt.Errorf("expected confirm kind, got %s", step.Kind)
	}
	if approved {
		step.Status = biz.StepStatusCompleted
	} else {
		step.Status = biz.StepStatusCancelled
	}
	step.CompletedAt = &now
	step.Version++
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
	return nil
}

// ConfirmTimeoutErrorCode is the ToolErrorCode set on confirm steps that
// expired without a user response. The frontend renders these as "已超时"
// instead of "已拒绝".
const ConfirmTimeoutErrorCode = "confirm_timeout"

// EmitConfirmTimeout marks a confirm step as cancelled due to confirmation
// deadline expiry. Implements biz.ActivityEmitter.
func (p *ActivityProjector) EmitConfirmTimeout(ctx context.Context, stepID string) error {
	if p == nil || p.seq == nil {
		return nil
	}
	now := time.Now()
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if !ok {
		p.mu.Unlock()
		return fmt.Errorf("confirm step not found: %s", stepID)
	}
	if step.Kind != biz.StepKindConfirm {
		p.mu.Unlock()
		return fmt.Errorf("expected confirm kind, got %s", step.Kind)
	}
	step.Status = biz.StepStatusCancelled
	step.ToolErrorCode = ConfirmTimeoutErrorCode
	step.CompletedAt = &now
	step.Version++
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
	return nil
}

// compile-time interface check
var _ biz.ActivityEmitter = (*ActivityProjector)(nil)

// === Tier 2: v1 parity callbacks ===

// stuckToolErrorCode is the ToolErrorCode set on action steps that never
// received a tool_result when the turn finalizes (OnStuckTools).
const stuckToolErrorCode = "tool_timeout"

// toolLimitMsgFragment 是 vendored 框架 functioncall 处理器在达到
// MaxToolIterations 时发出的 flow_error 消息片段（"max tool iterations
// (%d) exceeded"）。镜像 graph/adapter/agent_summary_fallback.go 的
// isToolLimitTerminalEvent——两处判定同一框架事件，语义必须一致。
const toolLimitMsgFragment = "max tool iterations"

// toolLimitUndispatchedNote 是工具上限硬停时关闭未执行调用步骤的说明文案。
const toolLimitUndispatchedNote = "达到工具调用次数上限，本次调用未实际执行"

// isToolLimitTerminalEvent reports whether ev is the framework's
// tool-iteration hard-stop event (flow_error, "max tool iterations …").
func isToolLimitTerminalEvent(ev *trpcevent.Event) bool {
	if ev == nil || ev.Response == nil || ev.Response.Error == nil {
		return false
	}
	return ev.Response.Object == trpcmodel.ObjectTypeError &&
		ev.Response.Error.Type == trpcmodel.ErrorTypeFlowError &&
		strings.Contains(ev.Response.Error.Message, toolLimitMsgFragment)
}

// OnError marks the root Task as failed and creates an error Step carrying
// the error message + classification. If no root task exists (error before
// OnTurnStart), the error is logged but no event is emitted.
//
// System-push continuation turns (ParentTaskID set) skip the task.failed
// emission — the original Task's state machine is owned by the original
// user-input turn, and a synthesis-trigger failure should not mark the user's
// original Task as failed (team execution results remain valid).
//
// The error Step uses Kind=error with Content=errMsg and ToolErrorCode=errType,
// providing the frontend with error details without requiring schema changes
// to the Task entity (v2 architecture: everything is a Step).
func (p *ActivityProjector) OnError(ctx context.Context, errMsg, errType, errCode string) {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return
	}
	now := time.Now()
	p.mu.Lock()
	task, ok := p.activeTask[p.meta.TaskID]
	if ok {
		task.Status = biz.TaskStatusFailed
		task.CompletedAt = &now
		task.UpdatedAt = now
		task.Version++
		delete(p.activeTask, p.meta.TaskID)
	}
	p.mu.Unlock()
	// System-push continuation: skip task.failed emission (preserve original
	// Task's state machine). Still emit the error Step below so the frontend
	// surfaces the synthesis failure.
	if p.meta.ParentTaskID != "" {
		p.lg.Info("projector_v2: OnError in system-push turn, skipping task.failed",
			loggateway.Str("parent_task_id", p.meta.ParentTaskID),
			loggateway.Str("err_msg", errMsg),
			loggateway.Str("err_type", errType))
	} else if !ok {
		p.lg.Warn("projector_v2: OnError with no active task",
			loggateway.Str("task_id", p.meta.TaskID),
			loggateway.Str("err_msg", errMsg),
			loggateway.Str("err_type", errType))
	}
	// Emit error Step (carries error details for the frontend).
	// Seq is allocated immediately from SeqAssigner so error steps sort after
	// prior steps (thinking/action/reply) in the frontend's Seq ASC ordering.
	n := p.stepCounter.Add(1)
	errStepID := p.meta.TurnID + "-s" + strconv.Itoa(int(n))
	var errSeq int64
	if p.seqAsg != nil {
		errSeq = p.seqAsg.NextSeq(p.meta.SpiritSessionID)
	}
	errStep := p.meta.newStep(errStepID, biz.StepKindError, errSeq)
	errStep.Content = errMsg
	errStep.ToolErrorCode = errType
	if errCode != "" && errType == "" {
		errStep.ToolErrorCode = errCode
	}
	errStep.Status = biz.StepStatusCompleted
	errStep.CompletedAt = &now
	errStep.Version++
	p.mu.Lock()
	p.activeStep[errStepID] = &errStep
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCreatedEvent(errStep))
	p.seq.Publish(ctx, biz.NewStepCompletedEvent(errStep))
	// Safety-limit hard stop (framework MaxLLMCalls StopError killed the turn
	// before the model could produce a final summary): emit a fallback final
	// reply step so the user sees a graceful closing message instead of a
	// bare hard stop.
	if isSafetyLimitHardStop(errType, errMsg) {
		p.emitSafetyLimitFallbackReply(ctx, now)
	}
	// Emit TaskFailedEvent (carries the Task with status=failed).
	// Skipped for system-push continuation turns (ParentTaskID set) — the
	// original Task's state machine is owned by the original user-input turn.
	if p.meta.ParentTaskID == "" && task != nil {
		p.seq.Publish(ctx, biz.NewTaskFailedEvent(*task))
	}
}

// safetyLimitStopMsgPrefix is the message prefix produced by the vendored
// framework's Invocation.IncLLMCallCount StopError
// (pkg/trpc-agent-go/agent/invocation.go). A stop_agent_error carrying this
// message means the turn was hard-stopped by the MaxLLMCalls safety limit
// before the model could produce a final summary. Intentional stops (ralph
// loop completion, plugin/human interrupts) carry different messages.
const safetyLimitStopMsgPrefix = "max LLM calls"

// safetyLimitFallbackContent is the user-visible closing text of the fallback
// reply emitted when a safety-limit hard stop prevented a final summary.
const safetyLimitFallbackContent = "本轮回复已达到 LLM 调用次数安全上限，已提前收尾。以上为本轮已产生的部分内容；如需继续，请发送新消息。"

// isSafetyLimitHardStop reports whether an error routed to OnError is the
// framework's MaxLLMCalls hard stop (vs. an intentional controlled stop).
func isSafetyLimitHardStop(errType, errMsg string) bool {
	return errType == trpcagent.ErrorTypeStopAgentError && strings.HasPrefix(errMsg, safetyLimitStopMsgPrefix)
}

// emitSafetyLimitFallbackReply emits a completed, final reply step carrying a
// graceful closing message. The step sorts after the error step via a fresh
// Seq from the SeqAssigner.
func (p *ActivityProjector) emitSafetyLimitFallbackReply(ctx context.Context, now time.Time) {
	n := p.stepCounter.Add(1)
	stepID := p.meta.TurnID + "-s" + strconv.Itoa(int(n))
	var seq int64
	if p.seqAsg != nil {
		seq = p.seqAsg.NextSeq(p.meta.SpiritSessionID)
	}
	step := p.meta.newStep(stepID, biz.StepKindReply, seq)
	step.Content = safetyLimitFallbackContent
	step.IsFinal = true
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	step.Version++
	p.mu.Lock()
	p.activeStep[stepID] = &step
	p.mu.Unlock()
	p.lg.Warn("safety limit hard stop, fallback final reply emitted",
		loggateway.StepID("agent.v2.projector.safety_limit_fallback"),
		loggateway.Str("task_id", p.meta.TaskID),
		loggateway.Str("step_id", stepID))
	// K3 降级：流程日志（ctx 无 TraceEmitter 时静默跳过，同 llm_caller 约定）。
	if em := event.TraceEmitterFromContext(ctx); em != nil {
		em.LogWarn("chat.turn.safety_limit_stop", "触发安全调用上限，降级收尾",
			"本轮回复达到 LLM 调用次数安全上限，已生成兜底收尾消息",
			event.P("task_id", p.meta.TaskID),
			event.P("fallback_step_id", stepID))
	}
	p.seq.Publish(ctx, biz.NewStepCreatedEvent(step))
	p.seq.Publish(ctx, biz.NewStepCompletedEvent(step))
}

// OnStuckTools marks all tool_running steps as failed. Called from
// OnTurnEndEnhanced when the turn ends with pending tool calls that never
// received a tool_result.
func (p *ActivityProjector) OnStuckTools(ctx context.Context) {
	if p == nil || p.seq == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	var stuck []*biz.Step
	for id, step := range p.activeStep {
		if step.Status == biz.StepStatusToolRunning {
			step.Status = biz.StepStatusFailed
			step.CompletedAt = &now
			step.ToolErrorCode = stuckToolErrorCode
			step.Version++
			stuck = append(stuck, step)
			delete(p.activeStep, id)
		}
	}
	p.mu.Unlock()
	for _, step := range stuck {
		p.lg.Warn("stuck tool detected at turn finalization",
			loggateway.StepID("agent.v2.projector.stuck_tool"),
			loggateway.Str("step_id", step.ID),
			loggateway.Str("tool_name", step.ToolName),
		)
		p.seq.Publish(ctx, biz.NewStepFailedEvent(*step))
	}
}

// processToolLimitHardStop 处理工具迭代上限硬停事件（2026-08-15）：框架在
// 达到 MaxToolIterations 时拒绝派发本批 tool_calls 并直接终止循环——这些
// 调用永远不会产生 tool_result。它们不是「执行失败」：按失败投影会留下
// 幻影故障证据（OnStuckTools 补标 tool_timeout + OnError 落 error step），
// 被交付物质量门 J4（MemberExecutionEvidence）误判为成员执行失败，把已被
// summaryFallbackAgent 兜底完成的团队打回修订。此处按真实语义投影：
//  1. 关闭全部 tool_running steps 为 completed（Content 注明「未执行」）；
//  2. 以 notice（非 error）记录终止原因，不 fail 根 Task——图节点侧的
//     summaryFallbackAgent 会追加兜底总结事件，让 turn 正常完成。
//
// 序列保证：agent 循环是串行的（LLM 响应 → 派发工具 → 收结果 → 下一次
// LLM 调用），硬停事件到达时该成员处于 tool_running 的步骤恰好就是被拒
// 派发的本批调用；真实已派发但悬挂的工具不会走到这里（其循环不会推进到
// 下一次 LLM 响应）。关闭按事件作者（ProcessEvent 已把 p.meta.AgentKey
// 切换为终止事件所属成员）限定范围——graph 并行成员共享同一 projector，
// 不得误关其他成员正在执行的工具步骤。
func (p *ActivityProjector) processToolLimitHardStop(ctx context.Context, message string) {
	now := time.Now()
	authorKey := p.meta.AgentKey
	p.mu.Lock()
	var closed []*biz.Step
	for id, step := range p.activeStep {
		if step.Status != biz.StepStatusToolRunning {
			continue
		}
		if step.AuthorAgentKey != authorKey {
			continue
		}
		step.Status = biz.StepStatusCompleted
		step.Content = toolLimitUndispatchedNote
		step.CompletedAt = &now
		step.Version++
		closed = append(closed, step)
		delete(p.activeStep, id)
	}
	if len(closed) > 0 {
		closedIDs := make(map[string]struct{}, len(closed))
		for _, s := range closed {
			closedIDs[s.ID] = struct{}{}
		}
		// 清掉 toolCallID → stepID 映射：这些调用不会收到 tool_result，
		// 残留映射会让迟到的同名结果事件查到已关闭步骤。
		for callID, stepID := range p.toolCallSteps {
			if _, ok := closedIDs[stepID]; ok {
				delete(p.toolCallSteps, callID)
			}
		}
	}
	p.mu.Unlock()
	for _, step := range closed {
		p.lg.Info("工具上限硬停：关闭未执行的工具调用步骤",
			loggateway.StepID("agent.v2.projector.tool_limit_close"),
			loggateway.Str("step_id", step.ID),
			loggateway.Str("tool_name", step.ToolName),
		)
		p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
	}
	_ = p.EmitNotice(ctx, message, "tool_iteration_limit")
}

// EmitSystemEvent emits a notice step for system-level notifications
// (e.g. context_usage token counts). Delegates to EmitNotice with noticeType
// derived from meta["type"] or the content itself. The kind parameter is
// accepted for v1 compatibility but v2 always uses StepKindNotice.
func (p *ActivityProjector) EmitSystemEvent(ctx context.Context, kind biz.ActivityKind, content string, meta map[string]any) {
	if p == nil || p.seq == nil || p.meta.TaskID == "" {
		return
	}
	noticeType := content
	if t, ok := meta["type"].(string); ok && t != "" {
		noticeType = t
	}
	contentText := content
	if contentText == "" {
		if data, err := json.Marshal(meta); err == nil {
			contentText = string(data)
		}
	}
	_ = p.EmitNotice(ctx, contentText, noticeType)
}

// MemberToolCalls returns per-member tool call counts observed during the
// turn. Used by stream_consumer for team run step persistence.
func (p *ActivityProjector) MemberToolCalls() map[string]int {
	p.mu.Lock()
	defer p.mu.Unlock()
	if len(p.memberToolCalls) == 0 {
		return nil
	}
	out := make(map[string]int, len(p.memberToolCalls))
	for k, v := range p.memberToolCalls {
		out[k] = v
	}
	return out
}

// Close is a no-op for the singleton v2 projector. Per-turn cleanup is
// handled by OnTurnEndEnhanced and Reset. The v2 Sequencer is a singleton
// and must NOT be closed per-turn (it would break other concurrent turns).
func (p *ActivityProjector) Close() {
	// no-op: sequencer is a singleton, not closed per-turn
}

// ActivityUsage holds token usage for a turn. Mirrors agent.ActivityUsage.
type ActivityUsage struct {
	PromptTokens     int
	CompletionTokens int
	TotalTokens      int
}

// OnTurnEndEnhanced is the v1-compatible OnTurnEnd that also handles stuck
// tools, token usage, and cancellation. This is the method the stream_consumer
// should call instead of OnTurnEnd.
//
// Sequence: OnStuckTools → finalize remaining active steps → OnTurnEnd.
// Usage is NOT attached to the Task entity (the stream_consumer attaches it
// to EventStreamResult, which is consumed by the team runner for persistence).
func (p *ActivityProjector) OnTurnEndEnhanced(ctx context.Context, meta ProjectMeta, usage *ActivityUsage, canceled bool) {
	if p == nil || p.seq == nil {
		return
	}
	// 1. Mark stuck tools as failed (before finalizing turn).
	p.OnStuckTools(ctx)

	// 2. Finalize remaining active steps with terminal status.
	now := time.Now()
	terminalStatus := biz.StepStatusCompleted
	if canceled {
		terminalStatus = biz.StepStatusCancelled
	}
	p.mu.Lock()
	var remaining []*biz.Step
	for id, step := range p.activeStep {
		step.Status = terminalStatus
		step.CompletedAt = &now
		step.Version++
		remaining = append(remaining, step)
		delete(p.activeStep, id)
	}
	p.mu.Unlock()
	for _, step := range remaining {
		p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
	}

	// 3. Emit turn.completed + task.completed (delegates to OnTurnEnd).
	// P1-02 fix: propagate the canceled flag so the Turn/Task entities receive
	// the correct terminal status (Cancelled) instead of Completed.
	p.OnTurnEnd(ctx, meta, canceled)

	// Usage is intentionally not attached to the Task entity — the
	// stream_consumer attaches usage to EventStreamResult for team run
	// persistence. This keeps the Task struct lean.
	_ = usage
}

// BeginStep creates a new step of the given kind, stores it in the active map,
// emits step.created, and returns the step ID.
//
// The step ID is derived from the turn ID with a "-s<N>" suffix where N is a
// per-projector counter. The step's Seq is allocated immediately from the
// SeqAssigner so that all step kinds (thinking/action/reply/notice/confirm/error)
// have a monotonic Seq for correct frontend ordering. Previously Seq was
// lazily assigned only for streaming steps (thinking/reply), causing action/
// notice steps to have Seq=0 and sort before thinking steps.
func (p *ActivityProjector) BeginStep(meta ProjectMeta, kind biz.StepKind) string {
	if p.seq == nil {
		return ""
	}
	n := p.stepCounter.Add(1)
	stepID := meta.TurnID + "-s" + strconv.Itoa(int(n))
	var seq int64
	if p.seqAsg != nil {
		seq = p.seqAsg.NextSeq(meta.SpiritSessionID)
	}
	step := meta.newStep(stepID, kind, seq)
	p.mu.Lock()
	p.activeStep[stepID] = &step
	p.mu.Unlock()
	p.seq.Publish(context.Background(), biz.NewStepCreatedEvent(step))
	return stepID
}

// OnReasoningDelta emits a step.streaming event with DeltaField="reasoning".
// The fourth parameter is reserved for future metadata and currently unused.
func (p *ActivityProjector) OnReasoningDelta(ctx context.Context, stepID, delta, _ string) {
	p.emitStreaming(ctx, stepID, "reasoning", delta)
}

// OnTextDelta emits a step.streaming event with DeltaField="content".
// The fourth parameter is reserved for future metadata and currently unused.
func (p *ActivityProjector) OnTextDelta(ctx context.Context, stepID, delta, _ string) {
	p.emitStreaming(ctx, stepID, "content", delta)
}

// emitStreaming looks up the active step and publishes a StepStreamingEvent.
// Seq is allocated upfront in BeginStep (no lazy allocation here).
// No-op if the step is unknown.
func (p *ActivityProjector) emitStreaming(ctx context.Context, stepID, field, delta string) {
	if p.seq == nil {
		return
	}
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	p.mu.Unlock()
	if !ok {
		return
	}
	p.seq.Publish(ctx, biz.NewStepStreamingEvent(step.SpiritSessionID, step.TaskID, stepID, field, delta))
}

// OnReasoningDone finalizes the reasoning content and completes the step.
func (p *ActivityProjector) OnReasoningDone(ctx context.Context, stepID, finalContent string) {
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.Reasoning = finalContent
	}
	p.mu.Unlock()
	p.completeStep(ctx, stepID, "", nil)
}

// OnTextDone finalizes the reply content and completes the step.
func (p *ActivityProjector) OnTextDone(ctx context.Context, stepID, finalContent string, isFinal bool) {
	// Strip memory <fact> machine-extraction tags (prompt.go convention) from
	// the user-visible reply before persisting. Fact extraction itself happens
	// upstream (v1 orchestrator immediateFactWriter); here we only guarantee
	// that stored/streamed reply content never carries raw tags — the v1
	// pipeline applied the same discipline to its (now removed) message store.
	finalContent = biz.StripFactMarks(finalContent)
	p.mu.Lock()
	if step, ok := p.activeStep[stepID]; ok {
		step.IsFinal = isFinal
	}
	p.mu.Unlock()
	p.completeStep(ctx, stepID, finalContent, nil)
}

// OnToolCall ensures an action step exists for the turn (creating one via
// BeginStep if needed), records the tool name/args, transitions it to
// ToolRunning, and emits step.updated.
func (p *ActivityProjector) OnToolCall(ctx context.Context, meta ProjectMeta, toolName string, args json.RawMessage) string {
	if p.seq == nil {
		return ""
	}
	stepID := p.findOrCreateActionStep(meta)
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if ok {
		step.ToolName = toolName
		step.ToolArgs = sanitizeRawJSON(args)
		step.Status = biz.StepStatusToolRunning
		step.Version++
	}
	// Track per-member tool calls for team run step persistence.
	if meta.TeamStageID != "" && meta.AgentKey != "" {
		p.memberToolCalls[meta.AgentKey]++
	}
	p.mu.Unlock()
	if ok {
		p.seq.Publish(ctx, biz.NewStepUpdatedEvent(*step))
	}
	return stepID
}

// findOrCreateActionStep returns the ID of an existing action step for the
// turn and agent, or creates a new one via BeginStep. Must be called without holding p.mu.
//
// 2026-07-04 问题 1 根因 2 修复：Graph 模式下多个 member agent 共享同一
// turn，原逻辑只按 TurnID + Kind 查找会导致后续 member 复用首个 member 的
// action step。增加 AuthorAgentKey 维度让每个 member agent 拥有独立的
// action step。
func (p *ActivityProjector) findOrCreateActionStep(meta ProjectMeta) string {
	p.mu.Lock()
	for id, s := range p.activeStep {
		if s.TurnID == meta.TurnID && s.Kind == biz.StepKindAction && s.AuthorAgentKey == meta.AgentKey {
			p.mu.Unlock()
			return id
		}
	}
	p.mu.Unlock()
	return p.BeginStep(meta, biz.StepKindAction)
}

// OnToolResult completes (success) or fails (error) the action step.
func (p *ActivityProjector) OnToolResult(ctx context.Context, stepID string, result json.RawMessage, err error) {
	if err != nil {
		p.failStep(ctx, stepID, err)
		return
	}
	p.completeStep(ctx, stepID, "", result)
}

// OnTurnEnd emits turn.completed and, for root turns, task.completed.
//
// The canceled flag controls the terminal status applied to the Turn and Task
// entities. When false (normal completion) the entities are marked Completed;
// when true (user-initiated cancellation) they are marked Cancelled. The event
// kind (TurnCompletedEvent / TaskCompletedEvent) is reused in both cases — it
// represents "lifecycle ended" while the entity.Status field carries the
// actual terminal state. This avoids introducing a parallel Cancelled event
// type and keeps the frontend terminal-event handling uniform.
//
// System-push continuation turns (meta.ParentTaskID != "") emit turn.completed
// only — the original Task's state machine is owned by the original user-input
// turn and is not touched here. Exception: synthesis continuation turns
// (TeamStageID == "") close the parent Task.
//
// 2026-07-04 问题 P5/D1 修复：Spirit 等待 team 完成后再关闭 Task。
// - Root turn（ParentTaskID=="" && TeamStageID==""）：
//   - 若 HasTeamDispatch(meta.TaskID) == true 且未取消 → 跳过 task.completed，
//     Task 保持 Running，等 synthesis turn 完成后再发 task.completed。
//   - 若已取消 → 立即发 task.completed（Status=Cancelled），不等待 synthesis turn。
//   - 否则 → 发 task.completed（原行为，无 team 派发的普通对话）。
//
// - Synthesis continuation turn（ParentTaskID!="" && TeamStageID==""）：
//   - 发 task.completed for ParentTaskID，并 ClearTeamDispatch。
//   - 原 behavior 是跳过所有 task 事件，现在需要补发 task.completed。
func (p *ActivityProjector) OnTurnEnd(ctx context.Context, meta ProjectMeta, canceled bool) {
	if p.seq == nil {
		return
	}
	now := time.Now()
	// P1-02 fix: select terminal status based on the canceled flag.
	turnStatus := biz.TurnStatusCompleted
	taskStatus := biz.TaskStatusCompleted
	if canceled {
		turnStatus = biz.TurnStatusCancelled
		taskStatus = biz.TaskStatusCancelled
	}
	p.mu.Lock()
	turn, ok := p.activeTurn[meta.TurnID]
	if ok {
		turn.CompletedAt = &now
		turn.Status = turnStatus
		turn.Version++
		delete(p.activeTurn, meta.TurnID)
	}
	p.mu.Unlock()
	if ok {
		p.seq.Publish(ctx, biz.NewTurnCompletedEvent(*turn))
	}
	// 2026-07-04 问题 P5/D1 修复：synthesis continuation turn 发 task.completed。
	// synthesis turn 是 system-push 的第二 turn（ParentTaskID!="" 且 TeamStageID==""），
	// 由 checkAllTeamsCompleted 触发，所有 team 已完成，此时关闭根 Task。
	if meta.ParentTaskID != "" {
		if meta.TeamStageID == "" {
			// Synthesis turn: complete (or cancel) the parent task.
			// 注意：synthesis turn 的 projector 是新实例，activeTask 中没有
			// parent task（它在 root turn 的 projector 中）。terminalTask 回读
			// DB 保留 CreatedAt/Seq/UserMessage。
			task := p.terminalTask(ctx, meta.ParentTaskID, meta.SpiritSessionID, taskStatus, now)
			p.seq.Publish(ctx, biz.NewTaskCompletedEvent(task))
			if p.factory != nil {
				p.factory.ClearTeamDispatch(meta.ParentTaskID)
			}
		}
		return
	}
	if meta.TeamStageID == "" {
		// 2026-07-04 问题 P5/D1 修复：若此 task 派发了 team，延迟 task.completed。
		// Task 保持 Running，等 synthesis turn 完成后再发 task.completed。
		// P1-02 fix: when canceled, do not delay — emit task.completed
		// (Status=Cancelled) immediately. A cancelled run will not produce
		// a synthesis turn, so waiting would leak a Running task forever.
		if !canceled && p.factory != nil && p.factory.HasTeamDispatch(meta.TaskID) {
			p.lg.Info("OnTurnEnd: task 已派发 team，延迟 task.completed（等 synthesis turn）",
				loggateway.Str("task_id", meta.TaskID),
				loggateway.Str("turn_id", meta.TurnID),
			)
			// 不删除 activeTask，不发 task.completed。
			// Task 保持 Running 状态（task.created 已设置）。
			return
		}
		p.mu.Lock()
		task, tok := p.activeTask[meta.TaskID]
		if tok {
			task.Status = taskStatus
			task.CompletedAt = &now
			task.Version++
			delete(p.activeTask, meta.TaskID)
		}
		p.mu.Unlock()
		if tok {
			p.seq.Publish(ctx, biz.NewTaskCompletedEvent(*task))
		} else if canceled {
			// Cancelled root turn whose task is not in activeTask (e.g. team
			// dispatch path where the task was already removed). terminalTask
			// 回读 DB 保留 CreatedAt/Seq/UserMessage。
			task := p.terminalTask(ctx, meta.TaskID, meta.SpiritSessionID, taskStatus, now)
			p.seq.Publish(ctx, biz.NewTaskCompletedEvent(task))
		}
	}
}

// terminalTask constructs the task.completed payload for paths where the task
// is not in activeTask (synthesis continuation / cancelled fallback). Fields
// immutable across the task lifecycle (CreatedAt/Seq/UserMessage/SessionID)
// are read back from the DB so the event payload doesn't carry zero values:
// a zero CreatedAt serializes to "0001-01-01T00:00:00Z" (truthy), defeating
// the frontend merge guard (t.CreatedAt || ex.CreatedAt) and corrupting the
// creation time display ("01-01 08:05"). Best-effort: reader failure keeps
// the minimal payload (frontend must then rely on its own zero-time guard).
func (p *ActivityProjector) terminalTask(ctx context.Context, taskID, sessionID string, status biz.TaskStatus, now time.Time) biz.Task {
	task := biz.Task{
		ID:          taskID,
		SessionID:   sessionID,
		Status:      status,
		CompletedAt: &now,
		UpdatedAt:   now,
		Version:     2, // Version > 1 覆盖 task.created 的 Version=1
	}
	if p.factory != nil && p.factory.taskReader != nil {
		// context.WithoutCancel：synthesis OnTurnEnd 的 ctx 可能已随 turn 结束
		// 被取消，DB 回读不应被中断（与 CompleteTaskTerminal 的 detached ctx 一致）。
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 2*time.Second)
		existing, err := p.factory.taskReader.GetTask(readCtx, taskID)
		cancel()
		if err != nil {
			p.lg.Warn("terminalTask: 回读父 Task 失败，回退最小载荷",
				loggateway.Str("task_id", taskID),
				loggateway.Err(err),
			)
			return task
		}
		task.SessionID = existing.SessionID
		task.UserMessage = existing.UserMessage
		task.Seq = existing.Seq
		task.CreatedAt = existing.CreatedAt
	}
	return task
}

// sanitizeRawJSON guards event payloads against malformed JSON from the LLM
// or tools: Step.ToolArgs/ToolResult are json.RawMessage, and RawMessage's
// MarshalJSON validates its content — one malformed tool call would make the
// whole step.updated/step.completed event fail to marshal and be dropped from
// BOTH the outbox persist path and the WS subscriber (2026-07-25 22:33
// incident: "outbox marshal failed" / "ws v2 marshal failed"). Invalid input
// is demoted to a plain JSON string so the event always stays marshalable
// and the original text remains inspectable.
func sanitizeRawJSON(b []byte) json.RawMessage {
	if len(b) == 0 {
		return nil
	}
	if json.Valid(b) {
		return json.RawMessage(b)
	}
	q, err := json.Marshal(string(b))
	if err != nil {
		return nil
	}
	return json.RawMessage(q)
}

// completeStep marks the step completed, removes it from the active map, and
// emits step.completed. content (if non-empty) overwrites step.Content; result
// (if non-nil) overwrites step.ToolResult.
func (p *ActivityProjector) completeStep(ctx context.Context, stepID, content string, result json.RawMessage) {
	if p.seq == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if !ok {
		p.mu.Unlock()
		return
	}
	if content != "" {
		step.Content = content
	}
	if result != nil {
		step.ToolResult = sanitizeRawJSON(result)
	}
	step.Status = biz.StepStatusCompleted
	step.CompletedAt = &now
	step.Version++
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
}

// failStep marks the step failed, records the error code, removes it from the
// active map, and emits step.failed.
func (p *ActivityProjector) failStep(ctx context.Context, stepID string, err error) {
	if p.seq == nil {
		return
	}
	now := time.Now()
	p.mu.Lock()
	step, ok := p.activeStep[stepID]
	if !ok {
		p.mu.Unlock()
		return
	}
	step.Status = biz.StepStatusFailed
	step.CompletedAt = &now
	step.Version++
	if err != nil {
		step.ToolErrorCode = err.Error()
	}
	delete(p.activeStep, stepID)
	p.mu.Unlock()
	p.seq.Publish(ctx, biz.NewStepFailedEvent(*step))
}

// === ProcessEvent: trpc event → v2 callback dispatch ===

// ProcessEvent dispatches a trpc runtime event to the appropriate v2 callbacks.
// It is the v2 equivalent of v1 ActivityProjector.ProcessEvent.
//
// The projector must have OnTurnStart called (to set ProjectMeta) before
// ProcessEvent is invoked. ProcessEvent owns the BeginStep lifecycle for
// thinking/reply steps: it lazily creates them on the first delta and
// finalizes them on the Done (non-partial) chunk.
//
// Error handling: error-bearing events that are NOT tool responses are logged
// but do not trigger a v2 callback (v2 baseline has no OnError). Tool response
// errors are routed through OnToolResult's err path so the action step is
// marked failed.
func (p *ActivityProjector) ProcessEvent(ctx context.Context, ev *trpcevent.Event) {
	if ev == nil || p.seq == nil || p.meta.TaskID == "" {
		return
	}
	if ev.Response == nil {
		return
	}

	// 2026-07-04 问题 1 根因 1 修复：Graph 模式下多个 member agent 的事件
	// 通过同一 projector 处理。ev.Author 有两种格式：
	//   1. graph node 主动 emit 的事件（lifecycle）：ev.Author = node ID
	//      （如 "member-1"，由 emitter.go:130-132 设置）
	//   2. member agent LLM 流式事件（ChatCompletionChunk 等）：ev.Author =
	//      agent key（如 "spirit-worker-a"，由 content.go:569 + llm_agent.go:1552
	//      设置 invocation.AgentName = a.name = agent key）
	// 之前的修复只处理 case 1，对 case 2（LLM 事件）lookup 失败，所有 member
	// agent 的 Step 被错误归到 anchor agent 名下，前端匹配不到成员活动。
	//
	// 修复策略：两种 lookup 都尝试
	//   - 先查 NodeIDToAgentKey（case 1：node ID → agent key）
	//   - 再检查 MemberAgentKeys 集合（case 2：author 本身就是 agent key）
	// ProcessEvent 由 stream consumer 顺序调用，无并发风险。
	if author := strings.TrimSpace(ev.Author); author != "" {
		var resolvedKey string
		// Case 1: author 是 node ID，查 NodeIDToAgentKey
		if p.meta.NodeIDToAgentKey != nil {
			if k, ok := p.meta.NodeIDToAgentKey[author]; ok && k != "" {
				resolvedKey = k
			}
		}
		// Case 2: author 本身就是 agent key，检查是否在 MemberAgentKeys 集合中
		if resolvedKey == "" && len(p.meta.MemberAgentKeys) > 0 {
			if _, ok := p.meta.MemberAgentKeys[author]; ok {
				resolvedKey = author
			}
		}
		// 切换 p.meta.AgentKey 让 BeginStep 创建的 Step.AuthorAgentKey 归属到
		// 正确的 member agent。defer 恢复原值。
		if resolvedKey != "" && resolvedKey != p.meta.AgentKey {
			origKey := p.meta.AgentKey
			p.meta.AgentKey = resolvedKey
			defer func() { p.meta.AgentKey = origKey }()
		}
	}

	// Tool responses carrying errors are handled by processToolResponse
	// (failStep path), not short-circuited here. Other error events are
	// routed to OnError which marks the root Task as failed.
	if ev.Response.Error != nil && ev.Response.Object != trpcmodel.ObjectTypeToolResponse {
		// 工具迭代上限硬停不是执行失败：框架拒绝派发本批调用并终止循环，
		// 走专门投影路径（关闭未执行步骤 + notice），避免产生幻影失败证据
		// （见 processToolLimitHardStop 注释）。
		if isToolLimitTerminalEvent(ev) {
			p.processToolLimitHardStop(ctx, ev.Response.Error.Message)
			return
		}
		errType := ev.Response.Error.Type
		if errType == "" {
			errType = "run_error"
		}
		p.OnError(ctx, ev.Response.Error.Message, errType, "")
		return
	}

	switch ev.Response.Object {
	case trpcmodel.ObjectTypeChatCompletionChunk:
		p.processChatChunk(ctx, ev)
	case trpcmodel.ObjectTypeChatCompletion:
		p.processChatCompletion(ctx, ev)
	case trpcmodel.ObjectTypeToolResponse:
		p.processToolResponse(ctx, ev)
	}
}

// processChatChunk handles streaming chat completion chunks.
// Partial chunks emit deltas; non-partial (final) chunks finalize the step.
func (p *ActivityProjector) processChatChunk(ctx context.Context, ev *trpcevent.Event) {
	for _, choice := range ev.Response.Choices {
		// Tool calls (from both Message and Delta)
		allToolCalls := append(choice.Message.ToolCalls, choice.Delta.ToolCalls...)
		for _, tc := range allToolCalls {
			p.handleToolCall(ctx, tc.ID, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
		}

		// Text and reasoning content
		text, reasoning := choiceStreamContent(choice, ev.Response.IsPartial)
		if ev.Response.IsPartial {
			if reasoning != "" {
				p.handleReasoningDelta(ctx, reasoning)
			}
			if text != "" {
				p.handleTextDelta(ctx, text)
			}
		} else {
			// Final chunk: finalize the step even if content is empty.
			p.handleReasoningDone(ctx, reasoning)
			p.handleTextDone(ctx, text)
		}
	}
}

// processChatCompletion handles non-streaming chat completion events.
func (p *ActivityProjector) processChatCompletion(ctx context.Context, ev *trpcevent.Event) {
	for _, choice := range ev.Response.Choices {
		msg := choice.Message

		for _, tc := range msg.ToolCalls {
			p.handleToolCall(ctx, tc.ID, tc.Function.Name, json.RawMessage(tc.Function.Arguments))
		}

		text := strings.TrimSpace(msg.Content)
		reasoning := strings.TrimSpace(msg.ReasoningContent)
		p.handleReasoningDone(ctx, reasoning)
		p.handleTextDone(ctx, text)
	}
}

// processToolResponse handles tool result events.
func (p *ActivityProjector) processToolResponse(ctx context.Context, ev *trpcevent.Event) {
	if len(ev.Response.Choices) == 0 {
		return
	}
	msg := ev.Response.Choices[0].Message
	toolID := strings.TrimSpace(msg.ToolID)
	if toolID == "" {
		return
	}

	p.mu.Lock()
	stepID, ok := p.toolCallSteps[toolID]
	delete(p.toolCallSteps, toolID)
	p.mu.Unlock()
	if !ok {
		// Tool result without a prior OnToolCall — nothing to complete.
		return
	}

	resultJSON := json.RawMessage(msg.Content)
	if ev.Response.Error != nil {
		p.OnToolResult(ctx, stepID, resultJSON, &toolResponseError{msg: ev.Response.Error.Message, code: ev.Response.Error.Type})
		return
	}
	p.OnToolResult(ctx, stepID, resultJSON, nil)
}

// toolResponseError wraps a trpc ResponseError as a Go error so it can be
// passed to OnToolResult's err parameter. failStep records err.Error() as
// ToolErrorCode.
type toolResponseError struct {
	msg  string
	code string
}

func (e *toolResponseError) Error() string {
	if e.code != "" {
		return e.code + ": " + e.msg
	}
	return e.msg
}

// handleToolCall records the tool_call_id → stepID mapping and delegates to OnToolCall.
func (p *ActivityProjector) handleToolCall(ctx context.Context, toolCallID, toolName string, args json.RawMessage) {
	stepID := p.OnToolCall(ctx, p.meta, toolName, args)
	if toolCallID != "" && stepID != "" {
		p.mu.Lock()
		p.toolCallSteps[toolCallID] = stepID
		p.mu.Unlock()
	}
}

// handleReasoningDelta lazily creates a thinking step on the first delta,
// then emits the streaming delta.
//
// 2026-07-04 问题 1 根因 2 修复：使用 per-agentKey map 隔离不同 member
// agent 的 thinking step，避免 graph 模式下后续 member 复用首个 member
// 的 thinking step。
func (p *ActivityProjector) handleReasoningDelta(ctx context.Context, delta string) {
	agentKey := p.meta.AgentKey
	stepID, ok := p.thinkingStepIDs[agentKey]
	if !ok || stepID == "" {
		stepID = p.BeginStep(p.meta, biz.StepKindThinking)
		p.thinkingStepIDs[agentKey] = stepID
	}
	p.OnReasoningDelta(ctx, stepID, delta, "")
}

// handleTextDelta lazily creates a reply step on the first non-blank delta,
// then emits the streaming delta.
//
// Pure-whitespace leading deltas (e.g. "\n", " ") do NOT create a step —
// LLM frameworks often emit these as separators. Creating a step for them
// would leave an empty ReplyStep when no real content follows, producing
// empty ReplyBlocks in the frontend. Once a step exists, subsequent
// whitespace deltas are streamed normally (whitespace is meaningful content
// mid-stream).
//
// Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.1.1
//
// 2026-07-04 问题 1 根因 2 修复：使用 per-agentKey map 隔离不同 member
// agent 的 reply step。
func (p *ActivityProjector) handleTextDelta(ctx context.Context, delta string) {
	agentKey := p.meta.AgentKey
	stepID, ok := p.replyStepIDs[agentKey]
	if !ok || stepID == "" {
		// 防止 LLM 输出引导空白（"\n", " "）导致创建空 ReplyStep。
		// 仅在 delta 含非空白字符时才创建 step；纯空白 delta 丢弃。
		if strings.TrimSpace(delta) == "" {
			return
		}
		stepID = p.BeginStep(p.meta, biz.StepKindReply)
		p.replyStepIDs[agentKey] = stepID
	}
	p.OnTextDelta(ctx, stepID, delta, "")
}

// handleReasoningDone finalizes the thinking step. If no thinking step was
// created (no prior delta) and the content is empty, this is a no-op.
func (p *ActivityProjector) handleReasoningDone(ctx context.Context, finalContent string) {
	agentKey := p.meta.AgentKey
	stepID, ok := p.thinkingStepIDs[agentKey]
	if !ok || stepID == "" {
		// No thinking step was started. If there's reasoning content, create
		// and immediately complete a step for it; otherwise skip.
		if strings.TrimSpace(finalContent) == "" {
			return
		}
		stepID = p.BeginStep(p.meta, biz.StepKindThinking)
		p.thinkingStepIDs[agentKey] = stepID
	}
	p.OnReasoningDone(ctx, stepID, finalContent)
	delete(p.thinkingStepIDs, agentKey)
}

// handleTextDone finalizes the reply step. If no reply step was created and
// the content is empty, this is a no-op. If a reply step was created (e.g.
// by an earlier delta) but both accumulated Content and finalContent are
// blank, the step is finalized with Status=cancelled (not completed) so the
// frontend can filter out empty ReplyBlocks.
//
// Spec: docs/superpowers/specs/2026-07-04-empty-reply-step-cleanup-design.md §4.1.2
func (p *ActivityProjector) handleTextDone(ctx context.Context, finalContent string) {
	agentKey := p.meta.AgentKey
	stepID, ok := p.replyStepIDs[agentKey]
	if !ok || stepID == "" {
		if strings.TrimSpace(finalContent) == "" {
			return
		}
		stepID = p.BeginStep(p.meta, biz.StepKindReply)
		p.replyStepIDs[agentKey] = stepID
	}
	// 检查 step 是否为空（已创建但 Content 与 finalContent 均为空白）
	p.mu.Lock()
	step, stepOk := p.activeStep[stepID]
	isBlank := stepOk && strings.TrimSpace(step.Content) == "" && strings.TrimSpace(finalContent) == ""
	if isBlank {
		// 空 reply 取消而非完成，前端按 status=cancelled 过滤
		now := time.Now()
		step.Status = biz.StepStatusCancelled
		step.IsFinal = false
		step.CompletedAt = &now
		step.Version++
		delete(p.activeStep, stepID)
	}
	p.mu.Unlock()
	if isBlank {
		p.seq.Publish(ctx, biz.NewStepCompletedEvent(*step))
		delete(p.replyStepIDs, agentKey)
		return
	}
	// isFinal=true for the root turn's reply so the frontend can mark it.
	p.OnTextDone(ctx, stepID, finalContent, true)
	delete(p.replyStepIDs, agentKey)
}

// choiceStreamContent extracts text and reasoning from a streaming choice.
// Mirrors internal/agent.ChoiceStreamContent but is duplicated here to avoid
// importing internal/agent (which would re-introduce the circular dependency).
func choiceStreamContent(choice trpcmodel.Choice, partial bool) (text, reasoning string) {
	msg := choice.Message
	delta := choice.Delta
	if partial {
		text = delta.Content
		reasoning = delta.ReasoningContent
		if text == "" {
			text = strings.TrimSpace(msg.Content)
		}
		if reasoning == "" {
			reasoning = strings.TrimSpace(msg.ReasoningContent)
		}
		return text, reasoning
	}
	text = firstNonEmpty(msg.Content, delta.Content)
	reasoning = firstNonEmpty(msg.ReasoningContent, delta.ReasoningContent)
	return text, reasoning
}

// firstNonEmpty returns the first non-empty string after trimming.
func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if s := strings.TrimSpace(v); s != "" {
			return s
		}
	}
	return ""
}
