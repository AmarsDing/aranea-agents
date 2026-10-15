package deliverable

import (
	"aranea-agents/internal/biz"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Name returns the toolset name.
func (ToolSet) Name() string { return "deliverable" }

// Close releases resources held by the toolset (none for deliverable).
func (ToolSet) Close() error { return nil }

// Tools returns all deliverable tools as a flat slice (no member contract).
func Tools() []trpctool.Tool {
	return ToolsWithContract(nil)
}

// ToolsWithContract returns the deliverable tools with the member-level
// deliverable contract installed on set_deliverable (nil = uncontracted).
func ToolsWithContract(contract *biz.MemberDeliverableContract) []trpctool.Tool {
	return []trpctool.Tool{
		NewSetDeliverableToolWithContract(contract),
		NewGetDeliverableTool(),
		NewAckDeliverableTool(),
	}
}
