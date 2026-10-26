package subagent

import (
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// V 桶 minor pin：turnCtx 此前无剔除路径，长跑进程内存随会话数单调增长。
// 条目只在 Spawn 瞬间被快照消费，超过 turnCtxTTL 的绑定视为未绑定——读路径
// 惰性剔除回退默认 runner，写路径顺带全表清扫。

func TestTurnContextForExpiresStaleBinding(t *testing.T) {
	def := &stubTagRunner{tag: "default"}
	svc, err := NewService(t.TempDir(), def, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	svc.clock = func() time.Time { return now }

	turn := &stubTagRunner{tag: "turn-A"}
	svc.SetTurnContext("sess-a", turn, "prov", "mod", "agent", "key")

	// TTL 内：绑定生效。
	svc.mu.Lock()
	tc := svc.turnContextFor("sess-a")
	svc.mu.Unlock()
	if tc.runner != turn {
		t.Fatalf("fresh binding must resolve the turn runner, got %+v", tc.runner)
	}

	// 越过 TTL：惰性剔除 + 回退默认 runner。
	now = now.Add(turnCtxTTL + time.Minute)
	svc.mu.Lock()
	tc = svc.turnContextFor("sess-a")
	_, stillBound := svc.turnCtx["sess-a"]
	svc.mu.Unlock()
	if tc.runner != def {
		t.Fatalf("expired binding must fall back to the default runner, got %+v", tc.runner)
	}
	if stillBound {
		t.Fatal("expired binding must be lazily deleted, not just ignored")
	}
	if tc.attribution.AgentID != "" {
		t.Fatalf("expired binding must not leak stale attribution, got %+v", tc.attribution)
	}
}

func TestSetTurnContextSweepsStaleBindings(t *testing.T) {
	svc, err := NewService(t.TempDir(), nil, loggateway.NewNoop())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	svc.clock = func() time.Time { return now }

	svc.SetTurnContext("sess-old-1", &stubTagRunner{tag: "1"}, "", "", "", "")
	svc.SetTurnContext("sess-old-2", &stubTagRunner{tag: "2"}, "", "", "", "")

	now = now.Add(turnCtxTTL + time.Minute)
	// 新会话写入触发全表清扫（写路径顺带，避免只依赖读路径逐键碰运气）。
	svc.SetTurnContext("sess-new", &stubTagRunner{tag: "3"}, "", "", "", "")

	svc.mu.Lock()
	defer svc.mu.Unlock()
	if _, ok := svc.turnCtx["sess-old-1"]; ok {
		t.Fatal("stale binding sess-old-1 survived the write-path sweep")
	}
	if _, ok := svc.turnCtx["sess-old-2"]; ok {
		t.Fatal("stale binding sess-old-2 survived the write-path sweep")
	}
	if _, ok := svc.turnCtx["sess-new"]; !ok {
		t.Fatal("fresh binding sess-new must survive the sweep")
	}
}
