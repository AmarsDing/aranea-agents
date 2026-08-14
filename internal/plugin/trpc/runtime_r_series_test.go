package plugintrpc

import (
	"context"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/event/contract"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

// R-1：兜底路径必须返回 nil（tracker 全部方法对 nil 接收器为 no-op），
// 不再构造带 persist/retry goroutine 的临时 tracker（旧实现每次调用泄漏两个 worker）。
func TestBudgetFallbacks_ReturnNil(t *testing.T) {
	var m *Manager
	if got := m.BudgetTrackerForContext(context.Background()); got != nil {
		t.Fatalf("nil Manager BudgetTrackerForContext=%p, want nil", got)
	}
	var rt *Runtime
	if got := rt.BudgetTrackerForContext(context.Background()); got != nil {
		t.Fatalf("nil Runtime BudgetTrackerForContext=%p, want nil", got)
	}
	var reg *CostGuardBudgetRegistry
	if got := reg.TrackerForScope("s"); got != nil {
		t.Fatalf("nil registry TrackerForScope=%p, want nil", got)
	}
	var c *CostGuardPlugin
	if got := c.budget(context.Background()); got != nil {
		t.Fatalf("nil plugin budget=%p, want nil", got)
	}
	// nil tracker 方法必须全部 no-op 不 panic。
	var tr *CostGuardBudgetTracker
	if tr.WouldExceed(10, 5) {
		t.Fatal("nil tracker WouldExceed should be false")
	}
	if !tr.TryConsume(10, 5) {
		t.Fatal("nil tracker TryConsume should allow")
	}
	tr.AddOverBudget(5)
	tr.Close()
}

// R-2：分桶数超软上限时，idle 超 TTL 的分桶必须被淘汰（Close 冲刷后删除）。
func TestCostGuardBudgetRegistry_EvictIdle(t *testing.T) {
	reg := NewCostGuardBudgetRegistry(loggateway.NewNoop())
	// 填到软上限：一个真实 tracker（idle 已过期）+ 其余 nil 占位。
	stale := NewCostGuardBudgetTracker(loggateway.NewNoop())
	stale.lastUsedUnix.Store(time.Now().Add(-costGuardScopeIdleTTL - time.Hour).Unix())
	reg.byScope["stale"] = stale
	for i := 0; i < costGuardMaxScopes; i++ {
		reg.byScope[string(rune('a'+i%26))+string(rune('A'+i/26))] = nil
	}
	fresh := NewCostGuardBudgetTracker(loggateway.NewNoop())
	fresh.lastUsedUnix.Store(time.Now().Unix())
	reg.byScope["fresh"] = fresh

	reg.mu.Lock()
	reg.evictIdleLocked(time.Now())
	reg.mu.Unlock()

	if _, ok := reg.byScope["stale"]; ok {
		t.Fatal("stale bucket should have been evicted")
	}
	if reg.byScope["fresh"] != fresh {
		t.Fatal("fresh bucket must survive eviction")
	}
	if len(reg.byScope) != 1 {
		t.Fatalf("byScope len=%d, want 1 (only fresh)", len(reg.byScope))
	}
	// 被淘汰的 stale tracker 应已 Close（再次 Close 不 panic 由调用方保证一次性，
	// 这里仅验证 fresh 仍可用）。
	fresh.Close()
}

// R-3：已有更新纪元落盘时，陈旧 Apply 快照必须被丢弃。
func TestRuntime_Apply_staleSnapshotDropped(t *testing.T) {
	rt := NewRuntime(nil, loggateway.NewNoop())
	sysCtx := workspace.WithContext(context.Background(), workspace.SystemWorkspaceID)
	v1 := biz.Plugin{Key: "audit_log", Enabled: true, Scope: "global", ConfigJSON: `{}`, DefaultConfigJSON: `{}`}
	rt.Apply(sysCtx, []biz.Plugin{v1})
	if got := len(rt.PluginsForAgent("a", "")); got != 1 {
		t.Fatalf("after first apply plugins=%d want 1", got)
	}
	// 模拟「纪元已前进但旧快照迟到」：appliedSeq 被推到未来，下一次 Apply 应被丢弃。
	rt.appliedSeq.Store(rt.applySeq.Load() + 10)
	mask := biz.Plugin{Key: "sensitive_data_mask", Enabled: true, Scope: "global", ConfigJSON: `{}`, DefaultConfigJSON: `{}`}
	rt.Apply(sysCtx, []biz.Plugin{v1, mask})
	if got := len(rt.PluginsForAgent("a", "")); got != 1 {
		t.Fatalf("stale apply must be dropped, plugins=%d want 1", got)
	}
	// 之后的正常 Apply（新纪元）仍生效。
	rt.appliedSeq.Store(rt.applySeq.Load())
	rt.Apply(sysCtx, []biz.Plugin{v1, mask})
	if got := len(rt.PluginsForAgent("a", "")); got != 2 {
		t.Fatalf("fresh apply after stale drop, plugins=%d want 2", got)
	}
}

// R-4：跨日后禁止回滚旧日 reservation（会误减新日额度）。
func TestCostGuardBudgetTracker_RollbackReservation_DayGuard(t *testing.T) {
	tr := NewCostGuardBudgetTracker(loggateway.NewNoop())
	defer tr.Close()
	tr.mu.Lock()
	tr.day = "2026-08-14"
	tr.tokens = 5
	tr.mu.Unlock()

	tr.rollbackReservation("2026-08-13", 2) // 跨日：不得回滚
	tr.mu.Lock()
	if tr.tokens != 5 {
		t.Fatalf("cross-day rollback corrupted tokens=%d, want 5", tr.tokens)
	}
	tr.mu.Unlock()

	tr.rollbackReservation("2026-08-14", 2) // 同日：正常回滚
	tr.mu.Lock()
	if tr.tokens != 3 {
		t.Fatalf("same-day rollback tokens=%d, want 3", tr.tokens)
	}
	tr.mu.Unlock()
}

// R-7：租户自有插件覆盖 shared 同 key 插件，不得重复注册。
func TestRuntime_PluginsForAgent_sameKeyTenantOverride(t *testing.T) {
	rt := NewRuntime(nil, loggateway.NewNoop())
	shared := biz.Plugin{Key: "audit_log", Enabled: true, Scope: "global", WorkspaceID: "", ConfigJSON: `{}`, DefaultConfigJSON: `{}`}
	tenant := biz.Plugin{Key: "audit_log", Enabled: true, Scope: "global", WorkspaceID: "ws-a", ConfigJSON: `{}`, DefaultConfigJSON: `{}`}
	other := biz.Plugin{Key: "sensitive_data_mask", Enabled: true, Scope: "global", WorkspaceID: "", ConfigJSON: `{}`, DefaultConfigJSON: `{}`}
	sysCtx := workspace.WithContext(context.Background(), workspace.SystemWorkspaceID)
	rt.Apply(sysCtx, []biz.Plugin{shared, tenant, other})

	got := rt.PluginsForAgent("agent-1", "ws-a")
	if len(got) != 2 {
		t.Fatalf("ws-a plugins=%d want 2 (tenant audit_log + shared mask, no duplicate)", len(got))
	}
	// 其他工作区不受影响，仍看 shared audit_log。
	got = rt.PluginsForAgent("agent-1", "ws-b")
	if len(got) != 2 {
		t.Fatalf("ws-b plugins=%d want 2 (shared audit_log + shared mask)", len(got))
	}
}

// R-6：MonitorEvent 经共享有界队列异步发布（不再每条日志 spawn 一个 goroutine）。
func TestPluginSafeLogger_queuedPublish(t *testing.T) {
	bus := &stubMonitorBus{}
	l := NewPluginSafeLogger("test_plugin", bus, loggateway.NewNoop())
	l.Info("hello", "k", "v")
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		bus.mu.Lock()
		n := len(bus.events)
		bus.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	bus.mu.Lock()
	defer bus.mu.Unlock()
	if len(bus.events) != 1 {
		t.Fatalf("published events=%d want 1", len(bus.events))
	}
	if bus.events[0].Source != "test_plugin" || bus.events[0].Level != "INFO" {
		t.Fatalf("unexpected event: %+v", bus.events[0])
	}
}

type stubMonitorBus struct {
	mu     sync.Mutex
	events []contract.MonitorEvent
}

func (s *stubMonitorBus) Publish(_ context.Context, ev contract.MonitorEvent) {
	s.mu.Lock()
	s.events = append(s.events, ev)
	s.mu.Unlock()
}

func (s *stubMonitorBus) Subscribe(contract.MonitorSubscribeOptions) (<-chan contract.MonitorEvent, func()) {
	ch := make(chan contract.MonitorEvent)
	return ch, func() { close(ch) }
}

func (s *stubMonitorBus) DropCount() uint64 { return 0 }
