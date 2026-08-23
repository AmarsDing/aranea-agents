package agent

import (
	_ "embed"
	"encoding/json"
	"strings"

	"aranea-agents/internal/biz"
)

//go:embed prompts/working_contract.md
var workingContractBody string

// workingContractWriteSignals are allow-list keys that mean the agent
// owns a coding / computer-use / shell face and should receive the
// working-contract prompt. Specialists on read_only / research / chat
// stay clean unless they explicitly opt into these tools.
var workingContractWriteSignals = map[string]bool{
	"coding_dispatch_task":    true,
	"coding_check_task":       true,
	"coding_cancel_task":      true,
	"computer_use_act":        true,
	"computer_use_launch":     true,
	"computer_use_observe":    true,
	"computer_use_screenshot": true,
	"computer_use_session":    true,
	"diff_edit":               true,
	"patch_file":              true,
	"shell_exec":              true,
	"group:computeruse":       true,
	"group:runtime":           true,
}

// ShouldAttachWorkingContract reports whether the Codex-style working
// contract belongs on this agent's system prompt. It is limited to the
// coding profile, full kitchen-sink profile, and explicit coding /
// computer-use allow extras — not the spirit orchestrator (no file/shell
// tools on the idle face) and not finance or CS specialists on
// read_only / research / chat_only.
func ShouldAttachWorkingContract(ag biz.Agent) bool {
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return false
	}
	// Spirit's empty/missing profile must not inherit the coding default.
	if strings.TrimSpace(ag.AgentKey) != biz.SpiritAgentKey {
		switch normalizeAgentToolProfile(ag.Settings.ToolsProfile) {
		case "coding", "full":
			return true
		}
	}
	for _, k := range agentAllowKeys(ag) {
		if workingContractWriteSignals[k] {
			return true
		}
	}
	return false
}

// WorkingContractBlock returns the tagged working-contract prompt, or
// empty when the agent should not receive it.
func WorkingContractBlock(ag biz.Agent) string {
	if !ShouldAttachWorkingContract(ag) {
		return ""
	}
	body := strings.TrimSpace(workingContractBody)
	if body == "" {
		return ""
	}
	return "<working_contract>\n" + body + "\n</working_contract>"
}

func normalizeAgentToolProfile(profile string) string {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "":
		return "coding"
	case "chat_only", "minimal":
		return "chat_only"
	case "read_only", "safe":
		return "read_only"
	default:
		return strings.ToLower(strings.TrimSpace(profile))
	}
}

func agentAllowKeys(ag biz.Agent) []string {
	if ag.Settings == nil {
		return nil
	}
	raw := strings.TrimSpace(ag.Settings.ToolsAllowJSON)
	if raw == "" {
		return nil
	}
	var keys []string
	if err := json.Unmarshal([]byte(raw), &keys); err != nil {
		return nil
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.ToLower(strings.TrimSpace(k))
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}
