package biz

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ---------------------------------------------------------------------------
// F1 (2026-09-03, lbg-verify-planner 复盘)：团队超时 idle 语义测试。
// wall-clock 一刀切记「长任务即超时」曾误杀仍在推进的团队（成员 LLM 首字节
// 重试预算 213s 未耗尽，600s 团队墙钟先到）。现语义：idle 窗口内无成员活动
// 才判死；仍有活动顺延；探测失败 fail-open 顺延；maxLifetime 绝对兜底。
// R1 审查修复（2026-09-03）：probeErr 路径原可无限顺延（runStartedAt 零值
// 跳过 maxLifetime 检查），现为——no-run 确定性信号超 2×idle 宽限判死；
// infra 失败记 strike 连续 ≥3 次 fail-closed；maxLifetime 基准在 run 启动
// 时间不可得时退化团队创建时间。
// ---------------------------------------------------------------------------

// fakeTimeoutTeamUC is a minimal SpiritTeamAssembler fake for timeout tests.
type fakeTimeoutTeamUC struct {
	mu           sync.Mutex
	team         Team
	runs         []TeamRunRecord
	runsErr      error
	getErr       error // 非空时 Get 失败（模拟瞬时 DB 抖动）
	transitioned []string
}

func (f *fakeTimeoutTeamUC) Create(_ context.Context, in Team) (Team, error) { return in, nil }
func (f *fakeTimeoutTeamUC) Get(_ context.Context, id string) (Team, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.getErr != nil {
		return Team{}, f.getErr
	}
	if f.team.ID != id {
		return Team{}, fmt.Errorf("not found: %s", id)
	}
	return f.team, nil
}

// clearGetErr 清除 Get 故障注入（模拟 DB 恢复）。
func (f *fakeTimeoutTeamUC) clearGetErr() {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.getErr = nil
}
func (f *fakeTimeoutTeamUC) Update(_ context.Context, _ string, patch Team) (Team, error) {
	return patch, nil
}
func (f *fakeTimeoutTeamUC) TransitionStatus(_ context.Context, id string, newStatus string) (Team, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.transitioned = append(f.transitioned, newStatus)
	f.team.Status = newStatus
	return f.team, nil
}
func (f *fakeTimeoutTeamUC) ListBySpiritSessionID(_ context.Context, _ string) ([]Team, error) {
	return nil, nil
}
func (f *fakeTimeoutTeamUC) BatchArchiveTeams(_ context.Context, _ []string) (int, error) {
	return 0, nil
}
func (f *fakeTimeoutTeamUC) ListRuns(_ context.Context, _ string, _ int) ([]TeamRunRecord, error) {
	if f.runsErr != nil {
		return nil, f.runsErr
	}
	return f.runs, nil
}

// fakeTimeoutSessionUC is a minimal SpiritSessionAccessor fake.
type fakeTimeoutSessionUC struct {
	children []Session
}

func (f *fakeTimeoutSessionUC) Get(_ context.Context, id string) (Session, error) {
	return Session{ID: id}, nil
}
func (f *fakeTimeoutSessionUC) Create(_ context.Context, in Session) (Session, error) {
	return in, nil
}
func (f *fakeTimeoutSessionUC) Search(_ context.Context, _ SessionSearchQuery) (SessionListResult, error) {
	return SessionListResult{}, nil
}
func (f *fakeTimeoutSessionUC) ListMessagesRecent(_ context.Context, _ string, _ int) ([]ChatMessage, error) {
	return nil, nil
}
func (f *fakeTimeoutSessionUC) ListChildSessions(_ context.Context, _ string) ([]Session, error) {
	return f.children, nil
}

// fakeStepActivityProbe implements SpiritStepReader + SpiritStepActivityReader.
// latestFn 非空时优先于 latest：活跃团队的 step 时间在每次探测时刷新
// （静态时间戳在窗口到达时必然已出窗，无法模拟持续推进）。
type fakeStepActivityProbe struct {
	latest   time.Time
	latestFn func() time.Time
	err      error
	queried  [][]string
	mu       sync.Mutex
}

