package agent

import (
	"strings"

	"aranea-agents/internal/biz"
)

// PermissionMode is the session-stable sandbox / write posture shown
// to the model. It is inferred from the tool profile and allow list,
// not from per-turn tool results.
type PermissionMode string

const (
	PermissionToolsOff       PermissionMode = "tools_off"
	PermissionReadOnly       PermissionMode = "read_only"
	PermissionWorkspaceWrite PermissionMode = "workspace_write"
	PermissionNeedsApproval  PermissionMode = "needs_approval"
)

var writeToolSignals = map[string]bool{
	"save_file":            true,
	"replace_content":      true,
	"delete_file":          true,
	"diff_edit":            true,
	"patch_file":           true,
	"shell_exec":           true,
	"computer_use_act":     true,
	"computer_use_launch":  true,
	"coding_dispatch_task": true,
	"group:filesystem":     true,
	"group:runtime":        true,
	"group:computeruse":    true,
}

var approvalToolSignals = map[string]bool{
	"delete_file":          true,
	"shell_exec":           true,
	"computer_use_act":     true,
	"computer_use_launch":  true,
	"coding_dispatch_task": true,
	"group:runtime":        true,
	"group:computeruse":    true,
}

// InferPermissionMode returns the session permission posture for ag.
func InferPermissionMode(ag biz.Agent) PermissionMode {
	if ag.Settings == nil || !ag.Settings.ToolsEnabled {
		return PermissionToolsOff
	}
	profile := normalizeAgentToolProfile(ag.Settings.ToolsProfile)
	allow := agentAllowKeys(ag)
	if profile == "chat_only" && len(allow) == 0 {
		return PermissionToolsOff
	}

	writes, approval := false, false
	switch profile {
	case "coding", "full", "spirit":
		writes = true
		approval = true
	}
	for _, k := range allow {
		if writeToolSignals[k] {
			writes = true
		}
		if approvalToolSignals[k] {
			approval = true
		}
	}
	if !writes {
		return PermissionReadOnly
	}
	if approval {
		return PermissionNeedsApproval
	}
	return PermissionWorkspaceWrite
}

// permissionStateSkipProfiles are canonical profiles whose actual tool domain
// the block text (workspace files / shell / desktop) does not describe:
//   - spirit: orchestrator-only tools (plan_and_execute etc.), no FS/desktop —
//     the hardcoded writes/approval inference would print a factually wrong
//     "workspace-write with approval" claim.
//   - system_memory / system_skills: custom butler toolsets injected outside
//     the catalog; neither read-only nor workspace-write wording applies.
var permissionStateSkipProfiles = map[string]bool{
	"spirit":        true,
	"system_memory": true,
	"system_skills": true,
}

// PermissionStateBlock returns the tagged permission paragraph, or
// empty when the agent has no runtime settings, or when the block's
// workspace-files/shell/desktop wording does not match the agent's
// actual tool domain (see permissionStateSkipProfiles and the chat_only
// business-minimal case below).
func PermissionStateBlock(ag biz.Agent) string {
	if ag.Settings == nil {
		return ""
	}
	mode := InferPermissionMode(ag)
	if mode != PermissionToolsOff {
		canonical := biz.CanonicalToolProfile(ag.Settings.ToolsProfile)
		if permissionStateSkipProfiles[canonical] {
			return ""
		}
		if canonical == "chat_only" && mode == PermissionReadOnly {
			// Business-domain minimal agents (e.g. ops_change_execution): the
			// allow list holds only business tools (twin_*/gns3_*) with no
			// coding write signal, so inference lands on read-only and the
			// block would tell the model to refuse writes and "switch tool
			// profile" — the exact hallucination behind the S05 high-risk
			// tool refusal (session-eval-20260827 T2). Their real posture
			// (HITL-gated high-risk calls) is documented in their own prompt
			// files. chat_only agents with genuine write signals
			// (WorkspaceWrite/NeedsApproval) keep the accurate block.
			return ""
		}
	}
	var body string
	switch mode {
	case PermissionToolsOff:
		body = strings.TrimSpace(`## Current session permissions
- Mode: tools disabled
- You have no tools this session. Answer from context only.
- Do not claim you can read files, edit the workspace, run shell commands, or operate the desktop.`)
	case PermissionReadOnly:
		body = strings.TrimSpace(`## Current session permissions
- Mode: read-only
- You MAY read and search. You MUST NOT claim you can create, edit, or delete files, run shell commands, or operate the desktop.
- If the user asks for a write, say you are in read-only mode and suggest switching this agent's tool profile.`)
	case PermissionWorkspaceWrite:
		body = strings.TrimSpace(`## Current session permissions
- Mode: workspace-write
- You MAY read and edit files under the workspace root (ARANEA_WORKSPACE_ROOT / WORKSPACE_ROOT).
- Do not claim you can change files outside the workspace.`)
	case PermissionNeedsApproval:
		body = strings.TrimSpace(`## Current session permissions
- Mode: workspace-write with approval
- You MAY read and edit files under the workspace root (ARANEA_WORKSPACE_ROOT / WORKSPACE_ROOT).
- Destructive or high-risk tools (exec_command, delete_file, computer_use_act, coding_dispatch_task) require user confirmation before they take effect. Read-only shell commands (go test / go vet, git status/diff/log, rg, ls/dir, linters) may run without a card. Do not claim a gated command already succeeded.
- Do not claim you can change files outside the workspace or bypass confirmation.`)
	default:
		return ""
	}
	return "<permission_state>\n" + body + "\n</permission_state>"
}
