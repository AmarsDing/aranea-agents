package adkruntime

import (
	"strings"

	"google.golang.org/adk/plugin"
	"google.golang.org/adk/tool"
)

func newConfirmationGuardPlugin() (*plugin.Plugin, error) {
	return plugin.New(plugin.Config{
		Name: "confirmation_guard",
		BeforeToolCallback: func(_ tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			reason := highRiskToolReason(t.Name(), args)
			if reason == "" {
				return nil, nil
			}
			return map[string]any{
				"status":  "blocked",
				"action":  "requires_confirmation",
				"message": "High-risk tool call blocked before execution: " + reason,
				"tool":    t.Name(),
			}, nil
		},
	})
}

func newPermissionGuardPlugin() (*plugin.Plugin, error) {
	return newPermissionGuardPluginWithConfig(nil)
}

func newPermissionGuardPluginWithConfig(config map[string]any) (*plugin.Plugin, error) {
	denyTools := configSet(config, "deny_tools", "ADK_PERMISSION_DENY_TOOLS")
	allowAgents := configSet(config, "agent_allowlist", "ADK_PERMISSION_ALLOW_AGENTS")
	blockHighRisk := configBool(config, "block_high_risk", "ADK_PERMISSION_BLOCK_HIGH_RISK", true)
	return plugin.New(plugin.Config{
		Name: "permission_guard",
		BeforeToolCallback: func(ctx tool.Context, t tool.Tool, args map[string]any) (map[string]any, error) {
			agentName := ""
			if ctx != nil {
				agentName = strings.ToLower(strings.TrimSpace(ctx.AgentName()))
			}
			if len(allowAgents) > 0 && !allowAgents[agentName] {
				return policyBlockedToolResult("agent is not allowed to call tools", t.Name()), nil
			}
			toolName := strings.ToLower(strings.TrimSpace(t.Name()))
			if denyTools[toolName] {
				return policyBlockedToolResult("tool is denied by permission policy", t.Name()), nil
			}
			if blockHighRisk && highRiskToolReason(t.Name(), args) != "" {
				return policyBlockedToolResult("high-risk tool is denied by permission policy", t.Name()), nil
			}
			return nil, nil
		},
	})
}
