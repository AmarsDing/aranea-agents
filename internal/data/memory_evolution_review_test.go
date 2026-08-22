package data

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestEvolutionReviewTarget(t *testing.T) {
	lg := loggateway.NewNoop()
	if got := evolutionReviewTarget("proposal_approved", `{"proposal_id":"p9"}`, "fallback", lg); got != "p9" {
		t.Fatalf("approve target: got %q", got)
	}
	if got := evolutionReviewTarget("event_reverted", `{"event_id":"e3"}`, "fallback", lg); got != "e3" {
		t.Fatalf("revert target: got %q", got)
	}
	if got := evolutionReviewTarget("proposal_rejected", `{}`, "prop-1", lg); got != "prop-1" {
		t.Fatalf("fallback target: got %q", got)
	}
}
