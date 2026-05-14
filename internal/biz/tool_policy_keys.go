package biz

import "strings"

// toolPolicyKeyAliases maps UI / API / legacy names to platform catalog tool_key.
// Extend here only; effective-tool resolution goes through computePolicyAllowedSet / computePolicyDenySet
// so new aliases or opt-in-only catalog rows do not need scattered special cases.
// Keep naming aligned with internal/tools/registry (see ApplyEffectiveAliases).
var toolPolicyKeyAliases = map[string]string{
	"shell":            "shell_exec",
	"web_search":       "duckduckgo_search",
	"write_file":       "save_file",
	"edit_file":        "replace_content",
	"list_files":       "list_file",
	"workspace_search": "search_content",
	"gemini_fetch":     "gemini_web_fetch",
	"wikipedia":        "wikipedia_search",
	"email":            "send_email",
	"todo":             "todo_write",
	"await_reply":      "await_user_reply",
}

// normalizeToolPolicyKey maps a key from allow/deny JSON or profile to catalog tool_key.
func normalizeToolPolicyKey(key string) string {
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

// propagateAllowAliases copies alias flags to canonical keys (e.g. shell → shell_exec).
func propagateAllowAliases(m map[string]bool) {
	for alias, canon := range toolPolicyKeyAliases {
		if m[alias] {
			m[canon] = true
		}
	}
}

// propagateDenyAliases ensures denying either alias or canonical blocks both.
func propagateDenyAliases(m map[string]bool) {
	for alias, canon := range toolPolicyKeyAliases {
		if m[alias] {
			m[canon] = true
		}
		if m[canon] {
			m[alias] = true
		}
	}
}
