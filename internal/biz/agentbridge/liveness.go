// liveness.go M4 活跃性监管（ADR-88，设计「活跃性监管」§A–§F）：
// 三信号融合（L-activity=session/update、L-protocol=_aranea/ping 探测、
// L-transport=run() 退出即终态），按 G0→G1→G2→G3→G4 严格逐级阶梯推进；
// 废除单任务硬超时——长任务合法性由活跃性证据保障（静默→探测→失联处置）。
package agentbridge

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ------- 阈值配置（设计 §E：全局默认 + coding_agents.liveness_override 按 key 覆盖） -------

// LivenessConfig 是活跃性阶梯阈值；<= 0 的字段回落默认（DefaultLivenessConfig）。
// LivenessOverride JSON 键与字段同名小写下划线（probe_interval 等），
// 取值=秒数（数字）或 Go 时长串（"90s"/"3m"）；lost_action 为字符串。
type LivenessConfig struct {
	ProbeInterval          time.Duration // G0/G1 基线探测间隔（30s）
	ProbeIntervalEscalated time.Duration // G2 升级探测间隔（10s）
	ProbeTimeout           time.Duration // 单次探测超时（10s）
	ProbeMaxMiss           int           // 连续 miss 达此值判失联（3）
	SilenceWarn            time.Duration // G1 静默标记（90s）
	SilenceEscalate        time.Duration // G2 可疑（3min）
	LostAfter              time.Duration // G3 失联（5min）
	LostAction             string        // ask（默认）/ keep_waiting / kill
	LostDecisionTimeout    time.Duration // 失联确认卡等待上限（10min），超时处置=kill
	CancelGrace            time.Duration // cancel 后宽限期，超时强制 Kill（10s）
	SoftDurationWarn       time.Duration // 软超时一次性提示（60min，0=关闭）
}

// DefaultLivenessConfig 返回设计 §E 全局默认。
func DefaultLivenessConfig() LivenessConfig {
	return LivenessConfig{
		ProbeInterval:          30 * time.Second,
		ProbeIntervalEscalated: 10 * time.Second,
		ProbeTimeout:           10 * time.Second,
		ProbeMaxMiss:           3,
		SilenceWarn:            90 * time.Second,
		SilenceEscalate:        3 * time.Minute,
		LostAfter:              5 * time.Minute,
		LostAction:             "ask",
		LostDecisionTimeout:    10 * time.Minute,
		CancelGrace:            10 * time.Second,
		SoftDurationWarn:       60 * time.Minute,
	}
}

// withDefaults 用全局默认补齐未配置字段（LostAction 空白时回落 ask）。
func (c LivenessConfig) withDefaults() LivenessConfig {
	d := DefaultLivenessConfig()
	if c.ProbeInterval <= 0 {
		c.ProbeInterval = d.ProbeInterval
	}
	if c.ProbeIntervalEscalated <= 0 {
		c.ProbeIntervalEscalated = d.ProbeIntervalEscalated
	}
	if c.ProbeTimeout <= 0 {
		c.ProbeTimeout = d.ProbeTimeout
	}
	if c.ProbeMaxMiss <= 0 {
		c.ProbeMaxMiss = d.ProbeMaxMiss
	}
	if c.SilenceWarn <= 0 {
		c.SilenceWarn = d.SilenceWarn
	}
	if c.SilenceEscalate <= 0 {
		c.SilenceEscalate = d.SilenceEscalate
	}
	if c.LostAfter <= 0 {
		c.LostAfter = d.LostAfter
	}
	if strings.TrimSpace(c.LostAction) == "" {
		c.LostAction = d.LostAction
	}
	if c.LostDecisionTimeout <= 0 {
		c.LostDecisionTimeout = d.LostDecisionTimeout
	}
	if c.CancelGrace <= 0 {
		c.CancelGrace = d.CancelGrace
	}
	// SoftDurationWarn 允许 0（关闭），仅在负值时回落默认。
	if c.SoftDurationWarn < 0 {
		c.SoftDurationWarn = d.SoftDurationWarn
	}
	return c
}

