package data

import (
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/modelregistry"
)

func TestUsageHourlyWhere_EmptyQuery(t *testing.T) {
	where, args := usageHourlyWhere(biz.UsageQuery{})
	if !strings.Contains(where, "WHERE") {
		t.Fatalf("expected WHERE clause, got %q", where)
	}
	if !strings.Contains(where, sqlUsageBillableKind) {
		t.Fatalf("expected billable kind filter, got %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("expected 0 args, got %d", len(args))
	}
}

func TestUsageHourlyWhere_WithAgentID(t *testing.T) {
	where, args := usageHourlyWhere(biz.UsageQuery{AgentID: "agent-1"})
	if !strings.Contains(where, "agent_id = ?") {
		t.Fatalf("expected agent_id filter, got %q", where)
	}
	if len(args) != 1 || args[0] != "agent-1" {
		t.Fatalf("expected 1 arg 'agent-1', got %v", args)
	}
}

func TestUsageHourlyWhere_WithDateRange(t *testing.T) {
	where, args := usageHourlyWhere(biz.UsageQuery{StartDate: "2025-01-01", EndDate: "2025-01-31"})
	if !strings.Contains(where, "hour_key >= ?") {
		t.Fatalf("expected hour_key >= filter, got %q", where)
	}
	if !strings.Contains(where, "hour_key <= ?") {
		t.Fatalf("expected hour_key <= filter, got %q", where)
	}
	if len(args) != 2 {
		t.Fatalf("expected 2 args, got %d", len(args))
	}
	if args[1] != "2025-01-31T23" {
		t.Fatalf("expected end date with T23 suffix, got %v", args[1])
	}
}

func TestUsageDailyWhere_EmptyQuery(t *testing.T) {
	where, args := usageDailyWhere(biz.UsageQuery{})
	if !strings.Contains(where, "WHERE") {
		t.Fatalf("expected WHERE clause, got %q", where)
	}
	if !strings.Contains(where, sqlUsageBillableKind) {
		t.Fatalf("expected billable kind filter, got %q", where)
	}
	if len(args) != 0 {
		t.Fatalf("expected 0 args, got %d", len(args))
	}
}

func TestUsageDailyWhere_WithProvider(t *testing.T) {
	where, args := usageDailyWhere(biz.UsageQuery{ProviderCode: "openai"})
	if !strings.Contains(where, "provider") {
		t.Fatalf("expected provider filter, got %q", where)
	}
	if len(args) < 1 {
		t.Fatalf("expected at least 1 arg, got %d", len(args))
	}
}

func TestUsageDailyWhere_WithTeamID(t *testing.T) {
	where, args := usageDailyWhere(biz.UsageQuery{TeamID: "team-1"})
	// Daily 表无 team_id 列，不应添加任何 team 相关过滤
	if strings.Contains(where, "workspace_id = ?") || strings.Contains(where, "team_id = ?") {
		t.Fatalf("daily query should not filter by team: %q", where)
	}
	for _, a := range args {
		if a == "team-1" {
			t.Fatalf("daily query should not contain team-1 in args: %v", args)
		}
	}
}

func TestUsageLimit_Zero(t *testing.T) {
	if got := usageLimit(0); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestUsageLimit_Negative(t *testing.T) {
	if got := usageLimit(-5); got != 10 {
		t.Fatalf("expected 10, got %d", got)
	}
}

func TestUsageLimit_OverMax(t *testing.T) {
	if got := usageLimit(300); got != 200 {
		t.Fatalf("expected 200, got %d", got)
	}
}

func TestUsageLimit_Normal(t *testing.T) {
	if got := usageLimit(50); got != 50 {
		t.Fatalf("expected 50, got %d", got)
	}
}

func TestMicroPricingToBizRule_Fields(t *testing.T) {
	micro := modelregistry.MicroPricing{
		Input: 30000, Output: 60000, CacheRead: 1500,
		CacheWrite: 7500, Reasoning: 12000, Embedding: 100,
	}
	rule := microPricingToBizRule("openai", "gpt-4", micro, "catalog")
	if rule.ProviderCode != "openai" {
		t.Fatalf("expected provider openai, got %q", rule.ProviderCode)
	}
	if rule.ModelAPIID != "gpt-4" {
		t.Fatalf("expected model gpt-4, got %q", rule.ModelAPIID)
	}
	if rule.Currency != "USD" {
		t.Fatalf("expected currency USD, got %q", rule.Currency)
	}
	if rule.InputPriceMicroUSDPer1K != 30000 {
		t.Fatalf("expected input micro 30000, got %d", rule.InputPriceMicroUSDPer1K)
	}
	if rule.OutputPriceMicroUSDPer1K != 60000 {
		t.Fatalf("expected output micro 60000, got %d", rule.OutputPriceMicroUSDPer1K)
	}
	if rule.Source != "catalog" {
		t.Fatalf("expected source catalog, got %q", rule.Source)
	}
	if rule.MetadataJSON != "{}" {
		t.Fatalf("expected metadata {}, got %q", rule.MetadataJSON)
	}
}

func TestRuleKey_Format(t *testing.T) {
	rule := modelregistry.ProviderMigrationRule{Legacy: "old", Catalog: "new"}
	got := ruleKey(rule)
	want := "old->new"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestContainsStr_Found(t *testing.T) {
	if !containsStr([]string{"a", "b", "c"}, "b") {
		t.Fatal("expected true")
	}
}

func TestContainsStr_NotFound(t *testing.T) {
	if containsStr([]string{"a", "b", "c"}, "d") {
		t.Fatal("expected false")
	}
}

func TestContainsStr_Empty(t *testing.T) {
	if containsStr([]string{}, "a") {
		t.Fatal("expected false for empty slice")
	}
}

func TestClampTimelinePageLimit_Zero(t *testing.T) {
	got := clampTimelinePageLimit(0, 0)
	if got != biz.MessageListMaxLimit {
		t.Fatalf("expected %d, got %d", biz.MessageListMaxLimit, got)
	}
}

func TestClampTimelinePageLimit_ZeroWithTotal(t *testing.T) {
	got := clampTimelinePageLimit(0, 42)
	if got != 42 {
		t.Fatalf("expected 42, got %d", got)
	}
}

func TestClampTimelinePageLimit_OverMax(t *testing.T) {
	got := clampTimelinePageLimit(biz.MessageListMaxLimit+100, 0)
	if got != biz.MessageListMaxLimit {
		t.Fatalf("expected %d, got %d", biz.MessageListMaxLimit, got)
	}
}

func TestClampTimelinePageLimit_Normal(t *testing.T) {
	got := clampTimelinePageLimit(25, 100)
	if got != 25 {
		t.Fatalf("expected 25, got %d", got)
	}
}

func TestMsToTime_Zero(t *testing.T) {
	got := msToTime(0)
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %v", got)
	}
}

func TestMsToTime_Negative(t *testing.T) {
	got := msToTime(-1)
	if !got.IsZero() {
		t.Fatalf("expected zero time for negative, got %v", got)
	}
}

func TestMsToTime_Positive(t *testing.T) {
	ms := int64(1700000000000)
	got := msToTime(ms)
	if got.IsZero() {
		t.Fatal("expected non-zero time")
	}
	expected := time.UnixMilli(ms).UTC()
	if !got.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func TestParseRFC3339_Empty(t *testing.T) {
	got := parseRFC3339("")
	if !got.IsZero() {
		t.Fatalf("expected zero time, got %v", got)
	}
}

func TestParseRFC3339_Valid(t *testing.T) {
	s := "2025-06-15T12:30:00Z"
	got := parseRFC3339(s)
	if got.IsZero() {
		t.Fatal("expected non-zero time")
	}
	expected, _ := time.Parse(time.RFC3339, s)
	if !got.Equal(expected) {
		t.Fatalf("expected %v, got %v", expected, got)
	}
}

func TestParseRFC3339_Invalid(t *testing.T) {
	got := parseRFC3339("not-a-date")
	if !got.IsZero() {
		t.Fatalf("expected zero time for invalid input, got %v", got)
	}
}

func TestDedupeStrings_Empty(t *testing.T) {
	got := dedupeStrings(nil)
	if got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestDedupeStrings_AllUnique(t *testing.T) {
	in := []string{"a", "b", "c"}
	got := dedupeStrings(in)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d", len(got))
	}
	for i, v := range got {
		if v != in[i] {
			t.Fatalf("at index %d: expected %q, got %q", i, in[i], v)
		}
	}
}

func TestDedupeStrings_Duplicates(t *testing.T) {
	in := []string{"a", "b", "a", "c", "b"}
	got := dedupeStrings(in)
	if len(got) != 3 {
		t.Fatalf("expected 3, got %d: %v", len(got), got)
	}
	if got[0] != "a" || got[1] != "b" || got[2] != "c" {
		t.Fatalf("expected [a b c], got %v", got)
	}
}

func TestDedupeStrings_EmptyStrings(t *testing.T) {
	in := []string{"", "a", "", "b", ""}
	got := dedupeStrings(in)
	if len(got) != 2 {
		t.Fatalf("expected 2, got %d: %v", len(got), got)
	}
	if got[0] != "a" || got[1] != "b" {
		t.Fatalf("expected [a b], got %v", got)
	}
}

func TestPlaceholders_Zero(t *testing.T) {
	if got := placeholders(0); got != "" {
		t.Fatalf("expected empty string, got %q", got)
	}
}

func TestPlaceholders_One(t *testing.T) {
	if got := placeholders(1); got != "?" {
		t.Fatalf("expected '?', got %q", got)
	}
}

func TestPlaceholders_Three(t *testing.T) {
	got := placeholders(3)
	want := "?,?,?"
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

// Phase 1c-3: tests for isSQLIdentByte/matchSQLWord/sqlWordBoundaryBefore/
// sqlWordBoundaryAfter removed — these helpers were deleted with the
// messages-table FTS5 search code (message_search.go).
