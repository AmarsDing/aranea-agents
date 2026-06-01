package usage

import (
	"math"
	"strings"
	"testing"
	"time"
)

func TestNormalizeTokenUsageEventForInsert(t *testing.T) {
	fixedTime := time.Date(2026, 5, 30, 14, 25, 0, 0, time.UTC)

	t.Run("fills default OccurredAt when empty", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-1"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.OccurredAt != fixedTime.Format(time.RFC3339) {
			t.Errorf("OccurredAt = %q, want %q", got.OccurredAt, fixedTime.Format(time.RFC3339))
		}
	})

	t.Run("preserves existing OccurredAt", func(t *testing.T) {
		ts := "2026-01-15T08:30:00Z"
		e := TokenUsageEvent{ID: "test-2", OccurredAt: ts}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.OccurredAt != ts {
			t.Errorf("OccurredAt = %q, want %q", got.OccurredAt, ts)
		}
	})

	t.Run("parses OccurredAt and overrides now for DateKey/HourKey", func(t *testing.T) {
		ts := "2026-03-10T06:45:00Z"
		e := TokenUsageEvent{ID: "test-3", OccurredAt: ts}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.DateKey != "2026-03-10" {
			t.Errorf("DateKey = %q, want %q", got.DateKey, "2026-03-10")
		}
		if got.HourKey != "2026-03-10T06:00" {
			t.Errorf("HourKey = %q, want %q", got.HourKey, "2026-03-10T06:00")
		}
	})

	t.Run("extracts DateKey from OccurredAt", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-4", OccurredAt: "2026-05-30T14:25:00Z"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.DateKey != "2026-05-30" {
			t.Errorf("DateKey = %q, want %q", got.DateKey, "2026-05-30")
		}
	})

	t.Run("extracts HourKey from OccurredAt", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-5", OccurredAt: "2026-05-30T14:25:00Z"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.HourKey != "2026-05-30T14:00" {
			t.Errorf("HourKey = %q, want %q", got.HourKey, "2026-05-30T14:00")
		}
	})

	t.Run("does not overwrite existing DateKey", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-6", OccurredAt: "2026-05-30T14:25:00Z", DateKey: "2026-01-01"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.DateKey != "2026-01-01" {
			t.Errorf("DateKey = %q, want %q", got.DateKey, "2026-01-01")
		}
	})

	t.Run("does not overwrite existing HourKey", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-7", OccurredAt: "2026-05-30T14:25:00Z", HourKey: "2026-05-30T15:00"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.HourKey != "2026-05-30T15:00" {
			t.Errorf("HourKey = %q, want %q", got.HourKey, "2026-05-30T15:00")
		}
	})

	t.Run("normalizes Status to success", func(t *testing.T) {
		for _, s := range []string{"", "success", "ok", "OK", "Success"} {
			e := TokenUsageEvent{ID: "test-8", Status: s}
			got := NormalizeTokenUsageEventForInsert(e, fixedTime)
			if got.Status != "success" {
				t.Errorf("Status %q => %q, want %q", s, got.Status, "success")
			}
		}
	})

	t.Run("normalizes Status to failed", func(t *testing.T) {
		for _, s := range []string{"failed", "fail", "error", "Error", "FAIL"} {
			e := TokenUsageEvent{ID: "test-9", Status: s}
			got := NormalizeTokenUsageEventForInsert(e, fixedTime)
			if got.Status != "failed" {
				t.Errorf("Status %q => %q, want %q", s, got.Status, "failed")
			}
		}
	})

	t.Run("normalizes Status to timeout", func(t *testing.T) {
		for _, s := range []string{"timeout", "timed_out", "TIMEOUT"} {
			e := TokenUsageEvent{ID: "test-10", Status: s}
			got := NormalizeTokenUsageEventForInsert(e, fixedTime)
			if got.Status != "timeout" {
				t.Errorf("Status %q => %q, want %q", s, got.Status, "timeout")
			}
		}
	})

	t.Run("normalizes Status to cancelled", func(t *testing.T) {
		for _, s := range []string{"cancelled", "canceled", "CANCELLED"} {
			e := TokenUsageEvent{ID: "test-11", Status: s}
			got := NormalizeTokenUsageEventForInsert(e, fixedTime)
			if got.Status != "cancelled" {
				t.Errorf("Status %q => %q, want %q", s, got.Status, "cancelled")
			}
		}
	})

	t.Run("defaults UsageKind to chat", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-12"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.UsageKind != "chat" {
			t.Errorf("UsageKind = %q, want %q", got.UsageKind, "chat")
		}
	})

	t.Run("preserves existing UsageKind", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-13", UsageKind: "team_member"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.UsageKind != "team_member" {
			t.Errorf("UsageKind = %q, want %q", got.UsageKind, "team_member")
		}
	})

	t.Run("defaults CallCount to 1 when zero or negative", func(t *testing.T) {
		for _, cc := range []int{0, -1} {
			e := TokenUsageEvent{ID: "test-14", CallCount: cc}
			got := NormalizeTokenUsageEventForInsert(e, fixedTime)
			if got.CallCount != 1 {
				t.Errorf("CallCount %d => %d, want 1", cc, got.CallCount)
			}
		}
	})

	t.Run("preserves positive CallCount", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-15", CallCount: 5}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.CallCount != 5 {
			t.Errorf("CallCount = %d, want 5", got.CallCount)
		}
	})

	t.Run("defaults ModelCategoryJSON to empty array", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-16"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.ModelCategoryJSON != "[]" {
			t.Errorf("ModelCategoryJSON = %q, want %q", got.ModelCategoryJSON, "[]")
		}
	})

	t.Run("defaults MetadataJSON to empty object", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-17"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.MetadataJSON != "{}" {
			t.Errorf("MetadataJSON = %q, want %q", got.MetadataJSON, "{}")
		}
	})

	t.Run("defaults CreatedAt to OccurredAt when empty", func(t *testing.T) {
		ts := "2026-05-30T14:25:00Z"
		e := TokenUsageEvent{ID: "test-18", OccurredAt: ts}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.CreatedAt != ts {
			t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, ts)
		}
	})

	t.Run("preserves existing CreatedAt", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-19", OccurredAt: "2026-05-30T14:25:00Z", CreatedAt: "2026-05-30T15:00:00Z"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.CreatedAt != "2026-05-30T15:00:00Z" {
			t.Errorf("CreatedAt = %q, want %q", got.CreatedAt, "2026-05-30T15:00:00Z")
		}
	})

	t.Run("no DateKey when OccurredAt too short", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-20", OccurredAt: "2026"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.DateKey != "" {
			t.Errorf("DateKey = %q, want empty", got.DateKey)
		}
	})

	t.Run("no HourKey when OccurredAt too short", func(t *testing.T) {
		e := TokenUsageEvent{ID: "test-21", OccurredAt: "2026-05-30"}
		got := NormalizeTokenUsageEventForInsert(e, fixedTime)
		if got.HourKey != "" {
			t.Errorf("HourKey = %q, want empty", got.HourKey)
		}
	})
}