// ResolveLivenessConfig 解析按 agent 阈值覆盖（coding_agents.liveness_override）：
// base 先用 withDefaults 归一，覆盖项按 JSON 键替换；非法取值静默忽略（FR-20 宽松）。
func ResolveLivenessConfig(base LivenessConfig, override map[string]any) LivenessConfig {
	cfg := base.withDefaults()
	if len(override) == 0 {
		return cfg
	}
	dur := func(key string, dst *time.Duration) {
		v, ok := override[key]
		if !ok {
			return
		}
		switch n := v.(type) {
		case float64:
			if n > 0 {
				*dst = time.Duration(n * float64(time.Second))
			}
		case string:
			if d, err := time.ParseDuration(strings.TrimSpace(n)); err == nil && d > 0 {
				*dst = d
			}
		}
	}
	dur("probe_interval", &cfg.ProbeInterval)
	dur("probe_interval_escalated", &cfg.ProbeIntervalEscalated)
	dur("probe_timeout", &cfg.ProbeTimeout)
	dur("silence_warn", &cfg.SilenceWarn)
	dur("silence_escalate", &cfg.SilenceEscalate)
	dur("lost_after", &cfg.LostAfter)
	dur("lost_decision_timeout", &cfg.LostDecisionTimeout)
	dur("cancel_grace", &cfg.CancelGrace)
	dur("soft_duration_warn", &cfg.SoftDurationWarn)
	if v, ok := override["probe_max_miss"]; ok {
		switch n := v.(type) {
		case float64:
			if n > 0 {
				cfg.ProbeMaxMiss = int(n)
			}
		case string:
			if i, err := strconv.Atoi(strings.TrimSpace(n)); err == nil && i > 0 {
				cfg.ProbeMaxMiss = i
			}
		}
	}
	if v, ok := override["lost_action"].(string); ok {
		switch strings.ToLower(strings.TrimSpace(v)) {
		case "ask", "keep_waiting", "kill":
			cfg.LostAction = v
		}
	}
	return cfg
}

// ------- 阶梯 / 端口 -------

// 任务行 liveness_state 列枚举（设计 §B/§C）。
const (
	LivenessHealthy = "healthy" // G0
	LivenessSilent  = "silent"  // G1
	LivenessSuspect = "suspect" // G2
	LivenessLost    = "lost"    // G3
)

// G4 「已处置」不落 liveness_state——kill 后经任务状态机 cancelled/failed 表达。

// heartbeatKind M4 §F：shim 心跳 update kind——计入活跃性证据但不计 progress_count。
const heartbeatKind = "_aranea/heartbeat"

// 失联确认卡可选项 ID（设计 §C G3）。
const (
	livenessOptKeepWaiting = "keep_waiting"
	livenessOptKill        = "kill"
)

// LivenessDecider 由 service 注入：G3 失联时向用户发起确认卡（复用审批卡通道，
// source=external_coding_liveness），阻塞等待用户结论；ctx 超时应返回错误。
// Stability:internal
type LivenessDecider interface {
	AskLivenessAction(ctx context.Context, t *CodingTask, opts []PermissionOption) (optionID string, err error)
}

// LivenessListener 可选（service 注入）：活跃性阶梯的用户可观察出口——
// 阶梯迁移（流程日志 + 进度面板徽标）与系统提示（静默/软超时/处置）。
// 实现必须快速返回（在监管 tick 路径上同步调用）。
// Stability:internal
type LivenessListener interface {
	// OnLivenessState 阶梯迁移（from→to ∈ healthy/silent/suspect/lost/killed）。
	OnLivenessState(t *CodingTask, from, to string)
	// OnLivenessHint 用户可见提示（「静默中」徽标/静默超 N 分钟/软超时/处置结果）。
	OnLivenessHint(t *CodingTask, text string)
}

// excerptLimit last_excerpt 列宽（设计 §B：256 截断）。
const excerptLimit = 256

// persistInterval last_active_at / last_excerpt 落库节流（流式事件防 DB 打爆）。
const persistInterval = 10 * time.Second

// ------- 单任务监管器 -------

// livenessSupervisor 是单个活跃任务的监管器（设计 §B/§C）。1s tick 驱动：
// 阶梯迁移、探测调度（严格串行：当前 probe 返回后才排下一次）、处置协调。
// 单测可直接调 tick(now) 取得确定性（白盒，同包）。
type livenessSupervisor struct {
	u    *AgentBridgeUsecase
	sess ACPSession
	cfg  LivenessConfig

	taskID string
	// snapshot 任务行拷贝（发卡/提示用字段），随补丁迁移逐步刷新。
	snapshot *CodingTask

	lastActivity atomic.Int64 // unixNano；noteActivity 与启动时初始化
	lastExcerpt  atomic.Value // string
	startedAt    time.Time

	stopCh   chan struct{}
	stopOnce sync.Once

	mu         sync.Mutex
	state      string
	miss       int
	nextProbe  time.Time
	deciding   bool // G3 确认卡挂起中（防重复发卡）
	suspended  bool // G4 处置已触发，阶梯观察暂停
	softWarned bool
	persistAt  time.Time // LastActiveAt/Excerpt 上次落库时刻
}

