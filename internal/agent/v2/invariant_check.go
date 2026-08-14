package v2

import (
	"os"
	"sync"
	"sync/atomic"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// P1-2 事件溯源不变量开发态断言（DSH §2.1 "model-visible means logged"）。
//
// 铁律：凡进入模型请求/模型可见的内容，必须可从 append-only 事件流重建。
// 落到 v2 的 Task→Turn→Step 链路上，即每个事件必须携带可重建的谱系：
//   - step 事件必须挂在已 step.created 的实体上（unknown_step）
//   - step.created 必须挂在已 turn.started 的 turn 上（unknown_turn）
//   - turn.started 必须挂在已 task.created 的 task 上（unknown_task）
//   - 同一实体不得重复创建（duplicate_create）
//   - 同一实体不得重复到达终态（duplicate_terminal）
//   - 终态后不得再产生更新（event_after_terminal）
//
// 运行模式（方案风险项 3）：仅日志、不阻断。违规计入 violationCount，
// 同一（违规类型, 实体）组合只发一条 Error 进程日志防刷屏。
// 校验入口由 ARANEA_ORCH_INVARIANT=1 开启；默认关闭，生产零开销。
//
// 有意豁免（防误报）：
//   - step.streaming 在终态后到达属 16ms 批合并窗口的良性竞态，只查存在性；
//   - TurnID/TaskID 为空时跳过包含性检查（如游离 error/notice step）；
//   - 非 Task/Turn/Step 链事件（系统/团队/计划事件）不参与断言。
//
// FlowLog warn 支路暂缓：v2 层无 FlowLogWriter 端口（红线 3：biz 层以外
// 不得直接发流程日志），先以进程日志观察误报率，再评估是否经端口接入。

const (
	envOrchInvariant = "ARANEA_ORCH_INVARIANT"

	// maxLineageSessions 是会话谱系跟踪上限（FIFO 逐出最老会话）。
	maxLineageSessions = 1024
	// maxLineageStepsPerSession 是单会话 step 跟踪上限（FIFO 批量逐出最老 1/4）。
	maxLineageStepsPerSession = 8192
	// maxLineageReported 是单会话日志去重键上限，超限后只计数不再记键。
	maxLineageReported = 4096
)

// invariantCheckEnabled 判定是否启用开发态断言。包级变量便于测试替换。
var invariantCheckEnabled = func() bool {
	return os.Getenv(envOrchInvariant) == "1"
}

// lineageEntity 记录一个已登记实体的终态标记。
type lineageEntity struct {
	terminal bool
}

// lineageSession 是单会话的 Task→Turn→Step 谱系登记表。
type lineageSession struct {
	tasks map[string]*lineageEntity
	turns map[string]*lineageEntity
	steps map[string]*lineageEntity
	// stepOrder 记录 step 登记顺序，超限时批量逐出最老的 1/4。
	stepOrder []string
	// reported 是日志去重键集合（violation|entityID），防同一根因刷屏。
	reported map[string]struct{}
}

func newLineageSession() *lineageSession {
	return &lineageSession{
		tasks:    make(map[string]*lineageEntity),
		turns:    make(map[string]*lineageEntity),
		steps:    make(map[string]*lineageEntity),
		reported: make(map[string]struct{}),
	}
}

// invariantChecker 观察流经 Sequencer 的全部事件并断言谱系可重建。
// 开发态工具，仅在 ARANEA_ORCH_INVARIANT=1 时挂载到 Sequencer。
type invariantChecker struct {
	lg loggateway.Logger

	mu           sync.Mutex
	sessions     map[string]*lineageSession
	sessionOrder []string // FIFO 逐出顺序

	violationCount atomic.Int64
	loggedCount    atomic.Int64
}

func newInvariantChecker(lg loggateway.Logger) *invariantChecker {
	return &invariantChecker{
		lg:       lg.With(loggateway.Str("assertion", "orch_invariant")),
		sessions: make(map[string]*lineageSession),
	}
}

// violations 返回累计检测到的违规次数（含未发日志的重复违规）。
func (c *invariantChecker) violations() int64 { return c.violationCount.Load() }

// logged 返回实际发出 Error 日志的条数（去重后）。
func (c *invariantChecker) logged() int64 { return c.loggedCount.Load() }

// trackedSessions 返回当前跟踪中的会话数（测试用）。
func (c *invariantChecker) trackedSessions() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sessions)
}

