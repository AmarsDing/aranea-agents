// Package alias re-exports RuntimeToolNameAliases to break import cycles.
package alias

// RuntimeToolNameAliases maps legacy/UI/catalog names to mounted declaration names.
// Policy resolution uses biz.toolPolicyKeyAliases; this map applies at runtime so
// LLM calls using common aliases still resolve.
//
// IMPORTANT: keep aliases consistent with biz/tool/tool_policy_keys.go. When the same
// alias appears in both maps, both must point to the SAME canonical catalog tool_key
// (TPM-P1-01). `web_search` previously pointed at `duckduckgo_search` here but at
// `web_research` in biz, producing split-brain routing — now aligned to `web_research`.
var RuntimeToolNameAliases = map[string]string{
	"write_file":       "save_file",
	"edit_file":        "diff_edit",
	"list_files":       "list_file",
	"workspace_search": "search_content",
	"shell":            "shell_exec",
	"shell_exec":       "exec_command",
	"todo":             "todo_write",
	"gemini_fetch":     "gemini_web_fetch",
	"wikipedia":        "wikipedia_search",
	"email":            "send_email",
	"await_reply":      "await_user_reply",
	"web_search":       "web_research",
	"claude_code":      "claudecode",
}