func TestCsvEscape(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{"normal string", "hello world", "hello world"},
		{"string with commas", "a,b,c", "a,b,c"},
		{"string with quotes", `say "hello"`, `say ""hello""`},
		{"string with newlines", "line1\nline2", "line1 line2"},
		{"empty string", "", ""},
		{"multiple newlines", "a\nb\nc", "a b c"},
		{"quotes and newlines", "\"hello\"\nworld", "\"\"hello\"\" world"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CsvEscape(tt.input)
			if got != tt.want {
				t.Errorf("CsvEscape(%q) = %q, want %q", tt.input, got, tt.want)
			}
		})
	}
}

func TestFormatUsageEventsCSV(t *testing.T) {
	t.Run("empty slice", func(t *testing.T) {
		got := FormatUsageEventsCSV(nil)
		if !strings.HasPrefix(got, "occurred_at,") {
			t.Errorf("missing header, got: %q", got)
		}
		lines := strings.Count(got, "\n")
		if lines != 1 {
			t.Errorf("expected 1 line (header only), got %d", lines)
		}
	})

	t.Run("single event", func(t *testing.T) {
		events := []TokenUsageEvent{
			{
				OccurredAt:       "2026-05-30T14:25:00Z",
				UsageKind:        "chat",
				AgentID:          "agent-1",
				ProviderCode:     "openai",
				ModelAPIID:       "gpt-4o",
				SessionID:        "sess-1",
				TeamID:           "team-1",
				InputTokens:      100,
				OutputTokens:     200,
				TotalTokens:      300,
				TotalCostMicroUSD: 500,
				LatencyMS:        1200,
				Status:           "success",
				ErrorMessage:     "",
			},
		}
		got := FormatUsageEventsCSV(events)
		lines := strings.Split(strings.TrimSpace(got), "\n")
		if len(lines) != 2 {
			t.Fatalf("expected 2 lines (header + 1 data), got %d", len(lines))
		}
		if !strings.Contains(lines[1], "100,200,300,500,1200") {
			t.Errorf("data line missing token/cost values: %q", lines[1])
		}
	})

	t.Run("multiple events", func(t *testing.T) {
		events := []TokenUsageEvent{
			{OccurredAt: "2026-05-30T10:00:00Z", UsageKind: "chat", AgentID: "a1", ProviderCode: "openai", ModelAPIID: "gpt-4o", SessionID: "s1", TeamID: "t1", InputTokens: 10, OutputTokens: 20, TotalTokens: 30, TotalCostMicroUSD: 100, LatencyMS: 500, Status: "success", ErrorMessage: ""},
			{OccurredAt: "2026-05-30T11:00:00Z", UsageKind: "team_member", AgentID: "a2", ProviderCode: "anthropic", ModelAPIID: "claude-3", SessionID: "s2", TeamID: "t2", InputTokens: 50, OutputTokens: 60, TotalTokens: 110, TotalCostMicroUSD: 200, LatencyMS: 800, Status: "failed", ErrorMessage: "timeout"},
		}
		got := FormatUsageEventsCSV(events)
		lines := strings.Split(strings.TrimSpace(got), "\n")
		if len(lines) != 3 {
			t.Fatalf("expected 3 lines (header + 2 data), got %d", len(lines))
		}
	})

	t.Run("special characters in error message", func(t *testing.T) {
		events := []TokenUsageEvent{
			{
				OccurredAt:   "2026-05-30T14:25:00Z",
				UsageKind:    "chat",
				AgentID:      "a1",
				ProviderCode: "openai",
				ModelAPIID:   "gpt-4o",
				SessionID:    "s1",
				TeamID:       "t1",
				InputTokens:  100,
				OutputTokens: 200,
				TotalTokens:  300,
				TotalCostMicroUSD: 500,
				LatencyMS:    1200,
				Status:       "failed",
				ErrorMessage: "error: \"bad\"\nrequest",
			},
		}
		got := FormatUsageEventsCSV(events)
		if !strings.Contains(got, "bad") {
			t.Errorf("error content missing in CSV output: %q", got)
		}
		if strings.Contains(got, "\\nrequest") {
			t.Errorf("newline should have been replaced by csvEscape: %q", got)
		}
	})
}

