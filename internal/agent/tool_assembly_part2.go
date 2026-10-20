package agent

import (
	"aranea-agents/internal/tools"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

func normalizeMCPServerTimeout(sec int) int {
	if sec <= 0 {
		return tools.DefaultMCPServerTimeoutSec
	}
	return sec
}

func filterMemoryTools(tools []trpctool.Tool) []trpctool.Tool {
	filtered := make([]trpctool.Tool, 0, len(tools))
	for _, t := range tools {
		if d := t.Declaration(); d != nil && d.Name == "memory_clear" {
			continue
		}
		filtered = append(filtered, t)
	}
	return filtered
}
