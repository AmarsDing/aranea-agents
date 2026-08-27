package agent

import (
	"context"
	"reflect"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// resetPolicyResolverForTest 保存并清空进程级单例状态，测试结束恢复（与
// inverse/registry_test.go 的 resetForTest 同模式，防用例间污染）。
// Q1 行为模式闸三个阈值 map 一并保存/恢复——漏掉任一都会让
// SetLoopGuardGateThresholds / Reload 的写入泄漏到其他用例。
func resetPolicyResolverForTest(t *testing.T) {
	t.Helper()
	r := globalPolicyResolver
	r.mu.Lock()
	oldMap, oldRepo := r.timeoutSec, r.repo
	oldLoadMax, oldWallSoft, oldWallHard := r.toolLoadMax, r.wallSoftSec, r.wallHardSec
	r.timeoutSec, r.repo = nil, nil
	r.toolLoadMax, r.wallSoftSec, r.wallHardSec = nil, nil, nil
	r.mu.Unlock()
	t.Cleanup(func() {
		r.mu.Lock()
		r.timeoutSec, r.repo = oldMap, oldRepo
		r.toolLoadMax, r.wallSoftSec, r.wallHardSec = oldLoadMax, oldWallSoft, oldWallHard
		r.mu.Unlock()
	})
}

type stubSettingsRepo struct {
	all map[string]biz.AgentRuntimeSettings
}

func (s stubSettingsRepo) GetAgentRuntimeSettings(_ context.Context, agentID string) (biz.AgentRuntimeSettings, error) {
	return s.all[agentID], nil
}

func (s stubSettingsRepo) ListAgentRuntimeSettings(context.Context) (map[string]biz.AgentRuntimeSettings, error) {
	return s.all, nil
}

func (s stubSettingsRepo) UpsertAgentRuntimeSettings(_ context.Context, v biz.AgentRuntimeSettings) (biz.AgentRuntimeSettings, error) {
	return v, nil
}

// TestPolicyResolver_ReloadAndQuery：Reload 全量加载；查询出口规范化
// （0/负值 → 默认，正数 → sec 秒）。
func TestPolicyResolver_ReloadAndQuery(t *testing.T) {
	resetPolicyResolverForTest(t)
	repo := stubSettingsRepo{all: map[string]biz.AgentRuntimeSettings{
		"a-explicit": {AgentID: "a-explicit", ToolsExecutionTimeoutSec: 30},
		"a-default":  {AgentID: "a-default", ToolsExecutionTimeoutSec: 0},
	}}
	InitPolicyResolver(repo, loggateway.NewNoop())

	if got := toolExecutionTimeoutFor("a-explicit", 999); got != 30*time.Second {
		t.Fatalf("explicit 30s: got %v", got)
	}
	// resolver 命中但值为 0 → 规范化默认（区别于 miss 回退构建期值）。
	if got := toolExecutionTimeoutFor("a-default", 999); got != defaultToolExecutionTimeout {
		t.Fatalf("explicit 0 must normalize to default %v, got %v", defaultToolExecutionTimeout, got)
	}
}

// TestPolicyResolver_MissFallsBackToBuildTime：resolver miss（无该 agent 行）
// 回退构建期快照值——未 Reload/未初始化的部署形态行为与改造前等价。
func TestPolicyResolver_MissFallsBackToBuildTime(t *testing.T) {
	resetPolicyResolverForTest(t)
	// 未 Init（repo=nil）：直接回退。
	if got := toolExecutionTimeoutFor("ghost", 45); got != 45*time.Second {
		t.Fatalf("miss with build-time 45: got %v", got)
	}
	// 构建期值也是 0 → 双重缺省落默认。
	if got := toolExecutionTimeoutFor("ghost", 0); got != defaultToolExecutionTimeout {
		t.Fatalf("miss with build-time 0 must fall to default: got %v", got)
	}

	InitPolicyResolver(stubSettingsRepo{all: map[string]biz.AgentRuntimeSettings{}}, loggateway.NewNoop())
	if got := toolExecutionTimeoutFor("ghost", 45); got != 45*time.Second {
		t.Fatalf("after Reload, unknown agent still falls back to build-time 45: got %v", got)
	}
}

// TestPolicyResolver_SetTakesEffectImmediately：Set 后下一次查询即新值
// （service 层「仅策略字段变化」路径的语义等价物）。
func TestPolicyResolver_SetTakesEffectImmediately(t *testing.T) {
	resetPolicyResolverForTest(t)
	InitPolicyResolver(stubSettingsRepo{all: map[string]biz.AgentRuntimeSettings{
		"a": {AgentID: "a", ToolsExecutionTimeoutSec: 10},
	}}, loggateway.NewNoop())
	if got := toolExecutionTimeoutFor("a", 0); got != 10*time.Second {
		t.Fatalf("pre-Set: got %v", got)
	}
	SetToolExecutionTimeout("a", 120)
	if got := toolExecutionTimeoutFor("a", 0); got != 120*time.Second {
		t.Fatalf("post-Set must be 120s immediately: got %v", got)
	}
	// Set 0 = 显式恢复默认（与 DB 语义一致）。
	SetToolExecutionTimeout("a", 0)
	if got := toolExecutionTimeoutFor("a", 999); got != defaultToolExecutionTimeout {
		t.Fatalf("Set 0 must mean explicit-default (resolver hit), got %v", got)
	}
}

// TestToolExecutionTimeoutHooks_PerCallLookup（AC 核心）：hook 构建后策略值
// 变化，BeforeTool 注入的 deadline 反映新值——每调用查询，零重建。
func TestToolExecutionTimeoutHooks_PerCallLookup(t *testing.T) {
	resetPolicyResolverForTest(t)
	InitPolicyResolver(stubSettingsRepo{all: map[string]biz.AgentRuntimeSettings{
		"agent-x": {AgentID: "agent-x", ToolsExecutionTimeoutSec: 60},
	}}, loggateway.NewNoop())

	// 镜像 callback_chain.go 的生产接线形态。
	hooks := toolExecutionTimeoutHooks(func() time.Duration {
		return toolExecutionTimeoutFor("agent-x", 0)
	}, loggateway.NewNoop())
	if len(hooks) != 2 {
		t.Fatalf("want 2 hooks, got %d", len(hooks))
	}
	type beforeToolHook interface {
		HandleBeforeTool(context.Context, *trpctool.BeforeToolArgs) (*trpctool.BeforeToolResult, error)
	}
	deadlineFor := func() time.Duration {
		res, err := hooks[0].(beforeToolHook).HandleBeforeTool(context.Background(), &trpctool.BeforeToolArgs{ToolCallID: "call-1"})
		if err != nil {
			t.Fatalf("before hook: %v", err)
		}
		dl, ok := res.Context.Deadline()
		if !ok {
			t.Fatal("before hook must inject a deadline")
		}
		return time.Until(dl)
	}
	if d := deadlineFor(); d < 55*time.Second || d > 61*time.Second {
		t.Fatalf("initial 60s policy: remaining = %v", d)
	}

	SetToolExecutionTimeout("agent-x", 300)
	if d := deadlineFor(); d < 295*time.Second || d > 301*time.Second {
		t.Fatalf("after Set(300), per-call lookup must yield ~300s: remaining = %v", d)
	}
}

// TestPolicyStrippedFields_Guard（report-05 P0-2 风险①同型守卫）：反射枚举
// AgentRuntimeSettings 全部字段，逐字段置非零后过 policyStrippedSettings——
// 被清零的字段必须且仅须是 resolverManagedPolicyFields 登记项。新增字段若
// 被剥离而未登记（或登记了却未剥离），本测试红。
func TestPolicyStrippedFields_Guard(t *testing.T) {
	managed := map[string]bool{}
	for _, n := range resolverManagedPolicyFields {
		managed[n] = true
	}
	typ := reflect.TypeOf(biz.AgentRuntimeSettings{})
	strippedSeen := map[string]bool{}
	for i := 0; i < typ.NumField(); i++ {
		f := typ.Field(i)
		s := biz.AgentRuntimeSettings{}
		v := reflect.ValueOf(&s).Elem().Field(i)
		if !v.CanSet() {
			continue
		}
		switch v.Kind() {
		case reflect.Bool:
			v.SetBool(true)
		case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
			v.SetInt(1)
		case reflect.Float32, reflect.Float64:
			v.SetFloat(0.5)
		case reflect.String:
			v.SetString("x")
		default:
			t.Fatalf("field %s has unhandled kind %s — extend this guard helper", f.Name, v.Kind())
		}
		stripped := policyStrippedSettings(&s)
		got := reflect.ValueOf(stripped).Elem().Field(i)
		if got.IsZero() {
			strippedSeen[f.Name] = true
			if !managed[f.Name] {
				t.Errorf("field %s stripped by policyStrippedSettings but NOT registered in resolverManagedPolicyFields", f.Name)
			}
		}
	}
	for _, n := range resolverManagedPolicyFields {
		if !strippedSeen[n] {
			t.Errorf("resolverManagedPolicyFields registers %s but policyStrippedSettings does not strip it", n)
		}
	}
}

// TestBuildCacheKey_ResolverManagedFieldInsensitive（AC 指纹侧）：resolver 化
// 字段变化不改变缓存键（零重建前提）；未 resolver 化字段变化仍改变键
// （保守兜底不回归）。
func TestBuildCacheKey_ResolverManagedFieldInsensitive(t *testing.T) {
	base := biz.Agent{ID: "ag-1", Settings: &biz.AgentRuntimeSettings{ToolsExecutionTimeoutSec: 10}}
	changed := biz.Agent{ID: "ag-1", Settings: &biz.AgentRuntimeSettings{ToolsExecutionTimeoutSec: 999}}
	if BuildCacheKey(base, TRPCBuilderDeps{}, "h", "", "") != BuildCacheKey(changed, TRPCBuilderDeps{}, "h", "", "") {
		t.Fatal("resolver-managed field (ToolsExecutionTimeoutSec) must not change the cache key")
	}

	other := biz.Agent{ID: "ag-1", Settings: &biz.AgentRuntimeSettings{ToolsExecutionTimeoutSec: 10, ToolsRetryMaxAttempts: 7}}
	if BuildCacheKey(base, TRPCBuilderDeps{}, "h", "", "") == BuildCacheKey(other, TRPCBuilderDeps{}, "h", "", "") {
		t.Fatal("non-managed field (ToolsRetryMaxAttempts) must still change the cache key (conservative default)")
	}
}