func (f *fakeStepActivityProbe) ListStepsBySessionID(_ context.Context, _ string) ([]Step, error) {
	return nil, nil
}
func (f *fakeStepActivityProbe) LatestStepActivityAt(_ context.Context, sessionIDs []string) (time.Time, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.queried = append(f.queried, sessionIDs)
	if f.latestFn != nil {
		return f.latestFn(), f.err
	}
	return f.latest, f.err
}

func newTimeoutTestOrchestration(teamUC *fakeTimeoutTeamUC, sessionUC *fakeTimeoutSessionUC, probe *fakeStepActivityProbe) (*SpiritOrchestration, *recordingTimeoutHandler) {
	delivery := &SpiritDelivery{lg: loggateway.NewNoop()}
	if probe != nil {
		delivery.stepReader = probe
	}
	orch := &SpiritOrchestration{
		teamUC:    teamUC,
		sessionUC: sessionUC,
		timeouts:  &teamTimeoutRegistry{},
		delivery:  delivery,
		lg:        loggateway.NewNoop(),
	}
	handler := &recordingTimeoutHandler{}
	orch.SetTimeoutHandler(handler)
	return orch, handler
}

func waitForCondition(deadline time.Duration, cond func() bool) bool {
	until := time.Now().Add(deadline)
	for time.Now().Before(until) {
		if cond() {
			return true
		}
		time.Sleep(20 * time.Millisecond)
	}
	return cond()
}

// 活跃团队：idle 窗口到达时成员仍有活动 → 不判死，定时器顺延。
func TestTeamTimeout_ActiveTeamExtended(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{ID: "t-active", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runs: []TeamRunRecord{{
			ID:        "run-1",
			TeamID:    "t-active",
			SessionID: "team-sess-1",
			StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		}},
	}
	sessionUC := &fakeTimeoutSessionUC{children: []Session{{ID: "child-1"}, {ID: "child-2"}}}
	// 成员持续在写 step：每次探测都返回当前时刻，模拟持续推进的活跃团队。
	probe := &fakeStepActivityProbe{latestFn: time.Now}
	orch, handler := newTimeoutTestOrchestration(teamUC, sessionUC, probe)

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-active")
	defer orch.CancelTimeoutTimer("t-active")

	// 等首轮触发 + 可能的顺延轮次，共观察 ~2.5s。
	time.Sleep(2500 * time.Millisecond)

	got, _ := teamUC.Get(context.Background(), "t-active")
	if got.Status != TeamStatusRunning {
		t.Fatalf("active team status=%q, want running (must be extended, not failed)", got.Status)
	}
	handler.mu.Lock()
	if len(handler.calls) != 0 {
		t.Fatalf("timeout handler called for active team: %v", handler.calls)
	}
	handler.mu.Unlock()
	// 探测必须覆盖团队主会话 + 成员子会话。
	probe.mu.Lock()
	defer probe.mu.Unlock()
	if len(probe.queried) == 0 {
		t.Fatal("activity probe was never queried")
	}
	gotIDs := probe.queried[0]
	want := map[string]bool{"team-sess-1": true, "child-1": true, "child-2": true}
	if len(gotIDs) != len(want) {
		t.Fatalf("probe session IDs=%v, want team session + 2 children", gotIDs)
	}
	for _, id := range gotIDs {
		if !want[id] {
			t.Fatalf("unexpected probe session ID %q in %v", id, gotIDs)
		}
	}
}

// 真 idle 团队：整窗无活动 → 判 failed 并通知 handler（原有超时语义保留）。
func TestTeamTimeout_IdleTeamFails(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{ID: "t-idle", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runs: []TeamRunRecord{{
			ID:        "run-1",
			TeamID:    "t-idle",
			SessionID: "team-sess-1",
			StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		}},
	}
	// 最后一次活动停在 10 分钟前（= run 起点），idle 窗口 1s → 必判死。
	probe := &fakeStepActivityProbe{latest: time.Now().Add(-10 * time.Minute)}
	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, probe)

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-idle")

	ok := waitForCondition(3*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-idle")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-idle")
		t.Fatalf("idle team status=%q, want failed", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 1 || handler.calls[0] != "t-idle" {
		t.Fatalf("timeout handler calls=%v, want [t-idle]", handler.calls)
	}
}

