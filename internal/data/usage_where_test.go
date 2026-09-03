package data

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestUsageWhere_billableExcludesTeamTurn(t *testing.T) {
	where, _ := usageWhere(DialectSQLite, biz.UsageQuery{StartDate: "2026-05-01"}, true)
	if !strings.Contains(where, "team_turn") {
		t.Fatalf("expected team_turn exclusion in %q", where)
	}
}

func TestUsageWhere_detailIncludesTeamTurn(t *testing.T) {
	where, _ := usageWhere(DialectSQLite, biz.UsageQuery{UsageKind: biz.UsageKindTeamTurn}, false)
	if strings.Contains(where, "<> 'team_turn'") {
		t.Fatalf("detail query should not auto-exclude team_turn: %q", where)
	}
	if !strings.Contains(where, "usage_kind = ?") {
		t.Fatalf("expected usage_kind filter: %q", where)
	}
}

func TestUsageWhere_teamIDFilter(t *testing.T) {
	where, args := usageWhere(DialectSQLite, biz.UsageQuery{TeamID: "team-1"}, true)
	if !strings.Contains(where, "team_id = ?") {
		t.Fatalf("expected team_id filter: %q", where)
	}
	if len(args) == 0 || args[len(args)-1] != "team-1" {
		t.Fatalf("args: %v", args)
	}
}

// 79-runtime-governance 1.5: session drill-down for per-session cache-hit ratio.
func TestUsageWhere_sessionIDFilter(t *testing.T) {
	where, args := usageWhere(DialectSQLite, biz.UsageQuery{SessionID: "sess-1", UsageKind: biz.UsageKindChatTurn}, false)
	if !strings.Contains(where, "session_id = ?") {
		t.Fatalf("expected session_id filter: %q", where)
	}
	if len(args) != 2 || args[0] != biz.UsageKindChatTurn || args[1] != "sess-1" {
		t.Fatalf("args order: %v", args)
	}
	// Empty SessionID must not constrain the query.
	whereAll, _ := usageWhere(DialectSQLite, biz.UsageQuery{}, false)
	if strings.Contains(whereAll, "session_id") {
		t.Fatalf("empty session id must not filter: %q", whereAll)
	}
}

// LBG-6: effort filter extracts metadata_json["effort"] via the dialect-aware
// JSONExtract so the same clause runs on SQLite and Postgres.
func TestUsageWhere_effortFilter(t *testing.T) {
	where, args := usageWhere(DialectSQLite, biz.UsageQuery{Effort: "high"}, false)
	if !strings.Contains(where, "json_extract(metadata_json, '$.effort') = ?") {
		t.Fatalf("expected sqlite effort extraction: %q", where)
	}
	if len(args) != 1 || args[0] != "high" {
		t.Fatalf("args: %v", args)
	}

	wherePG, argsPG := usageWhere(DialectPostgres, biz.UsageQuery{Effort: "low", UsageKind: biz.UsageKindChatTurn}, false)
	if !strings.Contains(wherePG, `->> 'effort'`) {
		t.Fatalf("expected postgres jsonb extraction: %q", wherePG)
	}
	if len(argsPG) != 2 || argsPG[0] != biz.UsageKindChatTurn || argsPG[1] != "low" {
		t.Fatalf("args order: %v", argsPG)
	}

	// Empty Effort must not constrain the query.
	whereAll, _ := usageWhere(DialectSQLite, biz.UsageQuery{}, false)
	if strings.Contains(whereAll, "effort") {
		t.Fatalf("empty effort must not filter: %q", whereAll)
	}
}
