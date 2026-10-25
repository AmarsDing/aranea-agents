package data

import (
	"strings"

	"aranea-agents/internal/biz"
)

func evalEpisodeContentForQuery(content, query string) string {
	var out []string
	keep := true
	for _, line := range strings.Split(content, "\n") {
		for _, role := range []string{"user: ", "assistant: ", "system: ", "developer: ", "tool: "} {
			if strings.HasPrefix(line, role) {
				keep = biz.EvalEvidenceApplicable(line, query)
				break
			}
		}
		if keep {
			out = append(out, line)
		}
	}
	return strings.Join(out, "\n")
}

func filterEvalItemsByMinimum(items []evalCollectedItem, minimum float64) []evalCollectedItem {
	out := items[:0]
	for _, env := range items {
		if env.item.Score >= minimum {
			out = append(out, env)
		}
	}
	return out
}

func evalEvidenceClass(env evalCollectedItem) string {
	if env.evidenceUnit && biz.IsEvalStructuredEvidence(stripEvalRolePrefix(env.item.Content)) {
		return "structured"
	}
	if biz.EvalPersonalBackground(env.item.Content) {
		return "personal_background"
	}
	switch env.kind {
	case "constraint", "agent_instruction":
		return "instruction"
	case "preference", "user_preference":
		return "preference"
	case "user_identity":
		return "identity"
	}
	return "event"
}
