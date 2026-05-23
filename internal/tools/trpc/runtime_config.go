package trpc

import (
	"strings"

	"aranea-agents/internal/biz"
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
		if v := configString(m, "filesystem_dir", "base_dir", "working_dir", "root_dir"); v != "" {
			cfg.FilesystemDir = v
		}
	case toolKey == "google_search":
		if v := configString(m, "api_key", "google_api_key"); v != "" {
			cfg.GoogleAPIKey = v
		}
		if v := configString(m, "cx", "engine_id", "google_cx", "search_engine_id"); v != "" {
			cfg.GoogleCX = v
		}
	case toolKey == biz.ToolKeyWebResearch:
		cfg.WebResearchCfg = webresearchpkg.ConfigFromMap(m)
	case toolKey == "gemini_web_fetch":
		if v := configString(m, "model", "gemini_model"); v != "" {
			cfg.GeminiModel = v
		}
	case toolKey == "claude_code":
		if v := configString(m, "base_dir", "claude_code_dir", "working_dir"); v != "" {
			cfg.ClaudeCodeDir = v
		}
	case toolKey == "shell_exec":
		if v := configString(m, "base_dir", "shell_root", "filesystem_dir", "working_dir", "root_dir"); v != "" {
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

func configString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if s, ok := v.(string); ok && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s)
			}
		}
	}
	return ""
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
