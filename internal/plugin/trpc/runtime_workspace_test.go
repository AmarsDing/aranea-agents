package plugintrpc

import (
	"context"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/loggateway"
)

func TestRuntime_Apply_workspacePartitionsDoNotOverwrite(t *testing.T) {
	rt := NewRuntime(nil, loggateway.NewNoop())

	shared := biz.Plugin{
		Key:               "audit_log",
		Enabled:           true,
		Scope:             "global",
		WorkspaceID:       "",
		ConfigJSON:        `{}`,
		DefaultConfigJSON: `{}`,
	}
	pluginA := biz.Plugin{
		Key:               "skill_usage_tracker",
		Enabled:           true,
		Scope:             "global",
		WorkspaceID:       "ws-a",
		ConfigJSON:        `{}`,
		DefaultConfigJSON: `{}`,
	}
	pluginB := biz.Plugin{
		Key:               "sensitive_data_mask",
		Enabled:           true,
		Scope:             "global",
		WorkspaceID:       "ws-b",
		ConfigJSON:        `{}`,
		DefaultConfigJSON: `{}`,
	}

	ctxA := workspace.WithContext(context.Background(), "ws-a")
	rt.Apply(ctxA, []biz.Plugin{shared, pluginA})

	ctxB := workspace.WithContext(context.Background(), "ws-b")
	// Tenant B reload mirrors List(shared+own); must not wipe ws-a partition.
	rt.Apply(ctxB, []biz.Plugin{shared, pluginB})

	gotA := rt.PluginsForAgent("agent-1", "ws-a")
	if len(gotA) != 2 {
		t.Fatalf("ws-a plugins=%d want 2 (shared+own)", len(gotA))
	}
	gotB := rt.PluginsForAgent("agent-1", "ws-b")
	if len(gotB) != 2 {
		t.Fatalf("ws-b plugins=%d want 2 (shared+own)", len(gotB))
	}
	gotC := rt.PluginsForAgent("agent-1", "ws-c")
	if len(gotC) != 1 {
		t.Fatalf("ws-c should see only shared, got %d", len(gotC))
	}
}

// N-B1：config getter 必须按调用方工作区过滤——本租户插件优先于 shared，
// 其他租户的分区绝不泄漏；system/legacy 调用方确定性取 shared。
func TestRuntime_ConfigGetters_workspaceIsolation(t *testing.T) {
	rt := NewRuntime(nil, loggateway.NewNoop())
	mk := func(ws, tool string) biz.Plugin {
		return biz.Plugin{
			Key:               "confirmation_guard",
			Enabled:           true,
			Scope:             "global",
			WorkspaceID:       ws,
			ConfigJSON:        `{"confirm_tools":["` + tool + `"]}`,
			DefaultConfigJSON: `{}`,
		}
	}
	sysCtx := workspace.WithContext(context.Background(), workspace.SystemWorkspaceID)
	rt.Apply(sysCtx, []biz.Plugin{mk("", "shared_tool"), mk("ws-a", "tenant_a_tool"), mk("ws-b", "tenant_b_tool")})

	// 本租户覆盖 shared。
	cfg, ok := rt.ConfirmationGuardConfigForAgent("agent-1", "ws-a")
	if !ok || len(cfg.ConfirmTools) != 1 || cfg.ConfirmTools[0] != "tenant_a_tool" {
		t.Fatalf("ws-a cfg=%+v ok=%v, want own tenant override", cfg, ok)
	}
	// 无本租户插件的工作区回退 shared，且看不到 ws-a/ws-b 配置。
	cfg, ok = rt.ConfirmationGuardConfigForAgent("agent-1", "ws-c")
	if !ok || len(cfg.ConfirmTools) != 1 || cfg.ConfirmTools[0] != "shared_tool" {
		t.Fatalf("ws-c cfg=%+v ok=%v, want shared fallback", cfg, ok)
	}
	// system/legacy 调用方确定性取 shared（不受 map 迭代序影响）。
	cfg, ok = rt.ConfirmationGuardConfigForAgent("agent-1", "")
	if !ok || len(cfg.ConfirmTools) != 1 || cfg.ConfirmTools[0] != "shared_tool" {
		t.Fatalf("system cfg=%+v ok=%v, want deterministic shared first", cfg, ok)
	}
}
