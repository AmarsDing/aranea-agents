package agent

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	"github.com/stretchr/testify/require"
	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// hotSwapAgent 是带 SetToolSets 记录的 fake agent：entry 内的 agent 恒为
// runScopedAgent 包装产物，换面经包装器转发到本 fake，断言转发链完整。
type hotSwapAgent struct {
	*mockAgent
	mu        sync.Mutex
	swapCalls int
	lastSets  []trpctool.ToolSet
}

func newHotSwapAgent(key string) *hotSwapAgent {
	return &hotSwapAgent{mockAgent: &mockAgent{key: key}}
}

func (a *hotSwapAgent) SetToolSets(ts []trpctool.ToolSet) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.swapCalls++
	a.lastSets = ts
}

func (a *hotSwapAgent) swapCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.swapCalls
}

func (a *hotSwapAgent) lastSwap() []trpctool.ToolSet {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.lastSets
}

// newHotSwapFixture 用最小真实构建链（eff 仅 datetime 单本地工具，零外部
// 依赖）产出分片计划与合并产物，作为 tryHotSwap 门禁比对的基准面。
// 返回的 holds 是分片引用占位符（非空——core 片 cacheable）。
func newHotSwapFixture(t *testing.T, id string) (biz.Agent, TRPCBuilderDeps, []trpctool.ToolSet, *shardPlan, []string) {
	t.Helper()
	ag := biz.Agent{
		ID:       id,
		AgentKey: "hs-" + id,
		Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true},
	}
	deps := TRPCBuilderDeps{
		TRPCToolAssemblyDeps: TRPCToolAssemblyDeps{
			CachedEffectiveTools: &biz.AgentEffectiveTools{
				ToolsEnabled: true,
				Items:        []biz.EffectiveAgentTool{{ToolKey: "datetime", Enabled: true}},
			},
		},
	}
	ctx := context.Background()
	plan, err := loadToolBuildPlanForSwap(ctx, ag, deps)
	require.NoError(t, err)
	ts, holds, sp, err := buildToolsetsForAgent(ctx, ag, deps, plan)
	require.NoError(t, err)
	require.NotNil(t, sp, "fixture 必须产出分片计划（datetime core 片）")
	require.NotEmpty(t, sp.specs)
	require.NotEmpty(t, holds, "core 片 cacheable，holds 非空")
	return ag, deps, holds, sp, flatToolNames(ts.Tools)
}

// oldFaceOf 按基准面构造兄弟 entry 的面元数据（fp 的 MCPHash 为旧值）。
func oldFaceOf(ag biz.Agent, deps TRPCBuilderDeps, mcpHash string, sp *shardPlan, flatNames []string) (string, *faceMeta) {
	fp := computeBuildKeyFP(ag, deps, deps.ToolVersionHash, deps.SkillVersionHash, mcpHash)
	return fp.key(), &faceMeta{
		fp:        fp,
		shards:    shardMetasOf(sp),
		deferred:  append([]string(nil), sp.deferredTools...),
		flatNames: append([]string(nil), flatNames...),
	}
}

func TestTryHotSwap_Applied(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-applied")
	c := newTestCache(8)
	defer c.Close()

	sibling := newHotSwapAgent("sibling")
	oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
	h := c.putWithFace(oldKey, sibling, nil, holds, oldFace)
	require.NotNil(t, h)

	depsNew := deps
	depsNew.MCPVersionHash = "NEW_MCP"
	newKey := BuildCacheKey(ag, depsNew, "", "", depsNew.MCPVersionHash)
	require.NotEqual(t, oldKey, newKey, "MCPHash 变化必须产生新 key（miss 前提）")

	got := c.tryHotSwap(context.Background(), newKey, ag, depsNew, loggateway.NewNoop())
	require.NotNil(t, got, "四道门禁全过应换面成功")
	require.Equal(t, 1, sibling.swapCount(), "SetToolSets 经包装器转发恰好一次")
	// 注意：fixture 仅 datetime 单本地工具 → ts.ToolSets 为空（无 ToolSet），
	// lastSwap() 返回 nil 是合法结果；swapCount 已证明转发链完整。

	c.mu.Lock()
	_, oldExists := c.items[oldKey]
	entry := c.items[newKey]
	gyLen := len(c.graveyard)
	c.mu.Unlock()
	require.False(t, oldExists, "旧 key 应已移除")
	require.NotNil(t, entry, "entry 应 re-key 到新 key")
	require.Same(t, h, entry.handle, "热替换不换代：handle 存活")
	require.Equal(t, "NEW_MCP", entry.face.fp.MCPHash, "面元数据应更新为新指纹")
	require.Equal(t, 1, gyLen, "旧 hold 组应进 graveyard")

	// 在途语义粗验：refs==0 的退役组 sweeper 当周期关闭（无 panic 即过）。
	c.sweepGraveyard(c.lg)
	c.mu.Lock()
	gyLen = len(c.graveyard)
	c.mu.Unlock()
	require.Equal(t, 0, gyLen, "无在途引用时 sweeper 应即周期回收")
}

