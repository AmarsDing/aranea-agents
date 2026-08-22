package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestShouldAttachWorkingContract(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ag   biz.Agent
		want bool
	}{
		{
			name: "coding profile",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "coding",
			}},
			want: true,
		},
		{
			name: "empty profile defaults to coding",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true,
			}},
			want: true,
		},
		{
			name: "spirit profile",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "spirit",
			}},
			want: true,
		},
		{
			name: "spirit agent key",
			ag: biz.Agent{
				AgentKey: biz.SpiritAgentKey,
				Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "research"},
			},
			want: true,
		},
		{
			name: "computer_use allow extra",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "research",
				ToolsAllowJSON: `["computer_use_act"]`,
			}},
			want: true,
		},
		{
			name: "read_only specialist",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "read_only",
			}},
			want: false,
		},
		{
			name: "research specialist",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "research",
			}},
			want: false,
		},
		{
			name: "tools disabled",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: false, ToolsProfile: "coding",
			}},
			want: false,
		},
		{
			name: "nil settings",
			ag:   biz.Agent{},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ShouldAttachWorkingContract(tc.ag); got != tc.want {
				t.Fatalf("ShouldAttachWorkingContract = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestBuildSystemPrompt_WorkingContractPlacement(t *testing.T) {
	t.Parallel()
	coding := biz.Agent{
		AgentDescription: "You fix Go bugs.",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"},
	}
	got := BuildSystemPrompt(coding, []biz.AgentPromptFile{{Name: "RULE.md", Body: "be careful"}}, "complete")
	if !strings.Contains(got, "<working_contract>") {
		t.Fatal("coding profile must inject working_contract")
	}
	if !strings.Contains(got, "search_content") || !strings.Contains(got, "diff_edit") {
		t.Fatal("working_contract must mention discovery and patch edit tools")
	}
	descIdx := strings.Index(got, "You fix Go bugs.")
	wcIdx := strings.Index(got, "<working_contract>")
	permIdx := strings.Index(got, "<permission_state>")
	cfgIdx := strings.Index(got, `<internal_config name="RULE.md">`)
	if descIdx < 0 || wcIdx < descIdx || permIdx < wcIdx || cfgIdx < permIdx {
		t.Fatalf("expected description → working_contract → permission_state → internal_config, got %q", got)
	}

	finance := biz.Agent{
		AgentDescription: "You answer invoice questions.",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"},
	}
	gotRO := BuildSystemPrompt(finance, nil, "complete")
	if strings.Contains(gotRO, "<working_contract>") {
		t.Fatal("read_only specialist must not receive working_contract")
	}
	if !strings.Contains(gotRO, `Mode: read-only`) {
		t.Fatal("read_only specialist must receive permission_state")
	}
}
