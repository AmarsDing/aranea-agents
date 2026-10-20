package agentbridge

import (
	"context"
	"errors"
	"testing"
	"time"
)

// ---------- 阶梯单测（确定性：直接驱动 tick(now)） ----------

// testLivenessEnv 装配一个已派发任务的监管器（不跑 goroutine，手驱 tick）。
type testLivenessEnv struct {
	*testEnv
	sess   *mockACPSession
	sup    *livenessSupervisor
	task   *CodingTask
	base   time.Time
	hints  *mockLivenessListener
	decide *mockLivenessDecider
}

type mockLivenessListener struct {
	states []string // "from→to"
	hints  []string
}

func (m *mockLivenessListener) OnLivenessState(_ *CodingTask, from, to string) {
	m.states = append(m.states, from+"→"+to)
}
func (m *mockLivenessListener) OnLivenessHint(_ *CodingTask, text string) {
	m.hints = append(m.hints, text)
}

type mockLivenessDecider struct {
	// answer 通道：发卡 goroutine 在此阻塞，测试可送 keep_waiting/kill 或注入错误。
	answer chan decisionStub
	asked  int32 // 发卡次数
}

type decisionStub struct {
	optionID string
	err      error
}

func (d *mockLivenessDecider) AskLivenessAction(ctx context.Context, _ *CodingTask, _ []PermissionOption) (string, error) {
	d.asked++
	select {
	case a := <-d.answer:
		return a.optionID, a.err
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

// tinyLivenessCfg 让阶梯在测试刻度上可驱动（tick 手驱，无需真等）。
func tinyLivenessCfg() LivenessConfig {
	cfg := LivenessConfig{
		ProbeInterval:          30 * time.Second,
		ProbeIntervalEscalated: 10 * time.Second,
		ProbeTimeout:           5 * time.Second,
		ProbeMaxMiss:           3,
		SilenceWarn:            90 * time.Second,
		SilenceEscalate:        3 * time.Minute,
		LostAfter:              5 * time.Minute,
		LostAction:             "ask",
		LostDecisionTimeout:    50 * time.Millisecond,
		CancelGrace:            30 * time.Millisecond,
		SoftDurationWarn:       60 * time.Minute,
	}
	return cfg.withDefaults()
}

func newTestLivenessEnv(t *testing.T) *testLivenessEnv {
	t.Helper()
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)
	sess := &mockACPSession{
		promptFn: func(ctx context.Context, _ string, _ string, _ EventHandler) (string, error) {
			<-ctx.Done() // 常规运行（测试只关心监管器）
			return "", ctx.Err()
		},
	}
	e.factory.sessions = []*mockACPSession{sess}
	le := &testLivenessEnv{testEnv: e, sess: sess,
		hints:  &mockLivenessListener{},
		decide: &mockLivenessDecider{answer: make(chan decisionStub, 2)},
	}
	le.uc.decider = le.decide
	le.uc.hints = le.hints
	le.uc.livenessCfg = tinyLivenessCfg()

	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "do", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	task, err := e.tasks.Get(context.Background(), res.Task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	le.task = task
	// 手驱 tick：直接调用内部 tick（白盒）。监管 goroutine 已在派发起—stop 掉避免干扰。
	sup := e.uc.sups[task.ID]
	sup.stop()
	le.sup = sup
	le.base = time.Now()
	sup.lastActivity.Store(le.base.UnixNano())
	return le
}

func repoState(t *testing.T, e *testLivenessEnv) (string, int) {
	t.Helper()
	task, err := e.tasks.Get(context.Background(), e.task.ID)
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	return task.LivenessState, task.HeartbeatMiss
}

// G0→G1：静默达 silence_warn 标记 silent（不跳级到 suspect）。
func TestLiveness_SilentThenSuspectStrictLadder(t *testing.T) {
	e := newTestLivenessEnv(t)
	sup := e.sup

	// 单次推进：刚过 warn（90s），但已远超 escalate（3min）——严格逐级只允许到 silent。
	sup.tick(e.base.Add(200 * time.Second))
	state, _ := repoState(t, e)
	if state != LivenessSilent {
		t.Fatalf("expected silent (strict ladder), got %q", state)
	}

	// 再 tick（仍在 silent 级）：静默已超 escalate → suspect。
	sup.tick(e.base.Add(210 * time.Second))
	state, _ = repoState(t, e)
	if state != LivenessSuspect {
		t.Fatalf("expected suspect, got %q", state)
	}
	if len(e.hints.hints) == 0 {
		t.Fatal("expected user hint on suspect")
	}
}

// G2→G3：连续 ping 失败达 probe_max_miss 判失联（探测路径；设计 §C：
// miss 达阈即失联，不必等 lost_after 静默耗尽）。
func TestLiveness_LostByProbeMisses(t *testing.T) {
	e := newTestLivenessEnv(t)
	sup := e.sup
	e.sess.pingErr = errors.New("dial")

	// 严格逐级：healthy tick 只推进一级。
	sup.tick(e.base.Add(100 * time.Second)) // → silent（探测 miss=1）
	sup.tick(e.base.Add(200 * time.Second)) // → suspect（探测 miss=2）
	if st, _ := repoState(t, e); st != LivenessSuspect {
		t.Fatalf("expected suspect, got %q", st)
	}
	// suspect 级升级间隔 10s：第三次 miss 达 probe_max_miss(3) → lost。
	sup.tick(e.base.Add(210 * time.Second))
	state, miss := repoState(t, e)
	if state != LivenessLost {
		t.Fatalf("expected lost, got %q", state)
	}
	if miss != 3 {
		t.Fatalf("expected heartbeat_miss=3, got %d", miss)
	}
}

// 任一信号复现 → 接口恢复 G0（noteActivity 恢复路径 + 心跳不计数）。
func TestLiveness_RecoversOnActivity(t *testing.T) {
	e := newTestLivenessEnv(t)
	sup := e.sup

	sup.tick(e.base.Add(200 * time.Second))
	if st, _ := repoState(t, e); st != LivenessSilent {
		t.Fatalf("expected silent, got %q", st)
	}
	sup.noteActivity("agent_message_chunk", "progress…")
	if st, _ := repoState(t, e); st != LivenessHealthy {
		t.Fatalf("expected recovered healthy, got %q", st)
	}
	found := false
	for _, tr := range e.hints.states {
		if tr == LivenessSilent+"→"+LivenessHealthy {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected silent→healthy state event, got %v", e.hints.states)
	}
}

// 心跳 kind 只刷新活跃性，不计 progress_count（countingHandler §F）。
func TestLiveness_HeartbeatNotCounted(t *testing.T) {
	inner := &recordingHandler{}
	c := &countingHandler{inner: inner}
	for i := 0; i < 5; i++ {
		c.OnUpdate(heartbeatKind, "")
	}
	c.OnUpdate("plan", "{}")
	if c.count != 1 {
		t.Fatalf("expected only non-heartbeat counted, got %d", c.count)
	}
}

type recordingHandler struct{}

func (recordingHandler) OnUpdate(_, _ string) {}
func (recordingHandler) OnPermission(_ context.Context, _ string, _ []PermissionOption) (string, error) {
	return "", errors.New("not implemented")
}

// G4 处置：ask 卡片用户选「终止任务」→ cancelling + 宽限强杀。
func TestLiveness_KillViaDecisionCard(t *testing.T) {
	e := newTestLivenessEnv(t)
	sup := e.sup

	// 推进到 suspect 后让 ping 持续失败（判失联），卡片等待中送 kill。
	e.sess.pingErr = errors.New("dial")
	e.decide.answer <- decisionStub{optionID: livenessOptKill}
	sup.tick(e.base.Add(400 * time.Second)) // → suspect
	sup.tick(e.base.Add(410 * time.Second)) // miss 也算 lost 触发（静默>lost_after）
	sup.tick(e.base.Add(420 * time.Second))
	sup.tick(e.base.Add(430 * time.Second))

	// 处置是异步发卡协程：等到 killed 或超时。
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e.sess.wasKilled() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected force kill after decision card; states=%v asked=%d", e.hints.states, e.decide.asked)
}

// G4 处置:卡片等待超 lost_decision_timeout → 默认终止（设计 §C）。
func TestLiveness_KillOnDecisionTimeout(t *testing.T) {
	e := newTestLivenessEnv(t)
	sup := e.sup
	sup.cfg.LostDecisionTimeout = 30 * time.Millisecond

	e.sess.pingErr = errors.New("dial")
	// 严格逐级：每 tick 至多推进一级，逐级驱动到 lost。
	sup.tick(e.base.Add(100 * time.Second)) // → silent
	sup.tick(e.base.Add(400 * time.Second)) // → suspect
	sup.tick(e.base.Add(410 * time.Second)) // → lost + 发卡（超时未决→kill）

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if e.sess.wasKilled() {
			task, _ := e.tasks.Get(context.Background(), e.task.ID)
			if task.Status == StatusCancelled || task.Status == StatusCancelling {
				return
			}
		}
		time.Sleep(10 * time.Millisecond)
	}
	task, _ := e.tasks.Get(context.Background(), e.task.ID)
	t.Fatalf("expected kill after decision timeout (status=%s killed=%v)", task.Status, e.sess.wasKilled())
}

// ask 卡用户选「继续等待」→ 回 G0 且活动时钟重置（不立即再触发）。
func TestLiveness_KeepWaitingResumes(t *testing.T) {
	e := newTestLivenessEnv(t)
	sup := e.sup

	e.sess.pingErr = errors.New("dial")
	e.decide.answer <- decisionStub{optionID: livenessOptKeepWaiting}
	// 严格逐级：100→silent，400→suspect，410→lost + 发卡。
	sup.tick(e.base.Add(100 * time.Second))
	sup.tick(e.base.Add(400 * time.Second))
	sup.tick(e.base.Add(410 * time.Second))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if st, _ := repoState(t, e); st == LivenessHealthy {
			// 用户刚选继续等待：活动时钟被重置——立即 tick 不应立刻 silent。
			sup.tick(time.Now().Add(time.Second))
			if st2, _ := repoState(t, e); st2 == LivenessSilent {
				t.Fatal("keep_waiting should reset silence clock")
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatalf("expected resume to healthy, states=%v", e.hints.states)
}

// 阈值覆盖解析：JSON 秒数/时长串/动作 + 非法值忽略（FR-20）。
func TestResolveLivenessConfig_Override(t *testing.T) {
	base := DefaultLivenessConfig()
	cfg := ResolveLivenessConfig(base, map[string]any{
		"probe_interval":   float64(5),
		"silence_warn":     "60s",
		"lost_action":      "kill",
		"probe_max_miss":   float64(7),
		"silence_escalate": "garbage", // 忽略
		"unknown_key":      123,       // 忽略
	})
	if cfg.ProbeInterval != 5*time.Second {
		t.Fatalf("probe_interval: %v", cfg.ProbeInterval)
	}
	if cfg.SilenceWarn != 60*time.Second {
		t.Fatalf("silence_warn: %v", cfg.SilenceWarn)
	}
	if cfg.LostAction != "kill" {
		t.Fatalf("lost_action: %v", cfg.LostAction)
	}
	if cfg.ProbeMaxMiss != 7 {
		t.Fatalf("probe_max_miss: %v", cfg.ProbeMaxMiss)
	}
	if cfg.SilenceEscalate != base.SilenceEscalate {
		t.Fatalf("garbage override should be ignored: %v", cfg.SilenceEscalate)
	}
}

// Cancel 宽限升级链：prompt 不响应 cancel 时宽限后 Kill 强杀（设计 §D）。
func TestUsecase_CancelEscalatesToKill(t *testing.T) {
	e := newTestEnv()
	e.seedAgent("codebuddy", true)
	e.seedProject("proj", `F:\proj`)
	var sess *mockACPSession
	sess = &mockACPSession{
		promptFn: func(ctx context.Context, _ string, _ string, _ EventHandler) (string, error) {
			// 模拟忽略 cancel 的 agent：直到升级链 Kill 才退出。
			for !sess.wasKilled() {
				time.Sleep(5 * time.Millisecond)
			}
			return "", errors.New("killed")
		},
	}
	e.factory.sessions = []*mockACPSession{sess}
	// 极小宽限驱动升级链。
	e.uc.livenessCfg.CancelGrace = 30 * time.Millisecond

	res, err := e.uc.Dispatch(context.Background(), DispatchInput{
		SessionID: "s1", AgentKey: "codebuddy", ProjectQuery: "proj", Prompt: "x", Handler: noopHandler(),
	})
	if err != nil {
		t.Fatalf("dispatch: %v", err)
	}
	if err := e.uc.Cancel(context.Background(), res.Task.ID); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if sess.wasKilled() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("expected kill after cancel grace expired")
}
