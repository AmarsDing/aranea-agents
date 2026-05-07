package registry

// ApplyEffectiveAliases synchronizes synonymous keys so ADK binding matches model/UI habits.
// Policy/API normalization lives in aranea-agents/internal/biz (toolPolicyKeyAliases); when adding an
// alias, update both places so runtime and effective-tools views stay aligned.
func ApplyEffectiveAliases(enabled map[string]bool) {
	if enabled == nil {
		return
	}
	if enabled[ShellExec] {
		enabled[ShellAlias] = true
	}
	if enabled[ShellAlias] {
		enabled[ShellExec] = true
	}
	// Catalog / UI use web_search; older rows may say google_search (Gemini-era name).
	if enabled[WebSearch] || enabled[GoogleSearch] {
		enabled[WebSearch] = true
		enabled[GoogleSearch] = true
	}
}
