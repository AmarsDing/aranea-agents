package diagnostics

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// 单项依赖错误只置该项 fail，不影响其余项（降级不 500 的 usecase 层契约）。
func TestRun_singleFailureIsolated(t *testing.T) {
	d := healthyDeps()
	d.ToolAssembly = &fakeToolAssembly{err: errors.New("reconcile boom")}
	d.CacheStats = &fakeCacheStats{err: errors.New("db down")}
	r := newTestUsecase(d).Run(context.Background())
	if got := itemByKey(t, r, KeyToolAssembly).Status; got != StatusFail {
		t.Fatalf("tool_assembly want fail, got %s", got)
	}
	if got := itemByKey(t, r, KeyCacheBaseline).Status; got != StatusFail {
		t.Fatalf("cache_baseline want fail, got %s", got)
	}
	for _, key := range []string{KeyModelProviders, KeyMCPServers, KeyMemoryStack, KeyConfigGraph} {
		if got := itemByKey(t, r, key).Status; got != StatusPass {
			t.Fatalf("%s must stay pass, got %s", key, got)
		}
	}
}

// joinCapped：超 3 个名字截断为 "+N more"。
func TestJoinCapped(t *testing.T) {
	if got := joinCapped([]string{"a", "b"}); got != "a, b" {
		t.Fatalf("short list: %q", got)
	}
	got := joinCapped([]string{"a", "b", "c", "d", "e"})
	if !strings.Contains(got, "a, b, c") || !strings.Contains(got, "+2 more") {
		t.Fatalf("capped: %q", got)
	}
}

// mcpHealthStatusOf：非法 JSON / 缺 key 返回空。
func TestMCPHealthStatusOf(t *testing.T) {
	if got := mcpHealthStatusOf("not-json"); got != "" {
		t.Fatalf("invalid json want empty, got %q", got)
	}
	if got := mcpHealthStatusOf(`{"other":1}`); got != "" {
		t.Fatalf("missing key want empty, got %q", got)
	}
	if got := mcpHealthStatusOf(`{"health_status":" error "}`); got != "error" {
		t.Fatalf("trim: got %q", got)
	}
}
