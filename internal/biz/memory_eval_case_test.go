package biz

import "testing"

// AMC CAMBench Coding (verified 2026-10-23): the task stream carries
// conventional-commit-shaped messages. They are durable event evidence and
// must receive their own non-governable kinds.
func TestEvalCaseFactKind(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"fix: login nil-pointer panic on empty token", EvalFactKindCaseFix},
		{"fix(auth): reject expired refresh tokens", EvalFactKindCaseFix},
		{"FIX: case-insensitive prefix", EvalFactKindCaseFix},
		{"fix(api/v2)!: drop legacy paging", EvalFactKindCaseFix},
		{"feat: add LDAP login", EvalFactKindCaseFeature},
		{"feat(api/v2): batch export endpoint", EvalFactKindCaseFeature},
		{"feat!: breaking config rename", EvalFactKindCaseFeature},
		{"  fix: leading whitespace tolerated", EvalFactKindCaseFix},
		{"fixture: prose, not a commit", ""},
		{"fixes: plural is prose", ""},
		{"hotfix: different word", ""},
		{"fix the sink is leaking", ""},
		{"fix : spaced colon is prose", ""},
		{"fix(unclosed scope: missing paren", ""},
		{"FIX(AUTH)!: breaking uppercase scope", EvalFactKindCaseFix},
		{"docs: not in the case kind set", ""},
		{"修复: 中文前缀不计入", ""},
		{"", ""},
	}
	for _, c := range cases {
		if got := EvalCaseFactKind(c.body); got != c.want {
			t.Errorf("EvalCaseFactKind(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

func TestEvalCaseScope(t *testing.T) {
	cases := []struct {
		body string
		want string
	}{
		{"fix(auth): reject expired tokens", "auth"},
		{"feat(api/v2): batch export", "api/v2"},
		{"fix(AUTH): scope is lower-cased", "auth"},
		{"fix: no scope", ""},
		{"prose only", ""},
	}
	for _, c := range cases {
		if got := EvalCaseScope(c.body); got != c.want {
			t.Errorf("EvalCaseScope(%q) = %q, want %q", c.body, got, c.want)
		}
	}
}

// Case kinds must win over text-cue heuristics (a commit is an event record,
// never an instruction or preference) but never override the system/developer
// protection gate.
func TestClassifyEvalFactKindCaseCommit(t *testing.T) {
	cases := []struct {
		statement string
		want      string
	}{
		{"user: fix: login nil-pointer panic on empty token", EvalFactKindCaseFix},
		{"assistant: feat(ui): dark mode toggle", EvalFactKindCaseFeature},
		{"user: feat: plan to migrate the database", EvalFactKindCaseFeature},
		{"system: fix: bump the memory window", "agent_instruction"},
		{"developer: feat: tighten sandbox", "agent_instruction"},
		{"user: 我喜欢咖啡", "preference"},
	}
	for _, c := range cases {
		if got := ClassifyEvalFactKind(c.statement); got != c.want {
			t.Errorf("ClassifyEvalFactKind(%q) = %q, want %q", c.statement, got, c.want)
		}
	}
}
