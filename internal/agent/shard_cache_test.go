package agent

import (
	"context"
	"encoding/json"
	"reflect"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	tooltrpc "aranea-agents/internal/tools/trpc"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// ---------- 测试桩 ----------

// closableToolSet 记录 Close 次数的 ToolSet 桩。
type closableToolSet struct {
	name   string
	closes atomic.Int32
}

func (s *closableToolSet) Name() string                          { return s.name }
func (s *closableToolSet) Tools(context.Context) []trpctool.Tool { return nil }
func (s *closableToolSet) Close() error {
	s.closes.Add(1)
	return nil
}

func shardSpecFor(fp string, cacheable bool, prod *shardProduct, buildCount *atomic.Int32) shardSpec {
	return shardSpec{
		id:        "test:" + fp,
		group:     "test",
		fp:        fp,
		cacheable: cacheable,
		build: func(context.Context) (*shardProduct, error) {
			if buildCount != nil {
				buildCount.Add(1)
			}
			return prod, nil
		},
	}
}

// ---------- A. 归类守卫 ----------

// TestSettingsFieldClassification_Guard 反射枚举 AgentRuntimeSettings 全部
// 字段，强制每个字段在归类表中登记；新增字段未归类则测试红（风险①化解②）。
func TestSettingsFieldClassification_Guard(t *testing.T) {
	st := reflect.TypeOf(biz.AgentRuntimeSettings{})
	for i := 0; i < st.NumField(); i++ {
		name := st.Field(i).Name
		if _, ok := settingsFieldClassification[name]; !ok {
			t.Errorf("字段 %s 未在 settingsFieldClassification 登记（三桶之一）", name)
		}
	}
	// 反向：表中键必须是真实字段（防字段重命名后残留死键）。
	for name := range settingsFieldClassification {
		if _, ok := st.FieldByName(name); !ok {
			t.Errorf("归类表键 %s 不是 AgentRuntimeSettings 的字段（残留死键）", name)
		}
	}
}

// TestSettingsFingerprintView_Buckets 验证指纹视图：full_rebuild 字段保留，
// resolver_managed / no_rebuild 字段清零；仅非 full_rebuild 字段不同的两份
// settings 产生相同视图（不触发重建）。
func TestSettingsFingerprintView_Buckets(t *testing.T) {
	if settingsFingerprintView(nil) != nil {
		t.Fatal("nil 输入应返回 nil")
	}

	base := &biz.AgentRuntimeSettings{
		AgentID:     "a1",
		ToolsEnabled: true,
		// resolver_managed
		ToolsExecutionTimeoutSec: 42,
		// no_rebuild
		HeartbeatEnabled:         true,
		HeartbeatIntervalMinutes: 7,
		UpdatedAt:                "2026-08-16T00:00:00Z",
	}
	v := settingsFingerprintView(base)
	if !v.ToolsEnabled || v.AgentID != "a1" {
		t.Error("full_rebuild 字段应保留")
	}
	if v.ToolsExecutionTimeoutSec != 0 {
		t.Error("resolver_managed 字段 ToolsExecutionTimeoutSec 应清零")
	}
	if v.HeartbeatEnabled || v.HeartbeatIntervalMinutes != 0 || v.UpdatedAt != "" {
		t.Error("no_rebuild 字段应清零")
	}
	// 原对象不被修改。
	if base.ToolsExecutionTimeoutSec != 42 || !base.HeartbeatEnabled {
		t.Error("settingsFingerprintView 不得修改原对象")
	}

	// 仅桶外字段不同的两份 settings → 视图 JSON 相同。
	other := *base
	other.ToolsExecutionTimeoutSec = 99
	other.HeartbeatIntervalMinutes = 30
	other.UpdatedAt = "2026-08-16T11:11:11Z"
	b1, _ := json.Marshal(settingsFingerprintView(base))
	b2, _ := json.Marshal(settingsFingerprintView(&other))
	if string(b1) != string(b2) {
		t.Error("仅 resolver/no_rebuild 字段差异不应改变指纹视图")
	}

	// full_rebuild 字段差异 → 视图不同。
	third := *base
	third.ToolsProfile = "coding"
	b3, _ := json.Marshal(settingsFingerprintView(&third))
	if string(b1) == string(b3) {
		t.Error("full_rebuild 字段差异必须改变指纹视图")
	}
}

// ---------- B. 分片指纹独立性 ----------

// TestShardFingerprint_Determinism 验证指纹的确定性、输入敏感性与组隔离。
func TestShardFingerprint_Determinism(t *testing.T) {
	type proj struct {
		A string
		B int
	}
	fp1 := shardFingerprint("g1", proj{A: "x", B: 1})
	fp2 := shardFingerprint("g1", proj{A: "x", B: 1})
	if fp1 != fp2 {
		t.Error("相同输入必须产生相同指纹")
	}
	if shardFingerprint("g1", proj{A: "x", B: 2}) == fp1 {
		t.Error("字段值变化必须改变指纹")
	}
	if shardFingerprint("g2", proj{A: "x", B: 1}) == fp1 {
		t.Error("不同组必须隔离（group 前缀进指纹）")
	}
	// map 键序不影响（canonical JSON）。
	type withMap struct {
		M map[string]string
	}
	m1 := shardFingerprint("g", withMap{M: map[string]string{"a": "1", "b": "2"}})
	m2 := shardFingerprint("g", withMap{M: map[string]string{"b": "2", "a": "1"}})
	if m1 != m2 {
		t.Error("map 键序不应影响指纹（canonical JSON）")
	}
	// 不可序列化输入 → 随机指纹（恒未命中，安全降级），两次调用不同。
	bad := func() {}
	r1 := shardFingerprint("g", bad)
	r2 := shardFingerprint("g", bad)
	if r1 == r2 {
		t.Error("序列化失败应产生随机指纹（恒未命中降级）")
	}
}

// TestShardFingerprint_MCPProjection 验证 MCP server 指纹投影的完备性
// （影响构建产物的配置字段全部进指纹）与红线（HeaderInjector 不进指纹——
// 其存在即 cacheable=false，不污染共享命名空间）。
func TestShardFingerprint_MCPProjection(t *testing.T) {
	base := tooltrpc.MCPServerConfig{
		Name:      "srv",
		Transport: "streamable_http",
		ServerURL: "http://localhost:8930/mcp",
		Headers:   map[string]string{"Authorization": "Bearer k"},
	}
	fpBase := shardFingerprint(shardGroupMCP, mcpServerFPFromConfig(base))

	mutants := []func(*tooltrpc.MCPServerConfig){
		func(c *tooltrpc.MCPServerConfig) { c.ServerURL = "http://other:8930/mcp" },
		func(c *tooltrpc.MCPServerConfig) { c.Transport = "stdio" },
		func(c *tooltrpc.MCPServerConfig) { c.Command = "npx" },
		func(c *tooltrpc.MCPServerConfig) { c.Args = []string{"-y", "srv"} },
		func(c *tooltrpc.MCPServerConfig) { c.Env = map[string]string{"K": "V"} },
		func(c *tooltrpc.MCPServerConfig) { c.Headers = map[string]string{"Authorization": "Bearer other"} },
		func(c *tooltrpc.MCPServerConfig) { c.TimeoutSec = 30 },
		func(c *tooltrpc.MCPServerConfig) { c.ToolPrefix = "p" },
		func(c *tooltrpc.MCPServerConfig) { c.SessionReconnectMax = 3 },
		func(c *tooltrpc.MCPServerConfig) { c.AllowAdHocHTTP = true },
		func(c *tooltrpc.MCPServerConfig) { c.AdHocTimeoutSec = 10 },
		func(c *tooltrpc.MCPServerConfig) { c.RequireUserCredentials = true },
		func(c *tooltrpc.MCPServerConfig) { c.AuthHeaderName = "X-Key" },
	}
	for i, mutate := range mutants {
		c := base
		mutate(&c)
		if shardFingerprint(shardGroupMCP, mcpServerFPFromConfig(c)) == fpBase {
			t.Errorf("mutant %d：影响构建的 MCP 配置字段变化必须改变分片指纹", i)
		}
	}
}

// TestMcpDirectMountEnabled_BrokerPriority 钉死 B2（2026-08-21 全链路审查）
// 装配矩阵：两键同开只挂 broker（直连不挂载，杜绝远程工具全量 dump 与
// 元工具重复进 tools block）；仅开 mcp_tool_set 且有 server 才直连挂载。
func TestMcpDirectMountEnabled_BrokerPriority(t *testing.T) {
	cases := []struct {
		name        string
		toolSet     bool
		broker      bool
		serverCount int
		want        bool
	}{
		{"both_off", false, false, 2, false},
		{"direct_only_with_servers", true, false, 2, true},
		{"direct_only_no_servers", true, false, 0, false},
		{"broker_only", false, true, 2, false},
		{"both_on_broker_wins", true, true, 2, false},
		{"both_on_no_servers", true, true, 0, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			eff := map[string]bool{
				biz.ToolKeyMCPToolSet: tc.toolSet,
				biz.ToolKeyMCPBroker:  tc.broker,
			}
			if got := mcpDirectMountEnabled(eff, tc.serverCount); got != tc.want {
				t.Errorf("mcpDirectMountEnabled(toolSet=%v, broker=%v, servers=%d) = %v, want %v",
					tc.toolSet, tc.broker, tc.serverCount, got, tc.want)
			}
		})
	}
}

