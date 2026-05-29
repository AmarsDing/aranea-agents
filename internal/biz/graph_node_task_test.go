package biz

import "testing"

func TestShouldCreateTaskForNode(t *testing.T) {
	tests := []struct {
		name string
		node *NodeDef
		want bool
	}{
		{"nil", nil, false},
		{"agent", &NodeDef{Type: "agent"}, true},
		{"Agent_upper", &NodeDef{Type: "Agent"}, true},
		{"llm", &NodeDef{Type: "llm"}, true},
		{"tool", &NodeDef{Type: "tool"}, true},
		{"tools", &NodeDef{Type: "tools"}, true},
		{"task", &NodeDef{Type: "task"}, true},
		{"review", &NodeDef{Type: "review"}, true},
		{"router", &NodeDef{Type: "router"}, false},
		{"join", &NodeDef{Type: "join"}, false},
		{"function", &NodeDef{Type: "function"}, false},
		{"required_role", &NodeDef{Type: "function", RequiredRole: "admin"}, true},
		{"assignment_mode", &NodeDef{Type: "function", AssignmentMode: "round_robin"}, true},
		{"reviewer_agent", &NodeDef{Type: "function", ReviewerAgent: "reviewer-1"}, true},
		{"whitespace_type", &NodeDef{Type: "  agent  "}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldCreateTaskForNode(tt.node); got != tt.want {
				t.Errorf("ShouldCreateTaskForNode(%+v) = %v, want %v", tt.node, got, tt.want)
			}
		})
	}
}

func TestShouldCreateTeamGraphTaskNode(t *testing.T) {
	tests := []struct {
		name string
		node *NodeDef
		want bool
	}{
		{"nil", nil, false},
		{"task", &NodeDef{Type: "task"}, true},
		{"review", &NodeDef{Type: "review"}, true},
		{"agent", &NodeDef{Type: "agent"}, false},
		{"llm", &NodeDef{Type: "llm"}, false},
		{"tool", &NodeDef{Type: "tool"}, false},
		{"router", &NodeDef{Type: "router"}, false},
		{"function", &NodeDef{Type: "function"}, false},
		{"task_with_role", &NodeDef{Type: "task", RequiredRole: "admin"}, true},
		{"Task_upper", &NodeDef{Type: "Task"}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldCreateTeamGraphTaskNode(tt.node); got != tt.want {
				t.Errorf("ShouldCreateTeamGraphTaskNode(%+v) = %v, want %v", tt.node, got, tt.want)
			}
		})
	}
}
