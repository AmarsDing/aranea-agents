package team

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestEnrichTeamRunMetricsFromSteps(t *testing.T) {
	run := biz.TeamRun{ID: "run-1"}
	steps := []biz.TeamRunStep{
		{TokenIn: 10, TokenOut: 20, OutputPreview: "first"},
		{TokenIn: 5, TokenOut: 8, OutputPreview: "final answer"},
	}
	enrichTeamRunMetricsFromSteps(&run, steps)
	if run.TokenIn != 15 || run.TokenOut != 28 {
		t.Fatalf("tokens: in=%d out=%d", run.TokenIn, run.TokenOut)
	}
	if run.OutputPreview == "" {
		t.Fatal("expected output preview from last step")
	}
}

func TestEnrichTeamRunMetricsFromSteps_preservesExistingPreview(t *testing.T) {
	run := biz.TeamRun{OutputPreview: "keep me"}
	enrichTeamRunMetricsFromSteps(&run, []biz.TeamRunStep{{OutputPreview: "new"}})
	if run.OutputPreview != "keep me" {
		t.Fatalf("preview=%q", run.OutputPreview)
	}
}
