package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestGraphTaskInputFromNode(t *testing.T) {
	cases := []struct {
		name                string
		node                biz.NodeDef
		wantRole            string
		wantAssignmentMode  string
		wantStrategy        string
		wantInput           string
	}{
		{
			name:               "full node",
			node:               biz.NodeDef{ID: "n1", RequiredRole: "admin", AssignmentMode: "dynamic", AssignmentStrategy: "round_robin", Description: "do stuff"},
			wantRole:           "admin",
			wantAssignmentMode: "dynamic",
			wantStrategy:       "round_robin",
			wantInput:          "do stuff",
		},
		{
			name:               "empty assignment mode defaults to static",
			node:               biz.NodeDef{ID: "n2", AssignmentMode: ""},
			wantAssignmentMode: "static",
			wantInput:          "n2",
		},
		{
			name:               "empty description falls back to id",
			node:               biz.NodeDef{ID: "n3", Description: ""},
			wantAssignmentMode: "static",
			wantInput:          "n3",
		},
		{
			name:               "whitespace description falls back to id",
			node:               biz.NodeDef{ID: "n4", Description: "   "},
			wantAssignmentMode: "static",
			wantInput:          "n4",
		},
		{
			name:               "whitespace assignment mode defaults to static",
			node:               biz.NodeDef{ID: "n5", AssignmentMode: "  "},
			wantAssignmentMode: "static",
			wantInput:          "n5",
		},
		{
			name:               "whitespace role trimmed",
			node:               biz.NodeDef{ID: "n6", RequiredRole: "  reviewer  "},
			wantRole:           "reviewer",
			wantAssignmentMode: "static",
			wantInput:          "n6",
		},
		{
			name:               "whitespace strategy trimmed",
			node:               biz.NodeDef{ID: "n7", AssignmentStrategy: "  least_busy  "},
			wantAssignmentMode: "static",
			wantStrategy:       "least_busy",
			wantInput:          "n7",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role, mode, strategy, input := biz.GraphTaskInputFromNode(tc.node)
			if role != tc.wantRole {
				t.Fatalf("role = %q, want %q", role, tc.wantRole)
			}
			if mode != tc.wantAssignmentMode {
				t.Fatalf("mode = %q, want %q", mode, tc.wantAssignmentMode)
			}
			if strategy != tc.wantStrategy {
				t.Fatalf("strategy = %q, want %q", strategy, tc.wantStrategy)
			}
			if input != tc.wantInput {
				t.Fatalf("input = %q, want %q", input, tc.wantInput)
			}
		})
	}
}