// newLivenessSupervisor 构造监管器（活动时钟从派发时开始）。
func newLivenessSupervisor(u *AgentBridgeUsecase, task *CodingTask, sess ACPSession, cfg LivenessConfig) *livenessSupervisor {
	now := time.Now()
	s := &livenessSupervisor{
		u:         u,
		sess:      sess,
		cfg:       cfg,
		taskID:    task.ID,
		snapshot:  task,
		startedAt: now,
		stopCh:    make(chan struct{}),
		state:     LivenessHealthy,
		nextProbe: now.Add(cfg.ProbeInterval),
	}
	s.lastActivity.Store(now.UnixNano())
	s.lastExcerpt.Store("")
	return s
}

// run 1s tick 主循环（safego 托管；run() 协程 defer stop() 后自然退出）。
func (s *livenessSupervisor) run() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-s.stopCh:
			return
		case now := <-ticker.C:
			s.tick(now)
		}
	}
}

func (s *livenessSupervisor) stop() {
	s.stopOnce.Do(func() { close(s.stopCh) })
}

// noteActivity M4 §A L-activity：任何 session/update（含心跳）证明数据通道
// 仍在产出 → 重置静默时钟并按「任一信号复现 → 回 G0」恢复阶梯。
// 非心跳事件刷新 excerpt；LastActiveAt/LastExcerpt 以 persistInterval 节流落库。
func (s *livenessSupervisor) noteActivity(kind, text string) {
	now := time.Now()
	s.lastActivity.Store(now.UnixNano())
	if kind != heartbeatKind {
		ex := truncateRunes(text, excerptLimit)
		s.lastExcerpt.Store(ex)
		s.mu.Lock()
		due := now.Sub(s.persistAt) >= persistInterval
		if due {
			s.persistAt = now
		}
		s.mu.Unlock()
		if due {
			at := now.UTC().Format(time.RFC3339)
			s.patchLiveness(TaskPatch{LastActiveAt: &at, LastExcerpt: &ex})
		}
	}
	s.mu.Lock()
	prev := s.state
	if prev == LivenessHealthy || s.suspended || s.deciding {
		s.mu.Unlock()
		return
	}
	s.state = LivenessHealthy
	s.miss = 0
	s.mu.Unlock()

	healthy, miss0 := LivenessHealthy, 0
	at := now.UTC().Format(time.RFC3339)
	s.patchLiveness(TaskPatch{LivenessState: &healthy, HeartbeatMiss: &miss0, LastActiveAt: &at})
	s.u.lg.Debug("agentbridge liveness recovered",
		fieldTaskID(s.taskID), fieldFromTo(prev, LivenessHealthy))
	s.emitState(prev, LivenessHealthy)
}

// tick 阶梯推进 + 探测调度（严格逐级：不跳级，逐级单次推进）。
func (s *livenessSupervisor) tick(now time.Time) {
	s.mu.Lock()
	if s.suspended || s.deciding {
		s.mu.Unlock()
		return
	}
	state := s.state
	s.mu.Unlock()

	last := time.Unix(0, s.lastActivity.Load())
	silence := now.Sub(last)

	// 软超时一次性提示（0=关闭）。
	if s.cfg.SoftDurationWarn > 0 && !s.softWarned && now.Sub(s.startedAt) >= s.cfg.SoftDurationWarn {
		s.softWarned = true
		s.emitHint(fmt.Sprintf("任务已运行超过 %d 分钟，可在面板查看活跃度与最近进展",
			int(s.cfg.SoftDurationWarn.Minutes())))
	}

	switch state {
	case LivenessHealthy:
		if silence >= s.cfg.SilenceWarn {
			s.toState(now, LivenessSilent, "静默中（最近无进展输出）")
		}
	case LivenessSilent:
		if silence >= s.cfg.SilenceEscalate {
			s.toState(now, LivenessSuspect,
				fmt.Sprintf("编程任务静默超过 %d 分钟，agent 可能仍在后台处理中",
					int(s.cfg.SilenceEscalate.Minutes())))
		}
	case LivenessSuspect:
		if silence >= s.cfg.LostAfter {
			s.toLost(now, 0)
		}
	}

	// L-protocol 探测：G2 用升级间隔，其余用基线；严格串行——当前 probe 返回
	// 后才排下一次（设计 §B「无并发 ping」）。
	if !now.Before(s.nextProbe) {
		interval := s.cfg.ProbeInterval
		if s.currentState() == LivenessSuspect {
			interval = s.cfg.ProbeIntervalEscalated
		}
		s.nextProbe = now.Add(interval)
		s.probe(now)
	}
}

