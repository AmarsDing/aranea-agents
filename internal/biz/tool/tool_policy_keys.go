package tool

import "strings"

// toolPolicyKeyAliases maps UI / API / legacy names to platform catalog tool_key.
// Extend here only; effective-tool resolution goes through computePolicyAllowedSet / computePolicyDenySet
// so new aliases or opt-in-only catalog rows do not need scattered special cases.
// Keep naming aligned with internal/tools/registry (see ApplyEffectiveAliases).
var toolPolicyKeyAliases = map[string]string{
	"shell":            "shell_exec",
	"shell_exec":       "exec_command",
	"web_search":       ToolKeyWebResearch,
	"write_file":       "save_file",
	"edit_file":        "diff_edit",
	"list_files":       "list_file",
	"workspace_search": "search_content",
	"gemini_fetch":     "gemini_web_fetch",
	"wikipedia":        "wikipedia_search",
	"email":            "send_email",
	"todo":             "todo_write",
	"await_reply":      "await_user_reply",
}

// NormalizeToolPolicyKey maps a key from allow/deny JSON or profile to catalog tool_key.
func NormalizeToolPolicyKey(key string) string {
	key = strings.TrimSpace(key)
	if key == "" || strings.HasPrefix(key, "group:") {
		return key
	}
	lk := strings.ToLower(key)
	if canon, ok := toolPolicyKeyAliases[lk]; ok {
		return canon
	}
	return key
}

// PropagateAllowAliases copies alias flags to canonical keys (e.g. shell -> shell_exec).
// Repeats until stable to handle chained aliases (shell -> shell_exec -> exec_command).
func PropagateAllowAliases(m map[string]bool) {
	changed := true
	for changed {
		changed = false
		for alias, canon := range toolPolicyKeyAliases {
			if m[alias] && !m[canon] {
				m[canon] = true
				changed = true
			}
		}
	}
}

// PropagateDenyAliases ensures denying either alias or canonical blocks both.
// Repeats until stable to handle chained aliases (shell -> shell_exec -> exec_command).
func PropagateDenyAliases(m map[string]bool) {
	changed := true
	for changed {
		changed = false
		for alias, canon := range toolPolicyKeyAliases {
			if m[alias] && !m[canon] {
				m[canon] = true
				changed = true
			}
			if m[canon] && !m[alias] {
				m[alias] = true
				changed = true
			}
		}
	}
}

// PolicyAliases returns a defensive copy of the policy alias map. Used by
// internal/tools to assert that runtime aliases agree on the same canonical
// target whenever both maps include the same alias key (TPM-P1-01).
func PolicyAliases() map[string]string {
	out := make(map[string]string, len(toolPolicyKeyAliases))
	for k, v := range toolPolicyKeyAliases {
		out[k] = v
	}
	return out
}
