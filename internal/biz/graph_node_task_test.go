package biz

import "testing"

func TestShouldCreateTaskForNode(t *testing.T) {
	tests := []struct {
		name string
		node *NodeDef
		meta NodeTaskMeta
		want bool
	}{
		{"nil", nil, NodeTaskMeta{}, false},
		{"agent", &NodeDef{Type: "agent"}, NodeTaskMeta{}, true},
		{"Agent_upper", &NodeDef{Type: "Agent"}, NodeTaskMeta{}, true},
		{"llm", &NodeDef{Type: "llm"}, NodeTaskMeta{}, true},
		{"tool", &NodeDef{Type: "tool"}, NodeTaskMeta{}, true},
		{"tools", &NodeDef{Type: "tools"}, NodeTaskMeta{}, true},
		{"task", &NodeDef{Type: "task"}, NodeTaskMeta{}, true},
		{"review", &NodeDef{Type: "review"}, NodeTaskMeta{}, true},
		{"router", &NodeDef{Type: "router"}, NodeTaskMeta{}, false},
		{"join", &NodeDef{Type: "join"}, NodeTaskMeta{}, false},
		{"function", &NodeDef{Type: "function"}, NodeTaskMeta{}, false},
		{"required_role", &NodeDef{Type: "function"}, NodeTaskMeta{RequiredRole: "admin"}, true},
		{"assignment_mode", &NodeDef{Type: "function"}, NodeTaskMeta{AssignmentMode: "round_robin"}, true},
		{"reviewer_agent", &NodeDef{Type: "function"}, NodeTaskMeta{ReviewerAgent: "reviewer-1"}, true},
		{"whitespace_type", &NodeDef{Type: "  agent  "}, NodeTaskMeta{}, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ShouldCreateTaskForNode(tt.node, tt.meta); got != tt.want {
				t.Errorf("ShouldCreateTaskForNode(%+v, %+v) = %v, want %v", tt.node, tt.meta, got, tt.want)
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
		{"task_with_role", &NodeDef{Type: "task"}, true},
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
