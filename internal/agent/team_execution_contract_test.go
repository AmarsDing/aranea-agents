package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestShouldAttachTeamExecutionContract(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ag   biz.Agent
		want bool
	}{
		{
			name: "spirit orchestration face",
			ag: biz.Agent{
				AgentKey: biz.SpiritAgentKey,
				Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "spirit"},
			},
			want: true,
		},
		{
			name: "spirit nil settings still orchestration face",
			ag:   biz.Agent{AgentKey: biz.SpiritAgentKey},
			want: true,
		},
		{
			name: "spirit with computer_use extra belongs to working contract",
			ag: biz.Agent{
				AgentKey: biz.SpiritAgentKey,
				Settings: &biz.AgentRuntimeSettings{
					ToolsEnabled: true, ToolsProfile: "spirit",
					ToolsAllowJSON: `["computer_use_act"]`,
				},
			},
			want: false,
		},
		{
			name: "coding specialist",
			ag: biz.Agent{
				AgentKey: "coder",
				Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"},
			},
			want: false,
		},
		{
			name: "read_only specialist",
			ag: biz.Agent{
				AgentKey: "finance",
				Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"},
			},
			want: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := ShouldAttachTeamExecutionContract(tc.ag); got != tc.want {
				t.Fatalf("ShouldAttachTeamExecutionContract = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestTeamExecutionContractBlock_Content(t *testing.T) {
	t.Parallel()
	spirit := biz.Agent{AgentKey: biz.SpiritAgentKey}
	block := TeamExecutionContractBlock(spirit)
	if !strings.HasPrefix(block, "<team_execution_contract>") || !strings.HasSuffix(block, "</team_execution_contract>") {
		t.Fatalf("block must be tagged, got %q", block)
	}
	// 三节契约：自主执行 / 交付范围 / 进度更新。
	for _, sec := range []string{"Autonomous execution", "Deliverable scope", "Progress updates"} {
		if !strings.Contains(block, sec) {
			t.Fatalf("contract missing section %q", sec)
		}
	}
	// 与既有兜底语义衔接：审批门 + todo_declare_blocker 最后手段。
	if !strings.Contains(block, "todo_declare_blocker") {
		t.Fatal("contract must keep todo_declare_blocker last-resort semantics")
	}
	if got := TeamExecutionContractBlock(biz.Agent{AgentKey: "finance"}); got != "" {
		t.Fatalf("non-spirit agent must get empty block, got %q", got)
	}
}

func TestBuildSystemPrompt_TeamExecutionContractPlacement(t *testing.T) {
	t.Parallel()
	spirit := biz.Agent{
		AgentKey:         biz.SpiritAgentKey,
		AgentDescription: "You orchestrate team runs.",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "spirit"},
	}
	got := BuildSystemPrompt(spirit, []biz.AgentPromptFile{{Name: "RULE.md", Body: "be careful"}}, "complete")
	if !strings.Contains(got, "<team_execution_contract>") {
		t.Fatal("spirit orchestration face must inject team_execution_contract")
	}
	if strings.Contains(got, "<working_contract>") {
		t.Fatal("spirit face must not also receive working_contract (mutual exclusion)")
	}
	descIdx := strings.Index(got, "You orchestrate team runs.")
	contractIdx := strings.Index(got, "<team_execution_contract>")
	cfgIdx := strings.Index(got, `<internal_config name="RULE.md">`)
	if descIdx < 0 || contractIdx < descIdx || cfgIdx < contractIdx {
		t.Fatalf("expected description → team_execution_contract → internal_config, got %q", got)
	}

	coding := biz.Agent{
		AgentKey:         "coder",
		AgentDescription: "You fix Go bugs.",
		Settings:         &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"},
	}
	gotCoding := BuildSystemPrompt(coding, nil, "complete")
	if strings.Contains(gotCoding, "<team_execution_contract>") {
		t.Fatal("coding face must not receive team_execution_contract")
	}
	if !strings.Contains(gotCoding, "<working_contract>") {
		t.Fatal("coding face keeps working_contract")
	}
}