func TestTryHotSwap_NoSibling(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-nosib")
	lg := loggateway.NewNoop()
	depsNew := deps
	depsNew.MCPVersionHash = "NEW_MCP"
	newKey := BuildCacheKey(ag, depsNew, "", "", depsNew.MCPVersionHash)

	t.Run("empty cache", func(t *testing.T) {
		c := newTestCache(8)
		defer c.Close()
		require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, lg))
	})

	t.Run("entry without face", func(t *testing.T) {
		c := newTestCache(8)
		defer c.Close()
		oldKey, _ := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
		c.put(oldKey, newHotSwapAgent("noface"), nil, nil) // put 不带 face
		require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, lg),
			"无面元数据的 entry 不得作为换面兄弟")
	})

	t.Run("dirty sibling skipped", func(t *testing.T) {
		c := newTestCache(8)
		defer c.Close()
		oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
		c.putWithFace(oldKey, newHotSwapAgent("dirty"), nil, nil, oldFace)
		c.mu.Lock()
		c.items[oldKey].dirty = true
		c.mu.Unlock()
		require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, lg),
			"脏 entry（后台重建中）不得作为换面兄弟")
	})

	t.Run("delta beyond mcp rejected", func(t *testing.T) {
		c := newTestCache(8)
		defer c.Close()
		oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
		c.putWithFace(oldKey, newHotSwapAgent("sib"), nil, holds, oldFace)
		// ToolHash 也变 → 唯一变量不再是 MCP 配置，拒绝兄弟匹配。
		depsBoth := deps
		depsBoth.ToolVersionHash = "T2"
		depsBoth.MCPVersionHash = "NEW_MCP"
		mixedKey := BuildCacheKey(ag, depsBoth, depsBoth.ToolVersionHash, "", depsBoth.MCPVersionHash)
		require.Nil(t, c.tryHotSwap(context.Background(), mixedKey, ag, depsBoth, lg))
	})
}

func TestTryHotSwap_TopologyChanged(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-topo")
	lg := loggateway.NewNoop()
	depsNew := deps
	depsNew.MCPVersionHash = "NEW_MCP"
	newKey := BuildCacheKey(ag, depsNew, "", "", depsNew.MCPVersionHash)

	t.Run("shard count differs", func(t *testing.T) {
		c := newTestCache(8)
		defer c.Close()
		sibling := newHotSwapAgent("sib")
		oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
		// 追加一个假分片 → 等长校验失败。
		oldFace.shards = append(oldFace.shards, faceShardMeta{id: "mcp:ghost", group: shardGroupMCP, fp: "x"})
		c.putWithFace(oldKey, sibling, nil, holds, oldFace)
		require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, lg))
		require.Equal(t, 0, sibling.swapCount())
		c.mu.Lock()
		_, stillThere := c.items[oldKey]
		c.mu.Unlock()
		require.True(t, stillThere, "回退时兄弟 entry 不得被 re-key/替换")
	})

	t.Run("non-mcp shard fp differs", func(t *testing.T) {
		c := newTestCache(8)
		defer c.Close()
		sibling := newHotSwapAgent("sib")
		oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
		// 篡改 core 组分片指纹（非 mcp 组 fp 差异 = 变量不纯，拒绝）。
		oldFace.shards[0].fp = "tampered"
		c.putWithFace(oldKey, sibling, nil, nil, oldFace)
		require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, lg))
		require.Equal(t, 0, sibling.swapCount())
	})
}

