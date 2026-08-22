package agentbridge

import "strings"

// DefaultACPLaunch returns the argv Codex/Claude/CodeBuddy adapters use
// when a coding agent is registered without an explicit command (E9 / M2-5).
// This is a launch preset, not a second protocol: all three still speak ACP
// over stdio. Unknown keys leave command empty so the API can require it.
func DefaultACPLaunch(agentKey string) (command string, args []string, ok bool) {
	switch normalizeCodingAgentKey(agentKey) {
	case "codebuddy":
		return "codebuddy", []string{"--acp"}, true
	case "claude_code":
		return "claude-code-acp", nil, true
	case "codex":
		return "npx", []string{"-y", "@zed-industries/codex-acp"}, true
	default:
		return "", nil, false
	}
}

func normalizeCodingAgentKey(agentKey string) string {
	k := strings.ToLower(strings.TrimSpace(agentKey))
	k = strings.ReplaceAll(k, "-", "_")
	switch k {
	case "claude", "claudecode", "claude_code_acp":
		return "claude_code"
	case "openai_codex", "codex_acp":
		return "codex"
	case "code_buddy", "tencent_codebuddy":
		return "codebuddy"
	default:
		return k
	}
}

// ApplyDefaultLaunch fills Command/Args when the caller omitted them and a
// known adapter preset exists. Existing explicit argv is left untouched.
func ApplyDefaultLaunch(agent *CodingAgent) {
	if agent == nil {
		return
	}
	if strings.TrimSpace(agent.Command) != "" {
		return
	}
	cmd, args, ok := DefaultACPLaunch(agent.AgentKey)
	if !ok {
		return
	}
	agent.Command = cmd
	if len(agent.Args) == 0 {
		agent.Args = append([]string(nil), args...)
	}
}
