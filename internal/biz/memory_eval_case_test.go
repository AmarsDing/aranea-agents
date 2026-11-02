package biz

import (
	"testing"
	"time"
)

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
		{`assistant: [tool_use Write] {\"file_path\": \"/testbed/core.py\", \"content\": \"class TimeSeries: pass\"}`, "event"},
		{"user: 我喜欢咖啡", "preference"},
	}
	for _, c := range cases {
		if got := ClassifyEvalFactKind(c.statement); got != c.want {
			t.Errorf("ClassifyEvalFactKind(%q) = %q, want %q", c.statement, got, c.want)
		}
	}
}

func TestSplitEvalStatements_CommitAndDiffStayWhole(t *testing.T) {
	commit := "fix(auth): reject expired tokens. Also rotate the signing key."
	if got := SplitEvalStatements(commit, 8); len(got) != 1 || got[0] != commit {
		t.Fatalf("commit split = %#v", got)
	}
	diff := "diff --git a/auth/login.go b/auth/login.go\n--- a/auth/login.go\n+++ b/auth/login.go\n@@ -1 +1 @@\n-old\n+new"
	if got := SplitEvalStatements(diff, 8); len(got) != 1 || got[0] != diff {
		t.Fatalf("diff split into %d atoms", len(got))
	}
	prose := "I fixed the sink. It still leaks."
	if got := SplitEvalStatements(prose, 8); len(got) != 2 {
		t.Fatalf("prose atoms = %#v", got)
	}
}

func TestAnalyzeEvalQuery_CodeAndVisual(t *testing.T) {
	now := time.Date(2026, 10, 30, 0, 0, 0, 0, time.UTC)
	if qa := AnalyzeEvalQuery("What was the fix in module auth?", now); !qa.Code || qa.CodeSolve || qa.Visual {
		t.Fatalf("ordinary code query must retain generic retrieval behavior: %+v", qa)
	}
	if scopes := EvalQueryCaseScopes("What was the fix in module auth?"); len(scopes) != 1 || scopes[0] != "auth" {
		t.Fatalf("scopes = %#v", scopes)
	}
	if qa := AnalyzeEvalQuery("What is the prefix of this word?", now); qa.Code {
		t.Fatalf("prefix must not be a code query: %+v", qa)
	}
	if qa := AnalyzeEvalQuery("截图里有什么", now); !qa.Visual || qa.Code {
		t.Fatalf("visual query = %+v", qa)
	}
	if qa := AnalyzeEvalQuery("这张图里写了什么", now); !qa.Visual {
		t.Fatalf("deictic image query = %+v", qa)
	}
	if qa := AnalyzeEvalQuery("我住在地图附近", now); qa.Visual {
		t.Fatalf("map mention became visual: %+v", qa)
	}
	if qa := AnalyzeEvalQuery("What is my current address?", now); qa.Code || qa.Visual {
		t.Fatalf("current-address query picked up code/visual: %+v", qa)
	}
	smoke := "This PR introduces a new astropy.timeseries sub-package to provide core classes for representing time series data, based on QTable as a foundation. Investigate the issue above and make the necessary source changes to address it."
	if qa := AnalyzeEvalQuery(smoke, now); !qa.Code || !qa.CodeSolve {
		t.Fatalf("real CAMBench solve query was not classified as code solve: %+v", qa)
	}
	for _, query := range []string{
		"How should TimeSeries and BinnedTimeSeries be implemented using QTable?",
		"astropy timeseries tests and expected behavior",
		"astropy/timeseries/core.py implementation details",
	} {
		if qa := AnalyzeEvalQuery(query, now); !qa.Code || !qa.CodeSolve {
			t.Errorf("code solve follow-up lost trajectory mode %q: %+v", query, qa)
		}
	}
	if qa := AnalyzeEvalQuery("What expected behavior did the user describe?", now); qa.CodeSolve {
		t.Fatalf("ordinary behavior question became code solve: %+v", qa)
	}
	if qa := AnalyzeEvalQuery("TimeSeries invalid expected 'time' as the first columns required columns", now); !qa.Code || !qa.CodeError || qa.CodeSolve {
		t.Fatalf("real Smoke error query must use error-driven code evidence: %+v", qa)
	}
	if qa := AnalyzeEvalQuery("test_required_columns remove_column flux _required_columns = [time, flux]", now); !qa.Code || !qa.CodeError || qa.CodeSolve {
		t.Fatalf("underscore test/API symbols must use error-driven code evidence: %+v", qa)
	}
	if qa := AnalyzeEvalQuery("astropy 13033 TimeSeries invalid required columns pull request", now); !qa.Code || qa.CodeSolve {
		t.Fatalf("ordinary pull-request follow-up must not force an unrelated trajectory: %+v", qa)
	}
	for _, query := range []string{
		"required column flux is missing required column not found time series invalid message",
		"required columns missing exception expected behavior TimeSeries remove_column flux",
	} {
		if qa := AnalyzeEvalQuery(query, now); !qa.Code || !qa.CodeError || qa.CodeSolve {
			t.Errorf("real error follow-up must retain error-driven source retrieval %q: %+v", query, qa)
		}
	}
}
