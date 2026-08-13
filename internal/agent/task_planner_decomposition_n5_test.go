package agent

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

// TestBuildDecompositionPrompt_IncludesSuccessCriteria covers N5 (2026-08-13
// 链路审查): the intent artifact's SuccessCriteria and SearchHints were
// produced by the intent pass but dropped from the decomposition prompt, so
// subtask contracts could drift from the success bar. They must be included
// when present and omitted when empty.
func TestBuildDecompositionPrompt_IncludesSuccessCriteria(t *testing.T) {
	artifact := &biz.IntentArtifact{
		RefinedGoal:     "build a CSV importer",
		IntentKind:      "code_change",
		RiskFlags:       []string{"touches-prod"},
		SuccessCriteria: []string{"imports 10k rows without loss", "duplicates rejected"},
		SearchHints:     []string{"existing csv parsers", "schema mapping"},
	}
	prompt := buildDecompositionPrompt("import users from csv", artifact, 0)
	for _, want := range []string{
		"imports 10k rows without loss",
		"duplicates rejected",
		"existing csv parsers",
		"schema mapping",
	} {
		if !strings.Contains(prompt, want) {
			t.Errorf("decomposition prompt missing %q", want)
		}
	}
}

// TestBuildDecompositionPrompt_OmitsEmptyCriteria covers the negative path:
// no SuccessCriteria/SearchHints in the artifact → no empty sections in the
// prompt (byte-stability for agents without intent artifacts).
func TestBuildDecompositionPrompt_OmitsEmptyCriteria(t *testing.T) {
	withEmpty := buildDecompositionPrompt("do something", &biz.IntentArtifact{
		RefinedGoal: "do something",
		IntentKind:  "task",
	}, 0)
	if strings.Contains(withEmpty, "Success criteria") || strings.Contains(withEmpty, "Search hints") {
		t.Error("empty criteria/hints must not add sections to the prompt")
	}
	if nilArtifact := buildDecompositionPrompt("do something", nil, 0); strings.Contains(nilArtifact, "Intent analysis") {
		t.Error("nil artifact must not add intent context")
	}
}
