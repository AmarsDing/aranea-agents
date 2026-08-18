package subagent

import "strings"

const (
	kindGeneral = "general"
	kindExplore = "explore"
	kindVerify  = "verify"
)

func normalizeKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case kindExplore, "search", "readonly":
		return kindExplore
	case kindVerify, "test", "qa":
		return kindVerify
	default:
		return kindGeneral
	}
}

func kindSystemPrompt(kind string) string {
	base := subagentRunPrompt
	switch normalizeKind(kind) {
	case kindExplore:
		return base + " Kind=explore: prefer search_content, search_file, read_file, and list_file. " +
			"Do not write files or run shell unless the task explicitly requires it. " +
			"Return findings with file paths and line numbers."
	case kindVerify:
		return base + " Kind=verify: run the requested tests or builds and report pass/fail with evidence " +
			"(command, exit, relevant output). Do not expand into unrelated refactors."
	default:
		return base
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
