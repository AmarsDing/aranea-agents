package data

import (
	"regexp"
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

// evalCodeGenericToken is a question word that appears in almost every code
// query. It must not by itself pull another module's commit or a prose
// lookalike (hotfix:, 修复:) into the evidence.
func evalCodeGenericToken(token string) bool {
	switch strings.ToLower(token) {
	case "fix", "feat", "fixes", "hotfix", "module", "package", "repo", "scope",
		"bug", "patch", "commit", "added", "change", "changed", "failed", "failure",
		"error", "code":
		return true
	default:
		return false
	}
}

func evalStripEvidencePrefix(content string) string {
	s := strings.TrimSpace(content)
	if strings.HasPrefix(s, "[") {
		if i := strings.Index(s, "] "); i > 0 && i <= 80 {
			if rest := strings.TrimSpace(s[i+2:]); rest != "" {
				return rest
			}
		}
	}
	return s
}

func evalCodeBody(content string) string {
	return biz.EvalCaseStatementBody(evalStripEvidencePrefix(content))
}

// Coding trajectories serialize tool arguments as JSON text inside the
// message, so older stored fragments contain \"file_path\" rather than a
// literal JSON quote. Normalize only that transport escaping for structural
// detection; the returned evidence itself remains byte-for-byte unchanged.
func evalCodePayloadText(content string) string {
	return strings.ReplaceAll(content, `\"`, `"`)
}

// evalCodeTokenHit reports whether content shares a non-generic query token.
// distinctive is false when the query is only words like "fix" and "module".
func evalCodeTokenHit(content, query string) (hit, distinctive bool) {
	low := strings.ToLower(content)
	for _, tok := range tokenizeQuery(query) {
		if evalCodeGenericToken(tok) {
			continue
		}
		distinctive = true
		if strings.Contains(low, strings.ToLower(tok)) {
			hit = true
		}
	}
	return hit, distinctive
}

func evalCodeUnitRelevant(content, caseScope, query string) bool {
	body := evalCodeBody(content)
	if caseScope == "" {
		caseScope = biz.EvalCaseScope(body)
	}
	low := strings.ToLower(body)
	for _, scope := range biz.EvalQueryCaseScopes(query) {
		if scope == "" {
			continue
		}
		if scope == caseScope || strings.Contains(low, scope) {
			return true
		}
	}
	hit, distinctive := evalCodeTokenHit(body, query)
	return hit || !distinctive
}

var evalNumberedCodeLine = regexp.MustCompile(`(?m)^[ \t]*[0-9]+(?:\t| +)[ \t]*(?:def\s+\w+\(|class\s+\w+|(?:if|for|while)\s+.+[:{]|(?:return|raise|throw)\b|(?:func|function)\b|[A-Za-z_][A-Za-z0-9_.]*\s*[:=])`)

func evalIsCodeUnit(content, kind string) bool {
	if biz.IsEvalCaseKind(kind) {
		return true
	}
	body := evalCodeBody(content)
	if biz.EvalKeepsCodeUnit(body) || biz.EvalCaseFactKind(body) != "" || strings.Contains(body, "```") {
		return true
	}
	// A fragmented tool-call payload atom lost its "[tool_use ...]" head but
	// still carries the escaped JSON patch; it must keep code-unit budgets so
	// the evidence-unit join can reassemble the complete call.
	payload := evalCodePayloadText(body)
	if strings.Contains(payload, `"file_path"`) &&
		(strings.Contains(payload, `"old_string"`) || strings.Contains(payload, `"new_string"`) || strings.Contains(payload, `"content"`)) {
		return true
	}
	// Tool output commonly contains numbered source listings rather than a
	// diff. Require multiple source-shaped lines so numbered prose stays prose.
	return len(evalNumberedCodeLine.FindAllStringIndex(body, 2)) == 2
}

// evalCodeQueryKeeps drops case evidence from another module and prose that
// only matched a generic word such as "fix" inside "hotfix".
func evalCodeQueryKeeps(env evalCollectedItem, query string) bool {
	if evalIsCodeUnit(env.item.Content, env.kind) {
		return evalCodeUnitRelevant(env.item.Content, env.caseScope, query)
	}
	hit, _ := evalCodeTokenHit(env.item.Content, query)
	return hit
}

func evalEpisodeRoleLine(line string) bool {
	for _, role := range []string{"user: ", "assistant: ", "system: ", "developer: ", "tool: "} {
		if strings.HasPrefix(line, role) {
			return true
		}
	}
	return false
}

// evalEpisodeCodeExcerpt keeps commit and diff blocks that answer a code
// query. A session transcript must not carry unrelated prose into the hit.
func evalEpisodeCodeExcerpt(content, query string) string {
	var out []string
	var block []string
	flush := func() {
		if len(block) == 0 {
			return
		}
		text := strings.Join(block, "\n")
		env := evalCollectedItem{item: biz.EvalMemoryItem{Content: text}, kind: biz.ClassifyEvalFactKind(text)}
		if evalCodeQueryKeeps(env, query) {
			out = append(out, text)
		}
		block = nil
	}
	for _, line := range strings.Split(content, "\n") {
		if evalEpisodeRoleLine(line) {
			flush()
		}
		block = append(block, line)
	}
	flush()
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
