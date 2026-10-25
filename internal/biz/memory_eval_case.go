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

// EvalCaseStatementBody strips a recognized role prefix the same way
// ClassifyEvalFactKind does (via evalSpeaker). Callers that tag metadata for
// a stored statement must use the same body the classifier saw, otherwise an
// exotic role prefix ("human: fix: x") would be tagged while the fact kind
// stays non-case — a divergence between metadata and classification.
func EvalCaseStatementBody(statement string) string {
	_, body := evalSpeaker(statement)
	return body
}