func TestUsageCostMicro(t *testing.T) {
	t.Run("USDPer1M calculation", func(t *testing.T) {
		tokens := 1000
		usdPer1M := 3.0
		got := UsageCostMicro(tokens, 0, usdPer1M)
		want := int64(math.Round(float64(tokens) * usdPer1M))
		if got != want {
			t.Errorf("UsageCostMicro(%d, 0, %f) = %d, want %d", tokens, usdPer1M, got, want)
		}
	})

	t.Run("USDPer1M takes precedence over microPer1K", func(t *testing.T) {
		tokens := 1000
		usdPer1M := 3.0
		microPer1K := int64(5000)
		got := UsageCostMicro(tokens, microPer1K, usdPer1M)
		want := int64(math.Round(float64(tokens) * usdPer1M))
		if got != want {
			t.Errorf("UsageCostMicro(%d, %d, %f) = %d, want %d (USDPer1M should take precedence)", tokens, microPer1K, usdPer1M, got, want)
		}
	})

	t.Run("microPer1K fallback", func(t *testing.T) {
		tokens := 2000
		microPer1K := int64(3000)
		got := UsageCostMicro(tokens, microPer1K, 0)
		want := int64(tokens) * microPer1K / 1000
		if got != want {
			t.Errorf("UsageCostMicro(%d, %d, 0) = %d, want %d", tokens, microPer1K, got, want)
		}
	})

	t.Run("zero tokens returns zero", func(t *testing.T) {
		got := UsageCostMicro(0, 3000, 3.0)
		if got != 0 {
			t.Errorf("UsageCostMicro(0, 3000, 3.0) = %d, want 0", got)
		}
	})

	t.Run("negative tokens returns zero", func(t *testing.T) {
		got := UsageCostMicro(-100, 3000, 3.0)
		if got != 0 {
			t.Errorf("UsageCostMicro(-100, 3000, 3.0) = %d, want 0", got)
		}
	})

	t.Run("zero price returns zero", func(t *testing.T) {
		got := UsageCostMicro(1000, 0, 0)
		if got != 0 {
			t.Errorf("UsageCostMicro(1000, 0, 0) = %d, want 0", got)
		}
	})

	t.Run("NaN usdPer1M falls back to microPer1K", func(t *testing.T) {
		tokens := 1000
		microPer1K := int64(3000)
		got := UsageCostMicro(tokens, microPer1K, math.NaN())
		want := int64(tokens) * microPer1K / 1000
		if got != want {
			t.Errorf("UsageCostMicro(%d, %d, NaN) = %d, want %d", tokens, microPer1K, got, want)
		}
	})

	t.Run("large token count with USDPer1M", func(t *testing.T) {
		tokens := 2_000_000
		usdPer1M := 3.0
		got := UsageCostMicro(tokens, 0, usdPer1M)
		want := int64(math.Round(float64(tokens) * usdPer1M))
		if got != want {
			t.Errorf("UsageCostMicro(%d, 0, %f) = %d, want %d", tokens, usdPer1M, got, want)
		}
	})
}
