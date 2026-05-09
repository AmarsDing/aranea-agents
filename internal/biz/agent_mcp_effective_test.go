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
