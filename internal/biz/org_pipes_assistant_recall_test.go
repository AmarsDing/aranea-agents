package biz

import (
	"strings"
	"testing"
)

// spec §6.3 召回接线：个人助理类 agent 默认 ["agent","team","user"]（显式
// 配置优先）；非个人助理（含 Clamp 后的专员）不受影响。
func TestApplyPersonalAssistantL3RecallDefault(t *testing.T) {
	cases := []struct {
		name   string
		policy MemoryRuntimePolicy
		agent  Agent
		want   []string
	}{
		{
			name:   "spirit+默认域→追加user",
			policy: MemoryRuntimePolicy{L3RecallScopes: []string{"agent", "team"}, L3RecallScopesDefaulted: true},
			agent:  Agent{AgentKey: SpiritAgentKey},
			want:   []string{"agent", "team", "user"},
		},
		{
			name:   "spirit+显式配置→不动",
			policy: MemoryRuntimePolicy{L3RecallScopes: []string{"agent"}, L3RecallScopesDefaulted: false},
			agent:  Agent{AgentKey: SpiritAgentKey},
			want:   []string{"agent"},
		},
		{
			name:   "metadata标记的自建助理→追加user",
			policy: MemoryRuntimePolicy{L3RecallScopes: []string{"agent", "team"}, L3RecallScopesDefaulted: true},
			agent:  Agent{AgentKey: "ag_custom", MetadataJSON: `{"memory_personal_assistant":true}`},
			want:   []string{"agent", "team", "user"},
		},
		{
			name:   "已有user→不重复追加",
			policy: MemoryRuntimePolicy{L3RecallScopes: []string{"agent", "user"}, L3RecallScopesDefaulted: true},
			agent:  Agent{AgentKey: SpiritAgentKey},
			want:   []string{"agent", "user"},
		},
		{
			name:   "普通agent+默认域→不动",
			policy: MemoryRuntimePolicy{L3RecallScopes: []string{"agent", "team"}, L3RecallScopesDefaulted: true},
			agent:  Agent{AgentKey: "be"},
			want:   []string{"agent", "team"},
		},
		{
			name:   "专员被Clamp后→本函数不再放宽",
			policy: MemoryRuntimePolicy{L3RecallScopes: OrgMemberL3Scopes(), L3RecallScopesDefaulted: true},
			agent:  Agent{AgentKey: "be"},
			want:   OrgMemberL3Scopes(),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p := c.policy
			ApplyPersonalAssistantL3RecallDefault(&p, c.agent)
			if strings.Join(p.L3RecallScopes, ",") != strings.Join(c.want, ",") {
				t.Fatalf("scopes = %v, want %v", p.L3RecallScopes, c.want)
			}
		})
	}
	// nil 策略不崩。
	var nilP *MemoryRuntimePolicy
	ApplyPersonalAssistantL3RecallDefault(nilP, Agent{AgentKey: SpiritAgentKey})
}

// Resolve 填充默认时必须打 Defaulted 标记（个人助理默认注入的开关）。
func TestResolveMemoryRuntimePolicy_MarksDefaultedScopes(t *testing.T) {
	p := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{MemoryEnabled: true})
	if !p.L3RecallScopesDefaulted {
		t.Fatal("默认填充的 L3RecallScopes 必须置 L3RecallScopesDefaulted")
	}
	explicit := `["agent","workspace"]`
	p2 := ResolveMemoryRuntimePolicy(&AgentRuntimeSettings{MemoryEnabled: true, L3RecallScopesJSON: explicit})
	if p2.L3RecallScopesDefaulted {
		t.Fatal("显式配置的 L3RecallScopes 不得置 Defaulted")
	}
	if strings.Join(p2.L3RecallScopes, ",") != "agent,workspace" {
		t.Fatalf("显式 scopes = %v", p2.L3RecallScopes)
	}
}
