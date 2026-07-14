package data

import (
	"strings"
	"testing"

	"aranea-agents/internal/biz"
)

func TestBreakdownQueryWhere_EmptyExceptDates(t *testing.T) {
	q := biz.UsageBreakdownQuery{StartDate: "2025-01-01", EndDate: "2025-01-31"}
	where, args := breakdownQueryWhere(q)
	if !strings.Contains(where, "date_key >= ?") {
		t.Errorf("where missing date_key >= ?: %q", where)
	}
	if !strings.Contains(where, "date_key <= ?") {
		t.Errorf("where missing date_key <= ?: %q", where)
	}
	if !strings.Contains(where, sqlUsageBillableKind) {
		t.Errorf("where missing billable-kind filter: %q", where)
	}
	if len(args) != 2 {
		t.Errorf("args len = %d, want 2", len(args))
	}
}

func TestBreakdownQueryWhere_WithProviderAndSearch(t *testing.T) {
	q := biz.UsageBreakdownQuery{
		StartDate:    "2025-01-01",
		EndDate:      "2025-01-31",
		ProviderCode: "openai",
		Search:       "gpt",
	}
	where, args := breakdownQueryWhere(q)
	if !strings.Contains(where, "provider_code = ?") {
		t.Errorf("where missing provider_code clause: %q", where)
	}
	if !strings.Contains(where, "(provider_code LIKE ? OR model_api_id LIKE ?)") {
		t.Errorf("where missing search LIKE clause: %q", where)
	}
	// 2 dates + 1 provider + 2 LIKE args (provider_code + model_api_id)
	if len(args) != 5 {
		t.Errorf("args len = %d, want 5", len(args))
	}
	if !strings.Contains(args[3].(string), "%gpt%") {
		t.Errorf("first search arg not wrapped in wildcards: %v", args[3])
	}
	if !strings.Contains(args[4].(string), "%gpt%") {
		t.Errorf("second search arg not wrapped in wildcards: %v", args[4])
	}
}

func TestBreakdownQueryWhere_SearchOnly(t *testing.T) {
	q := biz.UsageBreakdownQuery{
		StartDate: "2025-01-01",
		EndDate:   "2025-01-31",
		Search:    "claude",
	}
	where, args := breakdownQueryWhere(q)
	if !strings.Contains(where, "(provider_code LIKE ? OR model_api_id LIKE ?)") {
		t.Errorf("where missing OR search clause: %q", where)
	}
	// 2 dates + 2 LIKE args
	if len(args) != 4 {
		t.Errorf("args len = %d, want 4", len(args))
	}
}

func TestBreakdownSortClause_Whitelist(t *testing.T) {
	tests := []struct {
		name      string
		field     string
		dir       string
		wantField string
		wantDir   string
	}{
		{"call_count_desc", "call_count", "desc", "SUM(call_count)", "DESC"},
		{"total_tokens_asc", "total_tokens", "asc", "SUM(total_tokens)", "ASC"},
		{"total_cost_micro_usd_desc", "total_cost_micro_usd", "desc", "SUM(total_cost_micro_usd)", "DESC"},
		{"success_rate_desc", "success_rate", "desc", "success_rate", "DESC"},
		{"avg_latency_ms_asc", "avg_latency_ms", "asc", "avg_latency_ms", "ASC"},
		{"unknown_field_defaults_to_call_count", "evil; DROP TABLE", "desc", "SUM(call_count)", "DESC"},
		{"invalid_dir_defaults_to_desc", "call_count", "sideways", "SUM(call_count)", "DESC"},
		{"empty_field_defaults_to_call_count", "", "desc", "SUM(call_count)", "DESC"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := breakdownSortClause(tt.field, tt.dir)
			if !strings.Contains(got, tt.wantField) {
				t.Errorf("sort clause %q missing field %q", got, tt.wantField)
			}
			if !strings.Contains(got, tt.wantDir) {
				t.Errorf("sort clause %q missing dir %q", got, tt.wantDir)
			}
			// 防 SQL 注入：未知字段不应原样出现在 SQL 中
			if strings.Contains(got, "DROP TABLE") {
				t.Errorf("sort clause %q contains injection attempt", got)
			}
		})
	}
}
