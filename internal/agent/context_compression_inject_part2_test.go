package agent

import (
	"testing"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// assertNoOrphanToolPairs checks that every tool-call in msgs has its
// tool-result also in msgs, and vice versa.
func assertNoOrphanToolPairs(t *testing.T, msgs []trpcmodel.Message, side string) {
	t.Helper()
	toolCallIDs := make(map[string]bool)
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			toolCallIDs[tc.ID] = true
		}
	}
	for _, m := range msgs {
		if m.Role == trpcmodel.RoleTool && m.ToolID != "" {
			if !toolCallIDs[m.ToolID] {
				t.Fatalf("%s: orphan tool_result %q (no matching tool_call in %s)", side, m.ToolID, side)
			}
		}
	}
	toolResultIDs := make(map[string]bool)
	for _, m := range msgs {
		if m.Role == trpcmodel.RoleTool && m.ToolID != "" {
			toolResultIDs[m.ToolID] = true
		}
	}
	for _, m := range msgs {
		for _, tc := range m.ToolCalls {
			if !toolResultIDs[tc.ID] {
				t.Fatalf("%s: orphan tool_call %q (no matching tool_result in %s)", side, tc.ID, side)
			}
		}
	}
}
