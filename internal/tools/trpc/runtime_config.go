package trpc

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	webresearchpkg "aranea-agents/internal/tools/webresearch"
)

// ApplyRuntimeConfigMaps patches ToolsetConfig from merged per-tool_key config objects.
func ApplyRuntimeConfigMaps(cfg *ToolsetConfig, byToolKey map[string]map[string]any) {
	if cfg == nil || len(byToolKey) == 0 {
		return
	}
	for key, m := range byToolKey {
		if len(m) == 0 {
			continue
		}
		applyConfigMap(cfg, key, m)
	}
}

func applyConfigMap(cfg *ToolsetConfig, toolKey string, m map[string]any) {
	switch {
	case isFilesystemToolKey(toolKey):
		if v := tools.ConfigString(m, "filesystem_dir", "base_dir", "working_dir", "root_dir"); v != "" {
			cfg.FilesystemDir = v
		}
	case toolKey == "google_search":
		if v := tools.ConfigString(m, "api_key", "google_api_key"); v != "" {
			cfg.GoogleAPIKey = v
		}
		if v := tools.ConfigString(m, "cx", "engine_id", "google_cx", "search_engine_id"); v != "" {
			cfg.GoogleCX = v
		}
	case toolKey == biz.ToolKeyWebResearch:
		cfg.WebResearchCfg = webresearchpkg.ConfigFromMap(m)
	case toolKey == "gemini_web_fetch":
		if v := tools.ConfigString(m, "model", "gemini_model"); v != "" {
			cfg.GeminiModel = v
		}
	case toolKey == "claude_code":
		if v := tools.ConfigString(m, "base_dir", "claude_code_dir", "working_dir"); v != "" {
			cfg.ClaudeCodeDir = v
		}
	case toolKey == "shell_exec":
		if v := tools.ConfigString(m, "base_dir", "shell_root", "filesystem_dir", "working_dir", "root_dir"); v != "" {
			cfg.ShellExecDir = v
		}
	}
}

func isFilesystemToolKey(key string) bool {
	switch key {
	case "read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content", "diff_edit", "patch_file":
		return true
	default:
		return false
	}
}

// ResolveGeminiFetchModel fills GeminiModel when gemini_web_fetch is enabled but the tool
// has no per-agent override; uses the agent's chat model when its provider is Gemini-like.
func ResolveGeminiFetchModel(cfg *ToolsetConfig, agentProvider, agentModel string) {
	if cfg == nil || !cfg.GeminiFetch || strings.TrimSpace(cfg.GeminiModel) != "" {
		return
	}
	model := strings.TrimSpace(agentModel)
	if model == "" || !isGeminiLikeProvider(agentProvider) {
		return
	}
	cfg.GeminiModel = model
}

func isGeminiLikeProvider(provider string) bool {
	p := strings.ToLower(strings.TrimSpace(provider))
	return strings.Contains(p, "gemini") || strings.Contains(p, "google")
}
