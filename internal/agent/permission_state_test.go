package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestInferPermissionMode(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		ag   biz.Agent
		want PermissionMode
	}{
		{
			name: "nil settings",
			ag:   biz.Agent{},
			want: PermissionToolsOff,
		},
		{
			name: "tools disabled",
			ag:   biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: false, ToolsProfile: "coding"}},
			want: PermissionToolsOff,
		},
		{
			name: "chat_only",
			ag:   biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "chat_only"}},
			want: PermissionToolsOff,
		},
		{
			name: "read_only",
			ag:   biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"}},
			want: PermissionReadOnly,
		},
		{
			name: "research",
			ag:   biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "research"}},
			want: PermissionReadOnly,
		},
		{
			name: "coding needs approval for shell",
			ag:   biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"}},
			want: PermissionNeedsApproval,
		},
		{
			name: "research plus save_file",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "research",
				ToolsAllowJSON: `["save_file"]`,
			}},
			want: PermissionWorkspaceWrite,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := InferPermissionMode(tc.ag); got != tc.want {
				t.Fatalf("InferPermissionMode = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestPermissionStateBlock_ReadOnlyWording(t *testing.T) {
	t.Parallel()
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "read_only"}}
	got := PermissionStateBlock(ag)
	if !strings.Contains(got, "<permission_state>") {
		t.Fatal("expected tagged block")
	}
	if !strings.Contains(got, "read-only") {
		t.Fatalf("expected read-only wording, got %q", got)
	}
	if strings.Contains(got, "You MAY read and edit files") {
		t.Fatal("read-only must not claim workspace write")
	}
}

func TestPermissionStateBlock_ApprovalWording(t *testing.T) {
	t.Parallel()
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{ToolsEnabled: true, ToolsProfile: "coding"}}
	got := PermissionStateBlock(ag)
	if !strings.Contains(got, "workspace-write with approval") {
		t.Fatalf("coding profile must mention approval, got %q", got)
	}
	if !strings.Contains(got, "Do not claim a gated command already succeeded") {
		t.Fatal("approval mode must forbid claiming unconfirmed success")
	}
	if !strings.Contains(got, "go test") {
		t.Fatal("approval mode must mention read-only shell skip")
	}
}

// Domain gate (session-eval-20260827 T2 follow-up): the block's workspace
// files/shell/desktop wording must not be injected for agents whose real tool
// domain it misdescribes — business minimal agents (S05 root cause), spirit,
// and the memory/skills butlers. read_only/research/coding/full and the
// domain-neutral tools-off block are unaffected.
func TestPermissionStateBlock_DomainGate(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name       string
		ag         biz.Agent
		wantAttach bool
	}{
		{
			name: "business minimal agent (ops_change_execution shape) skipped",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "minimal",
				ToolsAllowJSON: `["twin_alarm_query","gns3_exec","gns3_fault_inject","gns3_fault_clear","twin_config_push"]`,
			}},
			wantAttach: false,
		},
		{
			name: "spirit orchestrator skipped",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "spirit",
			}},
			wantAttach: false,
		},
		{
			name: "system_memory butler skipped",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "system_memory",
			}},
			wantAttach: false,
		},
		{
			name: "system_skills butler skipped",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "system_skills",
			}},
			wantAttach: false,
		},
		{
			name: "read_only specialist keeps block",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "read_only",
			}},
			wantAttach: true,
		},
		{
			name: "research keeps block",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "research",
			}},
			wantAttach: true,
		},
		{
			name: "coding keeps block",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "coding",
			}},
			wantAttach: true,
		},
		{
			name: "chat_only without allow keeps tools-off block",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "chat_only",
			}},
			wantAttach: true,
		},
		{
			name: "tools disabled keeps tools-off block",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: false, ToolsProfile: "read_only",
			}},
			wantAttach: true,
		},
		{
			name: "chat_only with genuine write signal keeps block",
			ag: biz.Agent{Settings: &biz.AgentRuntimeSettings{
				ToolsEnabled: true, ToolsProfile: "chat_only",
				ToolsAllowJSON: `["save_file"]`,
			}},
			wantAttach: true,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := PermissionStateBlock(tc.ag)
			attached := strings.Contains(got, "<permission_state>")
			if attached != tc.wantAttach {
				t.Fatalf("PermissionStateBlock attached = %v, want %v (got %q)", attached, tc.wantAttach, got)
			}
		})
	}
}