func (s *livenessSupervisor) currentState() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.state
}

// probe 执行一次 `_aranea/ping` 探测：成功清 miss（pong 证明传输+协议存活），
// 失败计 miss；suspect 级 miss 达阈值立即判失联（设计 §C）。
// 只经 tick 调度调用——run 循环退出后不再新进 probe。
func (s *livenessSupervisor) probe(now time.Time) {
	ctx, cancel := context.WithTimeout(context.Background(), s.cfg.ProbeTimeout)
	err := s.sess.Ping(ctx)
	cancel()
	if err == nil {
		s.mu.Lock()
		hadMiss := s.miss > 0
		s.miss = 0
		s.mu.Unlock()
		if hadMiss {
			zero := 0
			s.patchLiveness(TaskPatch{HeartbeatMiss: &zero})
		}
		return
	}
	// 探测错误一律按「无法判定」（unverifiable）处理，绝不坍缩为「已死」：
	// 单次 miss 只累计证据——网络抖动/对端调度繁忙/时钟漂移都会让个别 Ping
	// 失败，仅当已进入 suspect 且连续 miss 达到阈值（多拍累计的正向证据）
	// 才推进 lost。与 orca hang-watchdog「A false positive must never kill
	// a live main thread」同一条红线（M19 注释契约）。
	s.mu.Lock()
	s.miss++
	miss, state := s.miss, s.state
	s.mu.Unlock()
	s.u.lg.Debug("agentbridge liveness probe miss",
		fieldTaskID(s.taskID), fieldMiss(miss))
	if state == LivenessSuspect && miss >= s.cfg.ProbeMaxMiss {
		s.toLost(now, miss)
		return
	}
	patch := TaskPatch{HeartbeatMiss: &miss}
	s.patchLiveness(patch)
}

// toState 阶梯迁移到 silent/suspect（G1/G2）：落库 + 流程日志 + 用户提示。
func (s *livenessSupervisor) toState(now time.Time, to, hint string) {
	s.mu.Lock()
	from := s.state
	if from != LivenessHealthy && from != LivenessSilent {
		s.mu.Unlock()
		return // 仅允许逐级前置态进入（silent/suspect 各有所属前置）
	}
	if (to == LivenessSilent && from != LivenessHealthy) ||
		(to == LivenessSuspect && from != LivenessSilent) {
		s.mu.Unlock()
		return
	}
	s.state = to
	s.mu.Unlock()

	s.patchLiveness(TaskPatch{LivenessState: &to})
	if to == LivenessSuspect {
		s.u.lg.Warn("agentbridge.liveness.suspect: task silent beyond escalate threshold",
			fieldTaskID(s.taskID), fieldFromTo(from, to))
	}
	s.emitState(from, to)
	if hint != "" {
		s.emitHint(hint)
	}
}

// toLost 判失联（G3）：落库 + 流程日志 + 按 lost_action 开始处置。
// missAtLost > 0 时一并落心跳 miss 数（探测判失联路径）。
func (s *livenessSupervisor) toLost(now time.Time, missAtLost int) {
	s.mu.Lock()
	from := s.state
	s.state = LivenessLost
	if missAtLost > 0 {
		s.miss = missAtLost
	}
	miss := s.miss
	s.mu.Unlock()

	lost := LivenessLost
	patch := TaskPatch{LivenessState: &lost, HeartbeatMiss: &miss}
	s.patchLiveness(patch)
	s.u.lg.Error("agentbridge.liveness.lost: task unreachable by all liveness signals",
		fieldTaskID(s.taskID), fieldFromTo(from, LivenessLost), fieldMiss(miss))
	s.emitState(from, LivenessLost)
	s.emitHint("与编程 agent 失去活动联系，等待处置")

	switch strings.ToLower(strings.TrimSpace(s.cfg.LostAction)) {
	case "kill":
		s.escalateKill()
	case "keep_waiting":
		// 留在 G3 观望：活动信号仍可救回（noteActivity → G0）。
	default: // ask
		s.decideLost(now)
	}
}