// ---------- C. shardCache 单测 ----------

func TestShardCache_MissThenHit(t *testing.T) {
	c := newShardCache(4)
	var builds atomic.Int32
	prod := &shardProduct{toolSets: []trpctool.ToolSet{&closableToolSet{name: "ts"}}}
	spec := shardSpecFor("fp1", true, prod, &builds)

	p1, r1, err := c.acquire(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if builds.Load() != 1 {
		t.Fatalf("首次 acquire 应构建 1 次，实际 %d", builds.Load())
	}
	p2, r2, err := c.acquire(context.Background(), spec)
	if err != nil {
		t.Fatal(err)
	}
	if builds.Load() != 1 {
		t.Fatal("第二次 acquire 同指纹应命中，不重建")
	}
	if p1 != p2 {
		t.Error("命中应返回同一产物指针")
	}
	if e := c.items["fp1"]; e == nil || e.refs != 2 {
		t.Errorf("refs 应为 2，实际 %+v", e)
	}
	r1()
	if c.items["fp1"].refs != 1 {
		t.Error("release 后 refs 应为 1")
	}
	r2()
	if c.items["fp1"].refs != 0 {
		t.Error("全部释放后 refs 应为 0")
	}
}

func TestShardCache_ReleaseIdempotent(t *testing.T) {
	c := newShardCache(4)
	prod := &shardProduct{}
	_, release, err := c.acquire(context.Background(), shardSpecFor("fp", true, prod, nil))
	if err != nil {
		t.Fatal(err)
	}
	release()
	release()
	release()
	if c.items["fp"].refs != 0 {
		t.Error("幂等 release：refs 不应为负")
	}
}

func TestShardCache_EvictionSkipsReferenced(t *testing.T) {
	c := newShardCache(1)
	idle := &closableToolSet{name: "idle"}
	// A：获取后释放 → 空闲，可淘汰。
	_, relA, _ := c.acquire(context.Background(), shardSpecFor("fpA", true,
		&shardProduct{toolSets: []trpctool.ToolSet{idle}}, nil))
	relA()
	// B：容量 1，A 应被淘汰并同步关闭。
	held := &closableToolSet{name: "held"}
	_, relB, _ := c.acquire(context.Background(), shardSpecFor("fpB", true,
		&shardProduct{toolSets: []trpctool.ToolSet{held}}, nil))
	if idle.closes.Load() != 1 {
		t.Error("LRU 淘汰空闲分片应立即关闭其产物")
	}
	if _, ok := c.items["fpA"]; ok {
		t.Error("被淘汰分片应移出索引")
	}
	// C：B 持有引用时再来 C（容量超限）→ B 不可淘汰，允许暂超容量。
	extra := &closableToolSet{name: "extra"}
	_, relC, _ := c.acquire(context.Background(), shardSpecFor("fpC", true,
		&shardProduct{toolSets: []trpctool.ToolSet{extra}}, nil))
	if held.closes.Load() != 0 {
		t.Error("refs>0 的分片不得被淘汰")
	}
	if len(c.items) != 2 {
		t.Errorf("全部被引用时允许暂超容量，期望 2 条，实际 %d", len(c.items))
	}
	// 释放 B 后，D 入库触发淘汰：B（LRU 尾部空闲）被关闭。
	relB()
	_, relD, _ := c.acquire(context.Background(), shardSpecFor("fpD", true,
		&shardProduct{toolSets: []trpctool.ToolSet{&closableToolSet{name: "d"}}}, nil))
	if held.closes.Load() != 1 {
		t.Error("释放后再次超容，空闲的 B 应被淘汰关闭")
	}
	relC()
	relD()
}

func TestShardCache_UncacheableAlwaysBuilds(t *testing.T) {
	c := newShardCache(4)
	var builds atomic.Int32
	spec := shardSpecFor("fpU", false, nil, &builds)
	spec.build = func(context.Context) (*shardProduct, error) {
		builds.Add(1)
		return &shardProduct{toolSets: []trpctool.ToolSet{&closableToolSet{name: "u"}}}, nil
	}
	p1, r1, _ := c.acquire(context.Background(), spec)
	p2, r2, _ := c.acquire(context.Background(), spec)
	if builds.Load() != 2 {
		t.Error("不可缓存分片每次 acquire 都应新建")
	}
	if p1 == p2 {
		t.Error("不可缓存分片不得返回同一产物")
	}
	if len(c.items) != 0 {
		t.Error("不可缓存分片不得入索引")
	}
	r1()
	r2()
	// 释放即关闭。
	if p1.toolSets[0].(*closableToolSet).closes.Load() != 1 ||
		p2.toolSets[0].(*closableToolSet).closes.Load() != 1 {
		t.Error("不可缓存分片 release 应直接关闭产物")
	}
}

func TestShardCache_ConcurrentCollision(t *testing.T) {
	c := newShardCache(4)
	barrier := make(chan struct{})
	var builds atomic.Int32
	var builtMu sync.Mutex
	var built []*closableToolSet // 记录所有构建产物的 ToolSet（区分入库者/被弃者）
	build := func(context.Context) (*shardProduct, error) {
		builds.Add(1)
		<-barrier // 两边都完成构建后才放行，制造稳定的撞车窗口
		ts := &closableToolSet{name: "race"}
		builtMu.Lock()
		built = append(built, ts)
		builtMu.Unlock()
		return &shardProduct{toolSets: []trpctool.ToolSet{ts}}, nil
	}
	spec := shardSpec{id: "test:race", group: "test", fp: "fpRace", cacheable: true, build: build}

	var wg sync.WaitGroup
	prods := make([]*shardProduct, 2)
	releases := make([]func(), 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			p, r, err := c.acquire(context.Background(), spec)
			if err != nil {
				t.Error(err)
				return
			}
			prods[i], releases[i] = p, r
		}(i)
	}
	// 等两个构建都抵达屏障后放行，保证稳定的撞车窗口。
	deadline := time.Now().Add(2 * time.Second)
	for builds.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(time.Millisecond)
	}
	close(barrier)
	wg.Wait()

	if builds.Load() != 2 {
		t.Fatalf("撞车场景两个构建都应执行，实际 %d", builds.Load())
	}
	if prods[0] != prods[1] {
		t.Error("撞车后两个 acquire 必须拿到同一份入库产物")
	}
	// 被弃产物经 go closeToolSetsNow 异步关闭，轮询等待其完成。
	closeDeadline := time.Now().Add(2 * time.Second)
	var keptCloses, loserCloses int32
	for {
		kept := prods[0].toolSets[0].(*closableToolSet)
		keptCloses = kept.closes.Load()
		loserCloses = 0
		builtMu.Lock()
		for _, ts := range built {
			if ts != kept {
				loserCloses += ts.closes.Load()
			}
		}
		builtMu.Unlock()
		if loserCloses == 1 || time.Now().After(closeDeadline) {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if loserCloses != 1 {
		t.Error("撞车被弃的重复产物应被关闭一次")
	}
	if keptCloses != 0 {
		t.Error("入库产物在引用未释放前不得关闭")
	}
	releases[0]()
	releases[1]()
	if e := c.items["fpRace"]; e == nil || e.refs != 0 {
		t.Error("撞车路径 refs 计数必须配平")
	}
}

func TestShardCache_Close(t *testing.T) {
	c := newShardCache(4)
	idleTS := &closableToolSet{name: "idle"}
	_, relIdle, _ := c.acquire(context.Background(), shardSpecFor("fpI", true,
		&shardProduct{toolSets: []trpctool.ToolSet{idleTS}}, nil))
	relIdle()

	heldTS := &closableToolSet{name: "held"}
	_, relHeld, _ := c.acquire(context.Background(), shardSpecFor("fpH", true,
		&shardProduct{toolSets: []trpctool.ToolSet{heldTS}}, nil))

	if err := c.Close(); err != nil {
		t.Fatal(err)
	}
	if idleTS.closes.Load() != 1 {
		t.Error("Close 应立即关闭空闲分片")
	}
	if heldTS.closes.Load() != 0 {
		t.Error("Close 不得关闭仍被引用的分片")
	}
	if len(c.items) != 0 {
		t.Error("Close 后索引应清空")
	}
	// 最后一次 release 时完成关闭。
	relHeld()
	if heldTS.closes.Load() != 1 {
		t.Error("closing 分片应在最后一次 release 时关闭")
	}
	// Close 幂等。
	if err := c.Close(); err != nil {
		t.Fatal("Close 应幂等")
	}
	// 关闭后 acquire 降级为直建（不缓存），release 直接关闭。
	var builds atomic.Int32
	afterTS := &closableToolSet{name: "after"}
	_, relAfter, err := c.acquire(context.Background(), shardSpecFor("fpAfter", true,
		&shardProduct{toolSets: []trpctool.ToolSet{afterTS}}, &builds))
	if err != nil {
		t.Fatal(err)
	}
	if len(c.items) != 0 {
		t.Error("关闭后 acquire 不得入缓存")
	}
	relAfter()
	if afterTS.closes.Load() != 1 {
		t.Error("关闭后产物的 release 应直接关闭")
	}
}

func TestShardCache_NilBuildReturnsEmpty(t *testing.T) {
	c := newShardCache(4)
	p, release, err := c.acquire(context.Background(), shardSpec{id: "empty", group: "test", fp: "fpNil", cacheable: true})
	if err != nil || p == nil {
		t.Fatal("nil build 应返回空产物")
	}
	release()
}

func TestShardHoldToolSet(t *testing.T) {
	var released atomic.Int32
	h := newShardHoldToolSet("mcp:srv", func() { released.Add(1) })
	if h.Name() != "shard_hold:mcp:srv" {
		t.Errorf("占位符命名不符：%s", h.Name())
	}
	if got := h.Tools(context.Background()); got != nil {
		t.Error("占位符 Tools 必须恒为 nil（不参与工具面）")
	}
	if err := h.Close(); err != nil {
		t.Fatal(err)
	}
	h.Close()
	if released.Load() != 1 {
		t.Error("占位符 Close 应幂等释放一次分片引用")
	}
}

// ---------- D. 基准 ----------

func BenchmarkShardCacheAcquireHit(b *testing.B) {
	c := newShardCache(8)
	spec := shardSpecFor("fpBench", true, &shardProduct{}, nil)
	ctx := context.Background()
	// 预热点：第一次构建入缓存。
	_, rel, _ := c.acquire(ctx, spec)
	rel()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, release, err := c.acquire(ctx, spec)
		if err != nil {
			b.Fatal(err)
		}
		release()
	}
}

func BenchmarkShardFingerprint(b *testing.B) {
	proj := mcpServerShardFP{
		Name:      "srv",
		Transport: "streamable_http",
		ServerURL: "http://localhost:8930/mcp",
		Args:      []string{"-y", "pkg"},
		Env:       map[string]string{"K1": "V1", "K2": "V2"},
		Headers:   map[string]string{"Authorization": "Bearer k"},
	}
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = shardFingerprint(shardGroupMCP, proj)
	}
}
