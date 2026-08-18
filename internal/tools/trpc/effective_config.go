package trpc

import (
	"aranea-agents/internal/biz"
	computerusepkg "aranea-agents/internal/tools/computeruse"
)

// ToolsetConfigFromEffectiveKeys maps effective tool_key flags to ToolsetConfig switches.
// Single source for agent runtime assembly; keep in sync with Registry / seed tool keys.
func ToolsetConfigFromEffectiveKeys(eff map[string]bool) ToolsetConfig {
	if len(eff) == 0 {
		return ToolsetConfig{}
	}
	has := func(key string) bool { return eff[key] }
	cfg := ToolsetConfig{
		Filesystem: has("read_file") || has("read_multiple_files") || has("save_file") ||
			has("list_file") || has("search_file") || has("search_content") || has("replace_content") || has("diff_edit") || has("patch_file"),
		ReadLints:        has("read_lints"),
		DeleteFile:       has("delete_file"),
		ShellExec:        has("shell_exec"),
		WebFetch:         has("web_fetch"),
		WebSearch:        has("duckduckgo_search"),
		WebResearch:      has(biz.ToolKeyWebResearch),
		GeminiFetch:      has("gemini_web_fetch"),
		GoogleSearch:     has("google_search"),
		ArxivSearch:      has("arxiv_search"),
		Wikipedia:        has("wikipedia_search"),
		Email:            has("send_email"),
		Todo:             has("todo_write"),
		AwaitReply:       has("await_user_reply"),
		ClaudeCode:       has("claude_code"),
		WorkspaceExec:    has("workspace_exec"),
		KnowledgeSearch:  has(biz.ToolKeyKnowledgeSearch),
		KnowledgeReflect: has(biz.ToolKeyKnowledgeReflect),
		CallAgent:        has(biz.ToolKeyCallAgent),
		Kanban:           has(biz.ToolKeyKanban),
		ReadDocument:     has("read_document"),
		ReadSpreadsheet:  has("read_spreadsheet"),
		WorkingMemory:    has("working_memory_read") || has("working_memory_list") || has("working_memory_write") || has("working_memory_patch") || has("working_memory_delete") || has("working_memory_complete"),
		Datetime:         has("datetime"),
		Message:          has("message"),
		SubAgent:         has("subagents_spawn") || has("subagents_list") || has("subagents_get") || has("subagents_cancel"),
		BrowserEnabled:   has("browser"),
		ClientBridge:     has("client_open_app") || has("client_open_url"),
		ComputerUse: has(computerusepkg.ToolObserve) || has(computerusepkg.ToolScreenshot) ||
			has(computerusepkg.ToolAct) || has(computerusepkg.ToolLaunch) || has(computerusepkg.ToolSession),
		CodingBridge: has("coding_dispatch_task") || has("coding_check_task") || has("coding_cancel_task"),
	}
	return cfg
}

// ToolsetConfigHasAny reports whether any tool switch or MCP attachment is enabled.
func ToolsetConfigHasAny(cfg ToolsetConfig) bool {
	return cfg.Filesystem || cfg.ShellExec || cfg.WebFetch || cfg.WebSearch || cfg.WebResearch ||
		cfg.GeminiFetch || cfg.GoogleSearch || cfg.ArxivSearch || cfg.Wikipedia ||
		cfg.Email || cfg.Todo || cfg.AwaitReply || cfg.ClaudeCode || cfg.WorkspaceExec ||
		cfg.KnowledgeSearch || cfg.KnowledgeReflect || cfg.CallAgent || cfg.Kanban || cfg.MemoryEnabled ||
		cfg.ReadDocument || cfg.ReadSpreadsheet || cfg.ReadLints || cfg.DeleteFile || cfg.WorkingMemory || cfg.Datetime || cfg.Message || cfg.BrowserEnabled || cfg.SubAgent ||
		cfg.ClientBridge || cfg.ComputerUse || cfg.CodingBridge ||
		len(cfg.AgentTools) > 0 || len(cfg.MCPServers) > 0 || cfg.MCPBroker != nil || len(cfg.CustomTools) > 0
}