// decideLost 发起失联确认卡（设计 §C G3 ask 路径，卡片在独立协程中等待，
// 不阻塞监管 tick——活动信号复现仍可通过 noteActivity 恢复；决议到达后以
// 决议为准，keep_waiting 重置活动时钟回 G0，kill/超时 → G4 处置）。
func (s *livenessSupervisor) decideLost(now time.Time) {
	s.mu.Lock()
	if s.deciding {
		s.mu.Unlock()
		return
	}
	s.deciding = true
	s.mu.Unlock()

	safego.GoBackground("agentbridge.liveness.decide", func() {
		defer func() {
			s.mu.Lock()
			s.deciding = false
			s.mu.Unlock()
		}()
		if s.u.decider == nil {
			// 无 decider 装配（测试/降级）：保持 G3 观望，等 lost_after 外的
			// 下一次 tick 不再重复发卡（deciding 复位后由 toLost 状态守护）。
			return
		}
		ctx := context.Background()
		if s.cfg.LostDecisionTimeout > 0 {
			var cancel context.CancelFunc
			ctx, cancel = context.WithTimeout(ctx, s.cfg.LostDecisionTimeout)
			defer cancel()
		}
		opts := []PermissionOption{
			{OptionID: livenessOptKeepWaiting, Name: "继续等待", Kind: "allow_once"},
			{OptionID: livenessOptKill, Name: "终止任务", Kind: "reject_once"},
		}
		optionID, err := s.u.decider.AskLivenessAction(ctx, s.snapshotForCard(), opts)
		if err != nil {
			// 超时/出错（设计 §C：lost_decision_timeout → kill）。
			s.escalateKill()
			return
		}
		if optionID == livenessOptKill {
			s.escalateKill()
			return
		}
		s.resumeWaiting()
	})
}

// snapshotForCard 返回任务行拷贝（补最近活跃信息供卡片渲染）。
func (s *livenessSupervisor) snapshotForCard() *CodingTask {
	cp := *s.snapshot
	cp.LivenessState = LivenessLost
	if ex, ok := s.lastExcerpt.Load().(string); ok {
		cp.LastExcerpt = ex
	}
	return &cp
}

// escalateKill G4 处置=终止（设计 §D 统一链：session/cancel → 宽限 → Kill）。
func (s *livenessSupervisor) escalateKill() {
	s.mu.Lock()
	if s.suspended {
		s.mu.Unlock()
		return
	}
	s.suspended = true
	from := s.state
	s.mu.Unlock()
	s.emitState(from, "killed")
	s.u.lg.Error("agentbridge.liveness.kill: escalating to force kill",
		fieldTaskID(s.taskID), fieldFromTo(from, "killed"))
	s.u.livenessKillTask(s.taskID, s.cfg.CancelGrace)
}

// resumeWaiting 用户选择「继续等待」：重置活动时钟回 G0 继续监控（防再次
// 立即触发阶梯）。
func (s *livenessSupervisor) resumeWaiting() {
	now := time.Now()
	s.lastActivity.Store(now.UnixNano())
	s.mu.Lock()
	from := s.state
	s.state = LivenessHealthy
	s.miss = 0
	s.mu.Unlock()
	healthy, miss0 := LivenessHealthy, 0
	s.patchLiveness(TaskPatch{LivenessState: &healthy, HeartbeatMiss: &miss0})
	s.u.lg.Info("agentbridge liveness: user chose to keep waiting, supervision resumed",
		fieldTaskID(s.taskID), fieldFromTo(from, LivenessHealthy))
	s.emitState(from, LivenessHealthy)
}

// patchLiveness 以「同态 noop」CAS 落 liveness 字段（状态机：同状态转换幂等合法）。
// 任务已被并发推进/到终态 → 静默放弃；终态则停监管（run() 随后也会 stop）。
func (s *livenessSupervisor) patchLiveness(patch TaskPatch) {
	ctx := context.Background()
	task, err := s.u.tasks.Get(ctx, s.taskID)
	if err != nil {
		return
	}
	if task.Status.IsTerminal() {
		s.stop()
		return
	}
	if err := s.u.tasks.UpdateStatus(ctx, s.taskID, task.Status, task.Status, patch); err != nil {
		s.u.lg.Debug("agentbridge liveness patch skipped (concurrent transition)",
			fieldTaskID(s.taskID))
		return
	}
	// 快照跟库（提示/卡片用字段：状态/最近活跃）。
	if patch.LivenessState != nil {
		s.snapshot.LivenessState = *patch.LivenessState
	}
}