func TestTryHotSwap_DeferredActive(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-deferred")
	c := newTestCache(8)
	defer c.Close()
	sibling := newHotSwapAgent("sib")
	oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
	oldFace.deferred = []string{"some_deferred_tool"} // 框架无 ToolFilter 热替换 API
	c.putWithFace(oldKey, sibling, nil, holds, oldFace)

	depsNew := deps
	depsNew.MCPVersionHash = "NEW_MCP"
	newKey := BuildCacheKey(ag, depsNew, "", "", depsNew.MCPVersionHash)
	require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, loggateway.NewNoop()))
	require.Equal(t, 0, sibling.swapCount())
}

func TestTryHotSwap_FlatChanged(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-flat")
	c := newTestCache(8)
	defer c.Close()
	sibling := newHotSwapAgent("sib")
	oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
	oldFace.flatNames = append(oldFace.flatNames, "ghost_tool") // flat 面无热替换 API
	c.putWithFace(oldKey, sibling, nil, holds, oldFace)

	depsNew := deps
	depsNew.MCPVersionHash = "NEW_MCP"
	newKey := BuildCacheKey(ag, depsNew, "", "", depsNew.MCPVersionHash)
	require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, loggateway.NewNoop()))
	require.Equal(t, 0, sibling.swapCount())
}

func TestTryHotSwap_NoSetter(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-nosetter")
	c := newTestCache(8)
	defer c.Close()
	oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
	c.putWithFace(oldKey, newHotSwapAgent("sib"), nil, holds, oldFace)
	// 剥掉 runScopedAgent 包装 → 裸 mockAgent 无 SetToolSets。
	c.mu.Lock()
	c.items[oldKey].agent = makeAgent("bare")
	c.mu.Unlock()

	depsNew := deps
	depsNew.MCPVersionHash = "NEW_MCP"
	newKey := BuildCacheKey(ag, depsNew, "", "", depsNew.MCPVersionHash)
	require.Nil(t, c.tryHotSwap(context.Background(), newKey, ag, depsNew, loggateway.NewNoop()))
}

// TestTryHotSwap_InFlightHoldsSurviveSwap 验证 FR-2.6 在途安全核心：
// 在途 run 持有代际引用（refs>0）时，旧面 hold 组进 graveyard 但不被关闭；
// refs 归零后 sweeper 当周期回收。
func TestTryHotSwap_InFlightHoldsSurviveSwap(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-inflight")
	c := newTestCache(8)
	defer c.Close()

	probe := &fakeToolSet{name: "inflight-probe"}
	sibling := newHotSwapAgent("sib")
	oldKey, oldFace := oldFaceOf(ag, deps, "OLD_MCP", sp, flatNames)
	h := c.putWithFace(oldKey, sibling, nil, append(holds, probe), oldFace)
	require.NotNil(t, h)
	h.acquire() // 模拟在途 run 持有本代际引用

	depsNew := deps
	depsNew.MCPVersionHash = "NEW_MCP"
	newKey := BuildCacheKey(ag, depsNew, "", "", depsNew.MCPVersionHash)
	got := c.tryHotSwap(context.Background(), newKey, ag, depsNew, loggateway.NewNoop())
	require.NotNil(t, got)

	c.sweepGraveyard(c.lg)
	require.False(t, probe.closed.Load(), "refs>0 时旧面不得关闭（在途零 abort）")

	h.release()
	c.sweepGraveyard(c.lg)
	require.True(t, probe.closed.Load(), "refs 归零后 sweeper 应当周期关闭旧面")
}