// 绝对兜底：成员仍有活动但单次执行尝试超过 maxLifetime → 强制判死。
func TestTeamTimeout_MaxLifetimeKillsActiveTeam(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{ID: "t-ceiling", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runs: []TeamRunRecord{{
			ID:        "run-1",
			TeamID:    "t-ceiling",
			SessionID: "team-sess-1",
			StartedAt: time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339),
		}},
	}
	// 成员此刻仍在活动——idle 探测会顺延，但 maxLifetime 兜底必须先触发。
	probe := &fakeStepActivityProbe{latest: time.Now()}
	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, probe)

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     3600, // 1h 上限，run 已跑 5h
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-ceiling")

	ok := waitForCondition(3*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-ceiling")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-ceiling")
		t.Fatalf("over-ceiling team status=%q, want failed", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 1 {
		t.Fatalf("timeout handler calls=%v, want 1 call", handler.calls)
	}
}

// 探测失败 fail-open：前 2 次不误杀，顺延重查（第 3 次 fail-closed 由
// TestTeamTimeout_ProbeErrorStrikesFailClosed 覆盖）。
func TestTeamTimeout_ProbeErrorExtends(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team:    Team{ID: "t-probeerr", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runsErr: fmt.Errorf("db down"),
	}
	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, &fakeStepActivityProbe{})

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-probeerr")
	defer orch.CancelTimeoutTimer("t-probeerr")

	// 触发时刻 ~1s(strike 1)、~2s(strike 2)，第 3 次 (~3s) 才判死；
	// 2.5s 观察窗内必须仍在运行。
	time.Sleep(2500 * time.Millisecond)

	got, _ := teamUC.Get(context.Background(), "t-probeerr")
	if got.Status != TeamStatusRunning {
		t.Fatalf("probe-error team status=%q, want running (fail-open extend)", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 0 {
		t.Fatalf("timeout handler called on probe error: %v", handler.calls)
	}
}

// P2-2 接线（R1 审查修复）：infra 类探测连续失败 ≥teamProbeMaxStrikes 次
// fail-closed 判死，替代无限顺延（病态团队可活到进程重启）。
func TestTeamTimeout_ProbeErrorStrikesFailClosed(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		// CreatedAt 缺失：ceiling 兜底不适用，判死完全由 strike 计数驱动。
		team:    Team{ID: "t-strikes", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runsErr: fmt.Errorf("db down"),
	}
	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, &fakeStepActivityProbe{})

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-strikes")

	// 触发 ~1s/~2s 顺延，~3s 第 3 次 strike → 判死。
	ok := waitForCondition(5*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-strikes")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-strikes")
		t.Fatalf("strikes team status=%q, want failed (fail-closed after %d strikes)", got.Status, teamProbeMaxStrikes)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 1 {
		t.Fatalf("timeout handler calls=%v, want 1 call", handler.calls)
	}
}

// P2-2 接线（R1 审查修复）：Running 但无任何 run 记录（errNoTeamRunToProbe，
// 启动链断裂的确定性信号）且创建已超 2×idle 宽限 → 首次触发即判死。
func TestTeamTimeout_NoRunRecordKillsAfterGrace(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{
			ID:              "t-norun",
			Status:          TeamStatusRunning,
			SpiritSessionID: "spirit-1",
			CreatedAt:       time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		},
		runs: nil, // 无 run 记录
	}
	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, &fakeStepActivityProbe{})

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1, // 宽限 = 2×1s = 2s << 10min
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-norun")

	ok := waitForCondition(3*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-norun")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-norun")
		t.Fatalf("no-run team status=%q, want failed (deterministic kill past grace)", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 1 {
		t.Fatalf("timeout handler calls=%v, want 1 call", handler.calls)
	}
}

// 宽限内的 no-run 团队不误杀：刚创建（启动链可能在途）→ 顺延。
func TestTeamTimeout_NoRunRecordWithinGraceExtends(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{
			ID:              "t-norun-grace",
			Status:          TeamStatusRunning,
			SpiritSessionID: "spirit-1",
			CreatedAt:       time.Now().UTC().Format(time.RFC3339), // 刚创建
		},
		runs: nil,
	}
	orch, _ := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, &fakeStepActivityProbe{})

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1, // 宽限 = 2s
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-norun-grace")
	defer orch.CancelTimeoutTimer("t-norun-grace")

	// 首次触发 ~1s 时在宽限内（创建 < 2s）→ 必须顺延而非判死。
	time.Sleep(1500 * time.Millisecond)
	got, _ := teamUC.Get(context.Background(), "t-norun-grace")
	if got.Status != TeamStatusRunning {
		t.Fatalf("grace-period no-run team status=%q, want running (must not be killed within grace)", got.Status)
	}
}