func (s *livenessSupervisor) emitState(from, to string) {
	if s.u.hints == nil {
		return
	}
	t := *s.snapshot
	t.LivenessState = s.currentState()
	s.u.hints.OnLivenessState(&t, from, to)
}

func (s *livenessSupervisor) emitHint(text string) {
	if s.u.hints == nil {
		return
	}
	t := *s.snapshot
	s.u.hints.OnLivenessHint(&t, text)
}

// truncateRunes 按 rune 截断（UTF-8 安全）。
func truncateRunes(s string, n int) string {
	if n <= 0 {
		return ""
	}
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n])
}

// ------- 统一取消升级链（设计 §D：session/cancel → cancel_grace → Kill） -------

// livenessKillTask M4 §D/§C G4：失联处置=终止——任务推进 cancelling
// （Error 落 liveness_lost_kill），发 session/cancel 并进入宽限升级链。
// 任务已终态/无法转换时 no-op。
func (u *AgentBridgeUsecase) livenessKillTask(taskID string, grace time.Duration) {
	ctx := context.Background()
	task, err := u.tasks.Get(ctx, taskID)
	if err != nil || task.Status.IsTerminal() {
		return
	}
	if err := Transition(task.Status, StatusCancelling); err != nil {
		return
	}
	reason := "liveness_lost_kill"
	if err := u.tasks.UpdateStatus(ctx, taskID, task.Status, StatusCancelling, TaskPatch{Error: &reason}); err != nil {
		u.lg.Warn("agentbridge liveness kill transition failed",
			loggateway.Str("task_id", taskID), loggateway.Err(err))
		return
	}
	sess := u.sessionOf(taskID)
	if sess == nil {
		// 无活跃会话：直接收尾 cancelled（同 Cancel 的无会话分支）。
		now := nowRFC3339()
		if err := u.tasks.UpdateStatus(ctx, taskID, StatusCancelling, StatusCancelled, TaskPatch{CompletedAt: &now}); err != nil {
			u.lg.Warn("agentbridge liveness kill finalize failed",
				loggateway.Str("task_id", taskID), loggateway.Err(err))
		}
		u.notifyTerminal(taskID)
		return
	}
	if err := sess.Cancel(ctx); err != nil {
		u.lg.Warn("agentbridge liveness acp cancel failed",
			loggateway.Str("task_id", taskID), loggateway.Err(err))
	}
	u.escalateCancel(taskID, sess, grace)
}

// escalateCancel M4 §D 统一升级链：cancel 发出后宽限期内 run() 未退出
// （running 表中会话仍在册）则强制 Kill 进程组。宽限取任务监管器配置
// （liveness_override 可覆盖 cancel_grace），无监管器时用全局默认。
func (u *AgentBridgeUsecase) escalateCancel(taskID string, sess ACPSession, grace time.Duration) {
	if grace <= 0 {
		grace = DefaultLivenessConfig().CancelGrace
	}
	safego.GoBackground("agentbridge.cancel.escalate", func() {
		timer := time.NewTimer(grace)
		defer timer.Stop()
		<-timer.C
		u.mu.Lock()
		_, alive := u.running[taskID]
		u.mu.Unlock()
		if !alive {
			return // run() 已在宽限内退出（cancel 生效）
		}
		// Kill 兜底是 ctx-Done（session 实现内），不允许永久阻塞。
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		u.lg.Warn("agentbridge.liveness.kill: cancel grace expired, force killing process group",
			loggateway.Str("task_id", taskID))
		_ = sess.Kill(ctx)
	})
}

// 日志字段小帮手（本文件专用）。
func fieldTaskID(id string) loggateway.Field { return loggateway.Str("task_id", id) }
func fieldMiss(n int) loggateway.Field       { return loggateway.Int("heartbeat_miss", n) }
func fieldErr(err error) loggateway.Field    { return loggateway.Err(err) }
func fieldFromTo(from, to string) loggateway.Field {
	return loggateway.Str("liveness_transition", from+"→"+to)
}
