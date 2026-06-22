package trpc

import "strings"

// PruneUnconfiguredToolFlags turns off tool switches that are enabled in policy but lack
// required runtime configuration, so Assemble does not fail the whole agent build.
// Returns catalog tool_key values that were skipped (for logging).
func PruneUnconfiguredToolFlags(cfg *ToolsetConfig) []string {
	if cfg == nil {
		return nil
	}
	var skipped []string
	if cfg.GeminiFetch && strings.TrimSpace(cfg.GeminiModel) == "" {
		cfg.GeminiFetch = false
		skipped = append(skipped, "gemini_web_fetch")
	}
	if cfg.GoogleSearch && (strings.TrimSpace(cfg.GoogleAPIKey) == "" || strings.TrimSpace(cfg.GoogleCX) == "") {
		cfg.GoogleSearch = false
		skipped = append(skipped, "google_search")
	}
	if cfg.WebResearch && !cfg.WebResearchCfg.Ready() {
		cfg.WebResearch = false
		skipped = append(skipped, "web_research")
	}
	if cfg.BrowserEnabled && cfg.Browser == nil {
		cfg.BrowserEnabled = false
		skipped = append(skipped, "browser")
	}
	if cfg.Message && cfg.OutboundRouter == nil {
		cfg.Message = false
		skipped = append(skipped, "message")
	}
	// workspace_exec factory returns nil,nil (not yet implemented). Force off to avoid
	// Assemble calling the factory and silently getting no tool back.
	if cfg.WorkspaceExec {
		cfg.WorkspaceExec = false
		skipped = append(skipped, "workspace_exec")
	}
	return skipped
}
