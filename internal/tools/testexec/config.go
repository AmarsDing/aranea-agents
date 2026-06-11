package testexec

import (
	"strings"

	"aranea-agents/internal/tools"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
	"aranea-agents/pkg/loggateway"
)

const (
	toolKeyKnowledgeSearch  = "knowledge_search"
	toolKeyKnowledgeReflect = "knowledge_reflect"
	toolKeyCallAgent        = "call_agent"
	toolKeyMCPToolSet       = "mcp_tool_set"
	toolKeyMCPBroker        = "mcp_broker"
)

// AssemblyForCatalogKey returns an AssemblyConfig for a single catalog tool_key.
func AssemblyForCatalogKey(key string, merged map[string]any, platform *webresearchpkg.PlatformFields, lg loggateway.Logger) (tools.AssemblyConfig, bool) {
	key = strings.TrimSpace(key)
	if key == "" {
		return tools.AssemblyConfig{}, false
	}
	switch key {
	case toolKeyKnowledgeSearch, toolKeyCallAgent, toolKeyMCPToolSet, toolKeyMCPBroker:
		return tools.AssemblyConfig{}, false
	case toolKeyKnowledgeReflect:
		return tools.AssemblyConfig{}, false
	case "read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content", "diff_edit", "patch_file":
		cfg := tools.AssemblyConfig{EnabledTools: []string{"file"}}
		applyFilesystemDir(&cfg, merged)
		return cfg, true
	case "shell_exec":
		cfg := tools.AssemblyConfig{EnabledTools: []string{"hostexec"}}
		applyShellExecDir(&cfg, merged)
		return cfg, true
	case "web_research":
		wcfg := webresearchpkg.ResolveConfig(merged, platform)
		if !wcfg.Ready() {
			return tools.AssemblyConfig{}, false
		}
		t, err := webresearchpkg.NewTool(wcfg, lg)
		if err != nil {
			return tools.AssemblyConfig{}, false
		}
		return tools.AssemblyConfig{Session: tools.SessionConfig{CustomTools: []tools.Tool{t}}}, true
	case "web_fetch":
		return tools.AssemblyConfig{EnabledTools: []string{"httpfetch"}}, true
	case "duckduckgo_search":
		return tools.AssemblyConfig{EnabledTools: []string{"duckduckgo"}}, true
	case "gemini_web_fetch":
		cfg := tools.AssemblyConfig{EnabledTools: []string{"geminifetch"}}
		if v := tools.ConfigString(merged, "model", "gemini_model"); v != "" {
			cfg.Search.GeminiModel = v
		}
		return cfg, true
	case "google_search":
		cfg := tools.AssemblyConfig{EnabledTools: []string{"google_search"}}
		if v := tools.ConfigString(merged, "api_key", "google_api_key"); v != "" {
			cfg.Search.GoogleAPIKey = v
		}
		if v := tools.ConfigString(merged, "cx", "engine_id", "google_cx", "search_engine_id"); v != "" {
			cfg.Search.GoogleCX = v
		}
		return cfg, true
	case "arxiv_search":
		return tools.AssemblyConfig{EnabledTools: []string{"arxiv_search"}}, true
	case "wikipedia_search":
		return tools.AssemblyConfig{EnabledTools: []string{"wikipedia"}}, true
	case "send_email":
		return tools.AssemblyConfig{EnabledTools: []string{"email"}}, true
	case "todo_write":
		return tools.AssemblyConfig{EnabledTools: []string{"todo"}}, true
	case "working_memory.read", "working_memory.list", "working_memory.write", "working_memory.patch", "working_memory.delete":
		return tools.AssemblyConfig{EnabledTools: []string{"working_memory"}}, true
	case "await_user_reply":
		return tools.AssemblyConfig{EnabledTools: []string{"await_user_reply"}}, true
	case "claude_code":
		cfg := tools.AssemblyConfig{EnabledTools: []string{"claudecode"}}
		if v := tools.ConfigString(merged, "base_dir", "claude_code_dir", "working_dir"); v != "" {
			cfg.ClaudeCode.Dir = v
		}
		return cfg, true
	case "workspace_exec":
		return tools.AssemblyConfig{}, false
	case "read_document":
		return tools.AssemblyConfig{EnabledTools: []string{"read_document"}}, true
	case "read_spreadsheet":
		return tools.AssemblyConfig{EnabledTools: []string{"read_spreadsheet"}}, true
	default:
		return tools.AssemblyConfig{}, false
	}
}

func applyFilesystemDir(cfg *tools.AssemblyConfig, m map[string]any) {
	if v := tools.ConfigString(m, "filesystem_dir", "base_dir", "working_dir", "root_dir"); v != "" {
		cfg.FilesystemDir = v
	}
}

func applyShellExecDir(cfg *tools.AssemblyConfig, m map[string]any) {
	if v := tools.ConfigString(m, "base_dir", "shell_root", "filesystem_dir", "working_dir", "root_dir"); v != "" {
		cfg.ShellExec.Dir = v
	}
}
