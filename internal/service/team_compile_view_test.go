package service

import (
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/team"
)

func TestBuildCompiledGraphNodeView(t *testing.T) {
	tests := []struct {
		name            string
		node            biz.NodeDef
		meta            biz.NodeTaskMeta
		metaOK          bool
		member          team.MemberDef
		memberOK        bool
		displayName     func(agentID string) string
		wantDisplayName string
		wantTaskPrompt  string
	}{
		{
			name: "member_name_takes_priority",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "node desc", Instruction: "node instruction",
			},
			member:          team.MemberDef{AgentID: "a1", Name: "Member Name", TaskPrompt: "member task"},
			memberOK:        true,
			displayName:     func(string) string { return "Catalog Name" },
			wantDisplayName: "Member Name",
			wantTaskPrompt:  "node instruction",
		},
		{
			name: "display_name_fallback_when_member_name_empty",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "node desc", Instruction: "node instruction",
			},
			member:          team.MemberDef{AgentID: "a1", Name: "", TaskPrompt: "member task"},
			memberOK:        true,
			displayName:     func(string) string { return "Catalog Name" },
			wantDisplayName: "Catalog Name",
		},
		{
			name: "task_prompt_fallback_to_member_when_node_empty",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "node desc", Instruction: "",
			},
			member:          team.MemberDef{AgentID: "a1", Name: "M1", TaskPrompt: "member task"},
			memberOK:        true,
			displayName:     nil,
			wantDisplayName: "M1",
			wantTaskPrompt:  "member task",
		},
		{
			name: "no_member_uses_node_description_as_display_name",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "node desc", Instruction: "node instruction",
			},
			member:          team.MemberDef{},
			memberOK:        false,
			displayName:     nil,
			wantDisplayName: "node desc",
			wantTaskPrompt:  "node instruction",
		},
		{
			name: "no_member_no_description_uses_agent_name",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "", Instruction: "",
			},
			member:          team.MemberDef{},
			memberOK:        false,
			displayName:     nil,
			wantDisplayName: "key-a1",
		},
		{
			name: "display_name_nil_uses_agent_name_fallback",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "", Instruction: "",
			},
			member:          team.MemberDef{AgentID: "a1", Name: ""},
			memberOK:        true,
			displayName:     nil,
			wantDisplayName: "key-a1",
		},
		{
			name: "display_name_returns_empty_string",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "", Instruction: "",
			},
			member:          team.MemberDef{AgentID: "a1", Name: ""},
			memberOK:        true,
			displayName:     func(string) string { return "" },
			wantDisplayName: "key-a1",
		},
		{
			name: "whitespace_only_member_name_falls_back",
			node: biz.NodeDef{
				ID: "n1", Type: "agent", AgentName: "key-a1",
				Description: "  ", Instruction: "task",
			},
			member:          team.MemberDef{AgentID: "a1", Name: "   "},
			memberOK:        true,
			displayName:     func(string) string { return "Catalog" },
			wantDisplayName: "Catalog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := buildCompiledGraphNodeView(tt.node, tt.meta, tt.metaOK, tt.member, tt.memberOK, tt.displayName)
			if got == nil {
				t.Fatal("expected non-nil view")
			}
			if got.AgentDisplayName != tt.wantDisplayName {
				t.Errorf("AgentDisplayName = %q, want %q", got.AgentDisplayName, tt.wantDisplayName)
			}
			if tt.wantTaskPrompt != "" && got.TaskPrompt != tt.wantTaskPrompt {
				t.Errorf("TaskPrompt = %q, want %q", got.TaskPrompt, tt.wantTaskPrompt)
			}
		})
	}
}

func TestBuildCompiledGraphNodeView_FieldMapping(t *testing.T) {
	node := biz.NodeDef{
		ID: "node-1", Type: "agent", AgentName: "key-x",
		Description: "desc", Instruction: "instr",
	}
	meta := biz.NodeTaskMeta{RequiredRole: "worker"}
	member := team.MemberDef{AgentID: "a1", Name: "Display"}
	displayName := func(string) string { return "" }

	got := buildCompiledGraphNodeView(node, meta, true, member, true, displayName)
	if got.Id != "node-1" {
		t.Errorf("Id = %q, want %q", got.Id, "node-1")
	}
	if got.Type != "agent" {
		t.Errorf("Type = %q, want %q", got.Type, "agent")
	}
	if got.AgentName != "key-x" {
		t.Errorf("AgentName = %q, want %q", got.AgentName, "key-x")
	}
	if got.Role != "worker" {
		t.Errorf("Role = %q, want %q", got.Role, "worker")
	}
	if got.Description != "desc" {
		t.Errorf("Description = %q, want %q", got.Description, "desc")
	}
}

func TestBuildCompiledGraphNodeView_NilDisplayName(t *testing.T) {
	node := biz.NodeDef{
		ID: "n1", Type: "agent", AgentName: "key-a1",
		Description: "desc", Instruction: "instr",
	}
	member := team.MemberDef{AgentID: "a1", Name: "M1"}

	got := buildCompiledGraphNodeView(node, biz.NodeTaskMeta{}, false, member, true, nil)
	if got == nil {
		t.Fatal("expected non-nil view with nil displayName")
	}
	if got.AgentDisplayName != "M1" {
		t.Errorf("AgentDisplayName = %q, want %q", got.AgentDisplayName, "M1")
	}
}

func TestBuildCompiledGraphNodeView_DisplayNamePriorityChain(t *testing.T) {
	t.Run("member_name_over_displayName_func", func(t *testing.T) {
		node := biz.NodeDef{ID: "n1", AgentName: "key-a1", Description: "desc"}
		member := team.MemberDef{AgentID: "a1", Name: "MemberName"}
		displayName := func(string) string { return "CatalogName" }
		got := buildCompiledGraphNodeView(node, biz.NodeTaskMeta{}, false, member, true, displayName)
		if got.AgentDisplayName != "MemberName" {
			t.Errorf("member.Name should win, got %q", got.AgentDisplayName)
		}
	})

	t.Run("displayName_func_over_description", func(t *testing.T) {
		node := biz.NodeDef{ID: "n1", AgentName: "key-a1", Description: "desc"}
		member := team.MemberDef{AgentID: "a1", Name: ""}
		displayName := func(string) string { return "CatalogName" }
		got := buildCompiledGraphNodeView(node, biz.NodeTaskMeta{}, false, member, true, displayName)
		if got.AgentDisplayName != "CatalogName" {
			t.Errorf("displayName func should win over description, got %q", got.AgentDisplayName)
		}
	})

	t.Run("agent_name_final_fallback", func(t *testing.T) {
		node := biz.NodeDef{ID: "n1", AgentName: "key-a1", Description: ""}
		member := team.MemberDef{AgentID: "a1", Name: ""}
		displayName := func(string) string { return "" }
		got := buildCompiledGraphNodeView(node, biz.NodeTaskMeta{}, false, member, true, displayName)
		if got.AgentDisplayName != "key-a1" {
			t.Errorf("AgentName should be final fallback, got %q", got.AgentDisplayName)
		}
	})
}
