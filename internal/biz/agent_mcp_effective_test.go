package biz

import (
	"testing"
)

func TestMCPPolicyFromAgentEffectiveTools_allowDeny(t *testing.T) {
	eff := AgentEffectiveTools{
		Allow: []string{"read_file", "mcp:alpha", "mcp:beta"},
		Deny:  []string{"mcp:gamma"},
	}
	pol := MCPPolicyFromAgentEffectiveTools(eff)
	if len(pol.AllowServerKeys) != 2 || pol.AllowServerKeys[0] != "alpha" || pol.AllowServerKeys[1] != "beta" {
		t.Fatalf("allow keys: %#v", pol.AllowServerKeys)
	}
	if len(pol.DenyServerKeys) != 1 || pol.DenyServerKeys[0] != "gamma" {
		t.Fatalf("deny keys: %#v", pol.DenyServerKeys)
	}
}

func TestFilterEffectiveMCPServers_allowAndDeny(t *testing.T) {
	servers := []EffectiveMCPServer{
		{ServerKey: "alpha", ConfigJSON: "{}"},
		{ServerKey: "beta", ConfigJSON: "{}"},
		{ServerKey: "gamma", ConfigJSON: "{}"},
	}
	pol := EffectiveMCPPolicy{AllowServerKeys: []string{"alpha", "beta"}, DenyServerKeys: []string{"beta"}}
	out := FilterEffectiveMCPServers(servers, pol)
	if len(out) != 1 || out[0].ServerKey != "alpha" {
		t.Fatalf("got %#v", out)
	}
}

func TestFilterEffectiveMCPServers_emptyAllowMeansAllExceptDeny(t *testing.T) {
	servers := []EffectiveMCPServer{{ServerKey: "a"}, {ServerKey: "b"}}
	out := FilterEffectiveMCPServers(servers, EffectiveMCPPolicy{DenyServerKeys: []string{"b"}})
	if len(out) != 1 || out[0].ServerKey != "a" {
		t.Fatalf("got %#v", out)
	}
}

func TestFilterEnabledMCPServerRows(t *testing.T) {
	rows := []MCPServer{
		{ID: "1", Key: "enabled-active", Enabled: true, Status: string(AgentStatusActive)},
		{ID: "2", Key: "enabled-empty-status", Enabled: true, Status: ""},
		{ID: "3", Key: "disabled", Enabled: false},
		{ID: "4", Key: "deleted", Enabled: true, DeletedAt: "2026-08-07T00:00:00Z"},
		{ID: "5", Key: "draft", Enabled: true, Status: "draft"},
	}
	out := filterEnabledMCPServerRows(rows)
	if len(out) != 2 {
		t.Fatalf("want 2 surviving rows, got %d: %#v", len(out), out)
	}
	if out[0].ServerKey != "enabled-active" || out[1].ServerKey != "enabled-empty-status" {
		t.Fatalf("unexpected survivors: %#v", out)
	}
	// ConfigJSON must carry over for the runtime conversion path.
	for _, s := range out {
		if s.ID == "" {
			t.Fatalf("ID must be preserved: %#v", s)
		}
	}
}