// R1 审查修复：探测失败时 runStartedAt 为零值，maxLifetime 基准退化团队
// 创建时间——创建超上限的团队即使探测持续失败也被 ceiling 判死。
func TestTeamTimeout_CreatedAtFallbackCeiling(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{
			ID:              "t-ceiling-fallback",
			Status:          TeamStatusRunning,
			SpiritSessionID: "spirit-1",
			CreatedAt:       time.Now().Add(-5 * time.Hour).UTC().Format(time.RFC3339),
		},
		runsErr: fmt.Errorf("db down"), // 探测永远失败 → runStartedAt 零值
	}
	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, &fakeStepActivityProbe{})

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     3600, // 1h 上限，创建已 5h
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-ceiling-fallback")

	ok := waitForCondition(3*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-ceiling-fallback")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-ceiling-fallback")
		t.Fatalf("probe-error over-ceiling team status=%q, want failed (CreatedAt fallback ceiling)", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 1 {
		t.Fatalf("timeout handler calls=%v, want 1 call", handler.calls)
	}
}

// R1 审查修复：超时回调里 Get 团队失败不得丢弃定时器——顺延重查，
// DB 恢复后 idle 团队仍被正常判死。
func TestTeamTimeout_GetFailureRearms(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{ID: "t-geterr", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runs: []TeamRunRecord{{
			ID:        "run-1",
			TeamID:    "t-geterr",
			SessionID: "team-sess-1",
			StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		}},
		getErr: fmt.Errorf("db down"),
	}
	// idle 团队（活动停在 10 分钟前），DB 恢复后首次触发即应判死。
	probe := &fakeStepActivityProbe{latest: time.Now().Add(-10 * time.Minute)}
	orch, _ := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, probe)

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-geterr")

	// 首轮触发 (~1s) Get 失败 → 顺延；1.5s 后恢复 DB。
	time.Sleep(1500 * time.Millisecond)
	teamUC.clearGetErr()

	ok := waitForCondition(4*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-geterr")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-geterr")
		t.Fatalf("get-error-rearmed team status=%q, want failed after DB recovery (timer must not be lost)", got.Status)
	}
}

// 降级路径：stepReader 不具备活动探测能力时退化为 run 起点活动
// （等价旧 wall-clock 语义），仍受 maxLifetime 兜底。
func TestTeamTimeout_NoActivityProbeDegradesToRunStart(t *testing.T) {
	teamUC := &fakeTimeoutTeamUC{
		team: Team{ID: "t-degraded", Status: TeamStatusRunning, SpiritSessionID: "spirit-1"},
		runs: []TeamRunRecord{{
			ID:        "run-1",
			TeamID:    "t-degraded",
			SessionID: "team-sess-1",
			StartedAt: time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		}},
	}
	// probe=nil：delivery.stepReader 未配置 → 活动=run 起点（10 分钟前）→ idle 判死。
	orch, handler := newTimeoutTestOrchestration(teamUC, &fakeTimeoutSessionUC{}, nil)

	orch.registerTeamTimeout(context.Background(), ParallelConfig{
		TeamTimeoutSeconds:         1,
		TeamMaxLifetimeSeconds:     14400,
		TimeoutHandlerDBTimeoutSec: 5,
	}, "t-degraded")

	ok := waitForCondition(3*time.Second, func() bool {
		got, err := teamUC.Get(context.Background(), "t-degraded")
		return err == nil && got.Status == TeamStatusFailed
	})
	if !ok {
		got, _ := teamUC.Get(context.Background(), "t-degraded")
		t.Fatalf("degraded team status=%q, want failed (run-start activity only)", got.Status)
	}
	handler.mu.Lock()
	defer handler.mu.Unlock()
	if len(handler.calls) != 1 {
		t.Fatalf("timeout handler calls=%v, want 1 call", handler.calls)
	}
}
