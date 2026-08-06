package biz

import (
	"context"
	"strings"
	"testing"
)

func TestParseChannelRouting_defaults(t *testing.T) {
	r, err := ParseChannelRouting(`{"type":"feishu","config":{"app_id":"x"},"routing":{}}`)
	if err != nil {
		t.Fatal(err)
	}
	if r.DMScope != "per-channel-peer" {
		t.Fatalf("dm_scope %q", r.DMScope)
	}
}

func TestMatchRoute_glob(t *testing.T) {
	r := ChannelRouting{
		DefaultAgentID: "fallback",
		Rules: []ChannelRouteRule{
			{PeerPattern: "oc_*", AgentID: "special"},
		},
	}
	a, _ := MatchRoute(r, "oc_123")
	if a != "special" {
		t.Fatal(a)
	}
	a2, _ := MatchRoute(r, "other")
	if a2 != "fallback" {
		t.Fatal(a2)
	}
}

// ResolveChannelTarget 的依赖防护必须对称：teams==nil 有防护（返回 apierror），
// agents==nil 此前直接穿透到 agents.GetAgentByID 引发 nil interface panic
// （channel.inbound.background goroutine 中被 safego recover 的 panic 噪音）。
func TestResolveChannelTarget_NilDeps_ReturnsErrorNotPanic(t *testing.T) {
	r := ChannelRouting{DefaultAgentID: "agent-1"}
	// 不得 panic；应返回明确的 apierror。
	_, _, _, err := ResolveChannelTarget(context.Background(), nil, nil, r, "peer-1")
	if err == nil {
		t.Fatal("expected error for nil agents repository, got nil")
	}
	if !strings.Contains(err.Error(), "agent repository not configured") {
		t.Fatalf("expected descriptive apierror, got %v", err)
	}

	// 对称性：team 路由 + nil teams 已有防护，保持不回归。
	rt := ChannelRouting{DefaultTeamID: "team-1"}
	_, _, _, err = ResolveChannelTarget(context.Background(), nil, nil, rt, "peer-1")
	if err == nil || !strings.Contains(err.Error(), "team repository not configured") {
		t.Fatalf("expected team-side guard, got %v", err)
	}
}

