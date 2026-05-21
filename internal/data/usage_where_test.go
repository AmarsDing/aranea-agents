package data

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestUsageWhere_billableExcludesTeamTurn(t *testing.T) {
	where, _ := usageWhere(biz.UsageQuery{StartDate: "2026-05-01"}, true)
	if !strings.Contains(where, "team_turn") {
		t.Fatalf("expected team_turn exclusion in %q", where)
	}
}

func TestUsageWhere_detailIncludesTeamTurn(t *testing.T) {
	where, _ := usageWhere(biz.UsageQuery{UsageKind: biz.UsageKindTeamTurn}, false)
	if strings.Contains(where, "<> 'team_turn'") {
		t.Fatalf("detail query should not auto-exclude team_turn: %q", where)
	}
	if !strings.Contains(where, "usage_kind = ?") {
		t.Fatalf("expected usage_kind filter: %q", where)
	}
}

func TestUsageWhere_teamIDFilter(t *testing.T) {
	where, args := usageWhere(biz.UsageQuery{TeamID: "team-1"}, true)
	if !strings.Contains(where, "team_id = ?") {
		t.Fatalf("expected team_id filter: %q", where)
	}
	if len(args) == 0 || args[len(args)-1] != "team-1" {
		t.Fatalf("args: %v", args)
	}
}
