package deferred

import (
	"sort"
)

// bizKeyToRegistryName 映射业务工具键到 Registry 名称。
// 多个业务键可以映射到同一个 Registry 名称（如 read_file/save_file → file）。
// 只包含可以被 deferred 的工具（Registry 中有对应条目且有 Factory 或 ToolSetFactory）。
var bizKeyToRegistryName = map[string]string{
	// filesystem group → file ToolSet
	"read_file":           "file",
	"read_multiple_files": "file",
	"save_file":           "file",
	"list_file":           "file",
	"search_file":         "file",
	"search_content":      "file",
	"replace_content":     "file",
	"diff_edit":           "file",
	"patch_file":          "file",
	"read_lints":          "read_lints",
	"delete_file":         "delete_file",

	// shell
	"shell_exec": "hostexec",

	// computer-use：装配为 CustomTools 扁平工具（Declaration 名 = catalog key），
	// 映射到自身才能被 FinalizeDeferredTools 按名包装进 ToolFilter。
	"computer_use_observe":    "computer_use_observe",
	"computer_use_screenshot": "computer_use_screenshot",
	"computer_use_act":        "computer_use_act",
	"computer_use_launch":     "computer_use_launch",
	"computer_use_session":    "computer_use_session",

	// spirit CustomTool：构图 schema ~1.7k tok，闲聊不常驻。
	"build_orchestration_graph": "build_orchestration_graph",

	// web
	"web_fetch":         "httpfetch",
	"gemini_web_fetch":  "geminifetch",
	"duckduckgo_search": "duckduckgo",
	"google_search":     "google_search",
	"arxiv_search":      "arxiv_search",
	"wikipedia_search":  "wikipedia",

	// communication
	"send_email": "email",
	"message":    "message",

	// productivity
	"todo_write": "todo",

	// interaction
	"await_user_reply": "await_user_reply",

	// coding
	"claude_code":    "claudecode",
	"workspace_exec": "workspace_exec",

	// browser
	"browser": "browser",

	// media
	"read_document":    "read_document",
	"read_spreadsheet": "read_spreadsheet",

	// memory
	"working_memory_read":     "working_memory",
	"working_memory_list":     "working_memory",
	"working_memory_write":    "working_memory",
	"working_memory_patch":    "working_memory",
	"working_memory_delete":   "working_memory",
	"working_memory_complete": "working_memory",

	// system
	"datetime":         "datetime",
	"read_tool_result": "read_tool_result",

	// composition
	"subagents_spawn":  "subagents_spawn",
	"subagents_list":   "subagents_list",
	"subagents_get":    "subagents_get",
	"subagents_cancel": "subagents_cancel",

	// client
	"client_open_app": "client",
	"client_open_url": "client",

	// team
	"set_deliverable": "deliverable",
	"get_deliverable": "deliverable",
}

// RegistryNamesForBizKeys 将业务工具键列表转换为 Registry 名称列表（去重、排序）。
// 只返回可以被 deferred 的 Registry 名称（即在 bizKeyToRegistryName 中有映射的）。
// 未映射的 CustomTools（如 memory_search、plan_and_execute）被跳过，保持常驻。
// computer_use_* / build_orchestration_graph 映射到自身（扁平 CustomTool）。
func RegistryNamesForBizKeys(bizKeys []string) []string {
	set := make(map[string]bool, len(bizKeys))
	for _, key := range bizKeys {
		if regName, ok := bizKeyToRegistryName[key]; ok {
			set[regName] = true
		}
	}
	names := make([]string, 0, len(set))
	for name := range set {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// BizKeysForRegistryName 返回指定 Registry 名称对应的所有业务工具键。
// 用于反向查询（如测试、调试）。
func BizKeysForRegistryName(registryName string) []string {
	var keys []string
	for bizKey, regName := range bizKeyToRegistryName {
		if regName == registryName {
			keys = append(keys, bizKey)
		}
	}
	sort.Strings(keys)
	return keys
}
