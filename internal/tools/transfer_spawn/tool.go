// Package transfer_spawn exposes spawn_subagent as a function tool alias for ADK agent transfer
// (sets TransferToAgent on the session event, same effect as built-in transfer_to_agent).
package transfer_spawn

import (
	"fmt"
	"strings"

	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/functiontool"
)

type spawnArgs struct {
	AgentName string `json:"agent_name"`
}

const desc = `Spawn / hand off to another agent by its registered agent name (same as transfer_to_agent). Use when the specialist agent is better suited to continue.`

// New builds function tool spawn_subagent (OpenAI/Gemini function-calling entrypoint).
func New() (tool.Tool, error) {
	return functiontool.New(functiontool.Config{
		Name:        "spawn_subagent",
		Description: desc,
	}, func(tc tool.Context, in spawnArgs) (map[string]any, error) {
		name := strings.TrimSpace(in.AgentName)
		if name == "" {
			return nil, fmt.Errorf("agent_name is required")
		}
		tc.Actions().TransferToAgent = name
		return map[string]any{"ok": true, "agent_name": name}, nil
	})
}
