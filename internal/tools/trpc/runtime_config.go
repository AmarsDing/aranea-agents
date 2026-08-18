package trpc

import (
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/tools"
	"aranea-agents/internal/tools/browser"
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
	case toolKey == "browser":
		if parsed := parsePlaywrightMCPConfig(m); parsed != nil {
			cfg.Browser = parsed
		}
	}
}

// parsePlaywrightMCPConfig converts a browser tool config map into a PlaywrightMCPConfig.
// Returns nil when the map lacks a usable command or server URL.
func parsePlaywrightMCPConfig(m map[string]any) *browser.PlaywrightMCPConfig {
	if len(m) == 0 {
		return nil
	}
	cfg := browser.DefaultPlaywrightMCPConfig()
	hasCustom := false
	if v := tools.ConfigString(m, "command"); v != "" {
		cfg.Command = v
		hasCustom = true
	}
	if args := configStringSlice(m, "args"); len(args) > 0 {
		cfg.Args = args
		hasCustom = true
	}
	if v := tools.ConfigString(m, "transport"); v != "" {
		cfg.Transport = v
		hasCustom = true
	}
	if v := tools.ConfigString(m, "server_url", "url"); v != "" {
		cfg.ServerURL = v
		hasCustom = true
	}
	if v, ok := configBool(m, "headless"); ok {
		cfg.Headless = browser.BoolPtr(v)
		hasCustom = true
	}
	if v, ok := configBool(m, "vision"); ok {
		cfg.Vision = browser.BoolPtr(v)
		hasCustom = true
	}
	if v, ok := configBool(m, "isolated"); ok {
		cfg.Isolated = browser.BoolPtr(v)
		hasCustom = true
	}
	if v, ok := configInt(m, "timeout_sec", "timeout"); ok {
		cfg.TimeoutSec = v
		hasCustom = true
	}
	if !hasCustom {
		return nil
	}
	return &cfg
}

func configStringSlice(m map[string]any, key string) []string {
	v, ok := m[key]
	if !ok {
		return nil
	}
	switch val := v.(type) {
	case []string:
		return val
	case []any:
		out := make([]string, 0, len(val))
		for _, item := range val {
			if s, ok := item.(string); ok && strings.TrimSpace(s) != "" {
				out = append(out, strings.TrimSpace(s))
			}
		}
		return out
	}
	return nil
}

func configBool(m map[string]any, keys ...string) (bool, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			if b, ok := v.(bool); ok {
				return b, true
			}
			if s, ok := v.(string); ok {
				switch strings.ToLower(strings.TrimSpace(s)) {
				case "true", "1", "yes":
					return true, true
				case "false", "0", "no":
					return false, true
				}
			}
		}
	}
	return false, false
}

func configInt(m map[string]any, keys ...string) (int, bool) {
	for _, k := range keys {
		if v, ok := m[k]; ok {
			switch n := v.(type) {
			case int:
				return n, true
			case int64:
				return int(n), true
			case float64:
				return int(n), true
			}
		}
	}
	return 0, false
}

func isFilesystemToolKey(key string) bool {
	switch key {
	case "read_file", "read_multiple_files", "save_file", "list_file", "search_file", "search_content", "replace_content", "diff_edit", "patch_file", "read_lints", "delete_file":
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
	return p == "gemini" || p == "google" || strings.HasPrefix(p, "gemini-") || strings.HasPrefix(p, "google-")
}