// check 观察一个事件。仅处理 Task/Turn/Step 链事件，其余忽略。
func (c *invariantChecker) check(e biz.Event) {
	if c == nil || e == nil {
		return
	}
	sid := e.SpiritSessionID()
	if sid == "" {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	sess := c.sessionLocked(sid)
	kind := string(e.EventKind())

	switch ev := e.(type) {
	// --- Task ---
	case *biz.TaskCreatedEvent:
		c.createLocked(sess, sess.tasks, "task", ev.Task.ID, kind)
	case *biz.TaskUpdatedEvent:
		c.updateLocked(sess, sess.tasks, "task", ev.Task.ID, kind)
	case *biz.TaskCompletedEvent:
		c.terminalLocked(sess, sess.tasks, "task", ev.Task.ID, kind)
	case *biz.TaskFailedEvent:
		c.terminalLocked(sess, sess.tasks, "task", ev.Task.ID, kind)

	// --- Turn ---
	case *biz.TurnStartedEvent:
		if ev.Turn.TaskID != "" {
			c.requireContainmentLocked(sess, sess.tasks, "task", ev.Turn.TaskID, "unknown_task", kind)
		}
		c.createLocked(sess, sess.turns, "turn", ev.Turn.ID, kind)
	case *biz.TurnCompletedEvent:
		c.terminalLocked(sess, sess.turns, "turn", ev.Turn.ID, kind)
	case *biz.TurnFailedEvent:
		c.terminalLocked(sess, sess.turns, "turn", ev.Turn.ID, kind)

	// --- Step ---
	case *biz.StepCreatedEvent:
		if ev.Step.TurnID != "" {
			c.requireContainmentLocked(sess, sess.turns, "turn", ev.Step.TurnID, "unknown_turn", kind)
		}
		c.createStepLocked(sess, ev.Step.ID, kind)
	case *biz.StepUpdatedEvent:
		c.updateLocked(sess, sess.steps, "step", ev.Step.ID, kind)
	case *biz.StepCompletedEvent:
		c.terminalLocked(sess, sess.steps, "step", ev.Step.ID, kind)
	case *biz.StepFailedEvent:
		c.terminalLocked(sess, sess.steps, "step", ev.Step.ID, kind)
	case *biz.StepStreamingEvent:
		// 只查存在性；终态后迟到 delta 属批合并良性竞态，豁免（防误报）。
		if ev.StepID != "" {
			if _, ok := sess.steps[ev.StepID]; !ok {
				c.reportLocked(sess, "unknown_step", kind, ev.StepID)
			}
		}
	}
}

// sessionLocked 取或建会话谱系，并按 FIFO 逐出超上限的最老会话。
func (c *invariantChecker) sessionLocked(sid string) *lineageSession {
	if sess, ok := c.sessions[sid]; ok {
		return sess
	}
	sess := newLineageSession()
	c.sessions[sid] = sess
	c.sessionOrder = append(c.sessionOrder, sid)
	if len(c.sessionOrder) > maxLineageSessions {
		oldest := c.sessionOrder[0]
		c.sessionOrder = c.sessionOrder[1:]
		delete(c.sessions, oldest)
	}
	return sess
}

// createLocked 登记实体创建；重复创建报 duplicate_create。
func (c *invariantChecker) createLocked(sess *lineageSession, m map[string]*lineageEntity, entity, id, kind string) {
	if id == "" {
		return
	}
	if _, ok := m[id]; ok {
		c.reportLocked(sess, "duplicate_create", kind, id)
		return
	}
	m[id] = &lineageEntity{}
}

// createStepLocked 登记 step 创建，带单会话容量批量逐出。
func (c *invariantChecker) createStepLocked(sess *lineageSession, id, kind string) {
	if id == "" {
		return
	}
	if _, ok := sess.steps[id]; ok {
		c.reportLocked(sess, "duplicate_create", kind, id)
		return
	}
	sess.steps[id] = &lineageEntity{}
	sess.stepOrder = append(sess.stepOrder, id)
	if len(sess.stepOrder) > maxLineageStepsPerSession {
		evict := maxLineageStepsPerSession / 4
		for _, old := range sess.stepOrder[:evict] {
			delete(sess.steps, old)
		}
		sess.stepOrder = sess.stepOrder[evict:]
	}
}

// updateLocked 断言非终态更新：实体须已登记且未达终态。
func (c *invariantChecker) updateLocked(sess *lineageSession, m map[string]*lineageEntity, entity, id, kind string) {
	if id == "" {
		return
	}
	ent, ok := m[id]
	if !ok {
		c.reportLocked(sess, "unknown_"+entity, kind, id)
		return
	}
	if ent.terminal {
		c.reportLocked(sess, "event_after_terminal", kind, id)
	}
}

// terminalLocked 断言终态转换：实体须已登记且未达终态，然后标记终态。
func (c *invariantChecker) terminalLocked(sess *lineageSession, m map[string]*lineageEntity, entity, id, kind string) {
	if id == "" {
		return
	}
	ent, ok := m[id]
	if !ok {
		c.reportLocked(sess, "unknown_"+entity, kind, id)
		return
	}
	if ent.terminal {
		c.reportLocked(sess, "duplicate_terminal", kind, id)
		return
	}
	ent.terminal = true
}

// requireContainmentLocked 断言父实体已登记（且未达终态）。
func (c *invariantChecker) requireContainmentLocked(sess *lineageSession, m map[string]*lineageEntity, parentEntity, parentID, violation, kind string) {
	ent, ok := m[parentID]
	if !ok {
		c.reportLocked(sess, violation, kind, parentID)
		return
	}
	if ent.terminal {
		c.reportLocked(sess, "event_after_terminal", kind, parentID)
	}
}

// reportLocked 记录违规：计数恒增；同一（违规, 实体）组合只发一条 Error 日志。
// 去重表满后只计数不再发日志（病态会话下彻底防刷屏）。
func (c *invariantChecker) reportLocked(sess *lineageSession, violation, kind, entityID string) {
	c.violationCount.Add(1)
	key := violation + "|" + entityID
	if _, dup := sess.reported[key]; dup {
		return
	}
	if len(sess.reported) >= maxLineageReported {
		return
	}
	sess.reported[key] = struct{}{}
	c.loggedCount.Add(1)
	c.lg.Error("orchestration invariant violation: event stream cannot reconstruct entity lineage",
		loggateway.Str("violation", violation),
		loggateway.Str("kind", kind),
		loggateway.Str("entity_id", entityID))
}
