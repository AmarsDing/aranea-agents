package biz

import (
	"strings"
	"testing"
)

// ValidatePlanStepContracts is called by dagRun at startup (before dispatch)
// to advisory-check contract matching across board.Steps. PlanStep contracts
// are persisted at PublishV2Board time, so all contracts are known before
// any team is lazily assembled — unlike ValidateDeliverableContracts which
// reads the teams table.

func TestValidatePlanStepContracts_NoDependencies(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Deliverables: []DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
	}
	if got := ValidatePlanStepContracts(steps); len(got) != 0 {
		t.Fatalf("steps without dependencies should produce no warnings, got %v", got)
	}
}

func TestValidatePlanStepContracts_Match(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Deliverables: []DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
		{ID: "s2", DependsOn: []string{"s1"}, InputContract: []DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
	}
	if got := ValidatePlanStepContracts(steps); len(got) != 0 {
		t.Fatalf("matching contracts should produce no warnings, got %v", got)
	}
}

func TestValidatePlanStepContracts_MissingUpstreamDeliverable(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Deliverables: []DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
		{ID: "s2", DependsOn: []string{"s1"}, InputContract: []DeliverableContract{{Name: "data", Type: "data", Format: "json"}}},
	}
	got := ValidatePlanStepContracts(steps)
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got %v", got)
	}
	if !strings.Contains(got[0], `"data"`) || !strings.Contains(got[0], "no matching upstream deliverable") {
		t.Fatalf("warning should mention unmatched contract name, got %q", got[0])
	}
}

func TestValidatePlanStepContracts_TypeAndFormatMismatch(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Deliverables: []DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
		{ID: "s2", DependsOn: []string{"s1"}, InputContract: []DeliverableContract{{Name: "report", Type: "code", Format: "json"}}},
	}
	got := ValidatePlanStepContracts(steps)
	if len(got) != 2 {
		t.Fatalf("expected 2 warnings (type + format), got %v", got)
	}
	joined := strings.Join(got, "\n")
	if !strings.Contains(joined, "type mismatch") || !strings.Contains(joined, "format mismatch") {
		t.Fatalf("warnings should cover type and format mismatch, got %v", got)
	}
}

func TestValidatePlanStepContracts_UpstreamWithoutDeliverables(t *testing.T) {
	// Upstream step declared no deliverables (e.g. LLM omitted them and the
	// deterministic fallback did not fire) while downstream declares inputs.
	steps := []PlanStep{
		{ID: "s1"},
		{ID: "s2", DependsOn: []string{"s1"}, InputContract: []DeliverableContract{{Name: "report", Type: "document", Format: "markdown"}}},
	}
	got := ValidatePlanStepContracts(steps)
	if len(got) != 1 {
		t.Fatalf("expected 1 warning, got %v", got)
	}
	if !strings.Contains(got[0], "no matching upstream deliverable") {
		t.Fatalf("warning should report missing upstream deliverable, got %q", got[0])
	}
}

func TestValidatePlanStepContracts_AggregatesMultipleUpstreams(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1", Deliverables: []DeliverableContract{{Name: "spec", Type: "document", Format: "markdown"}}},
		{ID: "s2", Deliverables: []DeliverableContract{{Name: "data", Type: "data", Format: "json"}}},
		{ID: "s3", DependsOn: []string{"s1", "s2"}, InputContract: []DeliverableContract{
			{Name: "spec", Type: "document", Format: "markdown"},
			{Name: "data", Type: "data", Format: "json"},
		}},
	}
	if got := ValidatePlanStepContracts(steps); len(got) != 0 {
		t.Fatalf("contracts satisfied across multiple upstreams should produce no warnings, got %v", got)
	}
}

func TestValidatePlanStepContracts_SkipsStepsWithoutInputContract(t *testing.T) {
	steps := []PlanStep{
		{ID: "s1"},
		{ID: "s2", DependsOn: []string{"s1"}}, // no InputContract → nothing to validate
	}
	if got := ValidatePlanStepContracts(steps); len(got) != 0 {
		t.Fatalf("steps without input contracts should produce no warnings, got %v", got)
	}
}