// TestHotSwapConcurrentMissCoalesced 验证 singleflight 合流：N 个并发请求对
// 同一新 key miss，热替换仅执行一次，且全量构建桩完全不被调用。
func TestHotSwapConcurrentMissCoalesced(t *testing.T) {
	ag, deps, holds, sp, flatNames := newHotSwapFixture(t, "hs-sf")
	sibling := newHotSwapAgent("sib")
	oldKey, oldFace := oldFaceOf(ag, deps, "OLD_SF", sp, flatNames)
	globalBuildCache.putWithFace(oldKey, sibling, nil, holds, oldFace)
	// 全局缓存换面会触发 retireToolSets → sweeper 启动；globalBuildCache 是
	// 进程级单例不能 Close，cleanup 仅停 sweeper 防 goleak 误报。
	t.Cleanup(func() {
		globalBuildCache.mu.Lock()
		cancel := globalBuildCache.sweeperCancel
		done := globalBuildCache.sweeperDone
		globalBuildCache.sweeperCancel = nil
		globalBuildCache.sweeperDone = nil
		globalBuildCache.mu.Unlock()
		if cancel != nil {
			cancel()
		}
		if done != nil {
			<-done
		}
	})

	var buildCalls atomic.Int32
	withStubbedBuilder(t, func(_ context.Context, _ biz.Agent, _ TRPCBuilderDeps, _ loggateway.Logger) (trpcagent.Agent, []trpctool.ToolSet, *faceMeta, error) {
		buildCalls.Add(1)
		return makeAgent("rebuilt"), nil, nil, nil
	})

	depsNew := deps
	depsNew.MCPVersionHash = "NEW_SF"
	const n = 8
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			a, err := BuildTRPCLLMAgentCached(context.Background(), ag, depsNew, loggateway.NewNoop())
			if err != nil {
				errs <- err
				return
			}
			if a == nil {
				errs <- errors.New("nil agent")
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		require.NoError(t, err)
	}
	require.Equal(t, int32(0), buildCalls.Load(), "热替换成功则全量构建桩不应被调用")
	require.Equal(t, 1, sibling.swapCount(), "singleflight 合流：N 个并发 miss 仅换面一次")
}

// TestBuildKeyFP_MCPOnlyDelta 覆盖兄弟匹配门禁①的全字段组合。
func TestBuildKeyFP_MCPOnlyDelta(t *testing.T) {
	base := buildKeyFP{
		AgentID:      "a1",
		AgentUpdated: "2026-08-16T00:00:00Z",
		ConfigJSON:   "{}",
		DialogMode:   "plan",
		SettingsJSON: `{"x":1}`,
		ToolHash:     "TH",
		SkillHash:    "SH",
		MCPHash:      "M1",
		CustomTools:  []string{"ct1", "ct2"},
	}
	onlyMCP := base
	onlyMCP.MCPHash = "M2"
	require.True(t, base.mcpOnlyDelta(onlyMCP), "仅 MCPHash 异应匹配兄弟")

	same := base
	require.False(t, base.mcpOnlyDelta(same), "完全同指纹不是兄弟（MCPHash 必须异）")

	cases := map[string]func(fp *buildKeyFP){
		"AgentID":      func(fp *buildKeyFP) { fp.AgentID = "a2" },
		"AgentUpdated": func(fp *buildKeyFP) { fp.AgentUpdated = "later" },
		"ConfigJSON":   func(fp *buildKeyFP) { fp.ConfigJSON = `{"changed":true}` },
		"DialogMode":   func(fp *buildKeyFP) { fp.DialogMode = "" },
		"SettingsJSON": func(fp *buildKeyFP) { fp.SettingsJSON = `{"x":2}` },
		"ToolHash":     func(fp *buildKeyFP) { fp.ToolHash = "TH2" },
		"SkillHash":    func(fp *buildKeyFP) { fp.SkillHash = "SH2" },
		"CustomTools":  func(fp *buildKeyFP) { fp.CustomTools = []string{"ct1"} },
		"CustomToolsOrder": func(fp *buildKeyFP) {
			fp.CustomTools = []string{"ct2", "ct1"}
		},
	}
	for name, mutate := range cases {
		fp := base
		fp.MCPHash = "M2" // 保持 MCP 差异，只叠加其他字段变化
		mutate(&fp)
		require.False(t, base.mcpOnlyDelta(fp), "%s 变化应拒绝兄弟匹配", name)
	}
}

func TestFlatToolNames(t *testing.T) {
	require.Nil(t, flatToolNames(nil))
	names := flatToolNames([]trpctool.Tool{
		stubDeclTool{name: "b"},
		stubDeclTool{name: "a"},
		stubDeclTool{name: "b"}, // 去重
		nil,                     // 跳过
	})
	require.Equal(t, []string{"a", "b"}, names)
}

func TestFaceMetaFromPlan_NilSafe(t *testing.T) {
	m := faceMetaFromPlan(buildKeyFP{AgentID: "x"}, nil, nil)
	require.NotNil(t, m)
	require.Nil(t, m.shards)
	require.Nil(t, m.deferred)
	require.Nil(t, m.flatNames)
}
