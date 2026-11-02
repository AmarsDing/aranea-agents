package biz

import (
	"regexp"
	"strings"
)

// AMC CAMBench Coding (verified against the official FAQ 2026-10-23): the
// task write stream carries conventional-commit-shaped messages. Those
// records are durable event evidence, not mutable preferences. They get
// dedicated, non-governable kinds so the supersession gates (which act only
// on identity / preference / constraint / instruction kinds) can never
// invalidate them: an earlier fix stays retrievable after a later, similar
// fix lands.
const (
	EvalFactKindCaseFix     = "case_fix"
	EvalFactKindCaseFeature = "case_feature"
)

// evalCaseCommitRe matches the conventional-commit subject shape: type,
// optional (scope), optional breaking-change !, colon. Prose lookalikes such
// as fixture: / fixes: / hotfix: do not match, because the colon must follow
// the type or scope immediately.
var evalCaseCommitRe = regexp.MustCompile(`^(?i)(fix|feat)(?:\(([a-z0-9_./-]{1,40})\))?(!)?:`)

// EvalCaseFactKind returns the durable case kind for a commit-shaped body
// (role prefix already stripped), or "" for prose.
func EvalCaseFactKind(body string) string {
	m := evalCaseCommitRe.FindStringSubmatch(strings.TrimSpace(body))
	if m == nil {
		return ""
	}
	if strings.EqualFold(m[1], "feat") {
		return EvalFactKindCaseFeature
	}
	return EvalFactKindCaseFix
}

// EvalCaseScope extracts the optional conventional-commit scope, lower-cased.
func EvalCaseScope(body string) string {
	m := evalCaseCommitRe.FindStringSubmatch(strings.TrimSpace(body))
	if m == nil {
		return ""
	}
	return strings.ToLower(strings.TrimSpace(m[2]))
}

// IsEvalCaseKind reports whether kind belongs to the code-board case family.
func IsEvalCaseKind(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case EvalFactKindCaseFix, EvalFactKindCaseFeature:
		return true
	default:
		return false
	}
}

var (
	evalQueryScopeWord = regexp.MustCompile(`(?i)\b(?:scope|module|package|repo)\s+([a-z0-9_./-]{1,40})\b`)
	evalQueryPathToken = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9_.-]*(?:/[a-z0-9_.-]+)+\b`)
	evalQueryFileToken = regexp.MustCompile(`(?i)\b[a-z0-9][a-z0-9_-]*\.[a-z0-9]{1,8}\b`)
	evalQueryCNModule  = regexp.MustCompile(`模块\s*([a-zA-Z0-9_./-]{1,40})`)
)

// EvalKeepsCodeUnit reports a commit record, a unified diff or an agent
// file-write/edit tool call. Those bodies stay one evidence atom: splitting
// on ". " or newlines drops the scope line and leaves the rest of the patch
// unclassified, and a JSON tool payload loses either the file path or the
// replacement half when it is cut at a sentence boundary.
func EvalKeepsCodeUnit(content string) bool {
	body := strings.TrimSpace(content)
	if body == "" {
		return false
	}
	if EvalCaseFactKind(body) != "" {
		return true
	}
	if strings.Contains(body, "[tool_use Write]") || strings.Contains(body, "[tool_use Edit]") ||
		strings.Contains(body, "[tool_use MultiEdit]") {
		return true
	}
	return strings.HasPrefix(body, "diff --git ") || strings.Contains(body, "\ndiff --git ")
}

// EvalQueryCaseScopes returns at most four scope/path tokens named by a code
// query. Equality against metadata.case_scope is the retrieval layer; ordinary
// words are not scopes.
func EvalQueryCaseScopes(query string) []string {
	var found []string
	add := func(s string) {
		s = strings.ToLower(strings.Trim(strings.TrimSpace(s), "./"))
		// A sub-3-letter "scope" is a stopword captured from prose
		// ("sub-package to provide" → "to"); it substring-matches nearly
		// every evidence body and drowns the real module scopes.
		if s == "" || len(s) < 3 || len(s) > 40 || len(found) >= 4 {
			return
		}
		for _, have := range found {
			if have == s {
				return
			}
		}
		found = append(found, s)
	}
	if scope := EvalCaseScope(query); scope != "" {
		add(scope)
	}
	for _, re := range []*regexp.Regexp{evalQueryScopeWord, evalQueryCNModule} {
		for _, m := range re.FindAllStringSubmatch(query, 4) {
			if len(m) > 1 {
				add(m[1])
			}
		}
	}
	for _, re := range []*regexp.Regexp{evalQueryPathToken, evalQueryFileToken} {
		for _, m := range re.FindAllStringSubmatch(query, 4) {
			if len(m) > 0 {
				add(m[0])
			}
		}
	}
	return found
}

// EvalCaseStatementBody strips a recognized role prefix the same way
// ClassifyEvalFactKind does (via evalSpeaker). Callers that tag metadata for
// a stored statement must use the same body the classifier saw, otherwise an
// exotic role prefix ("human: fix: x") would be tagged while the fact kind
// stays non-case — a divergence between metadata and classification.
func EvalCaseStatementBody(statement string) string {
	_, body := evalSpeaker(statement)
	return body
}
