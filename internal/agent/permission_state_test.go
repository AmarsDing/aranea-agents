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
	if !strings.Contains(got, "Do not claim they already succeeded") {
		t.Fatal("approval mode must forbid claiming unconfirmed success")
	}
}
