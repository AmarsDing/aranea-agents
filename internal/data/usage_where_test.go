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

// 79-runtime-governance 1.5: session drill-down for per-session cache-hit ratio.
func TestUsageWhere_sessionIDFilter(t *testing.T) {
	where, args := usageWhere(biz.UsageQuery{SessionID: "sess-1", UsageKind: biz.UsageKindChatTurn}, false)
	if !strings.Contains(where, "session_id = ?") {
		t.Fatalf("expected session_id filter: %q", where)
	}
	if len(args) != 2 || args[0] != biz.UsageKindChatTurn || args[1] != "sess-1" {
		t.Fatalf("args order: %v", args)
	}
	// Empty SessionID must not constrain the query.
	whereAll, _ := usageWhere(biz.UsageQuery{}, false)
	if strings.Contains(whereAll, "session_id") {
		t.Fatalf("empty session id must not filter: %q", whereAll)
	}
}
