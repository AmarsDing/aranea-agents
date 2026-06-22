package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz"
)

func TestFromProtoUsageQuery(t *testing.T) {
	t.Run("nil_input", func(t *testing.T) {
		got := fromProtoUsageQuery(nil)
		if got != (biz.UsageQuery{}) {
			t.Fatalf("expected zero value, got %+v", got)
		}
	})

	t.Run("full_mapping", func(t *testing.T) {
		in := &v1.UsageQuery{
			Range:        "7d",
			StartDate:    "2026-01-01",
			EndDate:      "2026-01-07",
			ProviderCode: "openai",
			ModelApiId:   "gpt-4o",
			AgentId:      "agent-1",
			TeamId:       "team-1",
			UsageKind:    "chat_turn",
			Status:       "success",
			Limit:        100,
			Granularity:  "daily",
		}
		got := fromProtoUsageQuery(in)
		if got.Range != "7d" {
			t.Errorf("Range = %q", got.Range)
		}
		if got.StartDate != "2026-01-01" {
			t.Errorf("StartDate = %q", got.StartDate)
		}
		if got.EndDate != "2026-01-07" {
			t.Errorf("EndDate = %q", got.EndDate)
		}
		if got.ProviderCode != "openai" {
			t.Errorf("ProviderCode = %q", got.ProviderCode)
		}
		if got.ModelAPIID != "gpt-4o" {
			t.Errorf("ModelAPIID = %q", got.ModelAPIID)
		}
		if got.AgentID != "agent-1" {
			t.Errorf("AgentID = %q", got.AgentID)
		}
		if got.TeamID != "team-1" {
			t.Errorf("TeamID = %q", got.TeamID)
		}
		if got.UsageKind != "chat_turn" {
			t.Errorf("UsageKind = %q", got.UsageKind)
		}
		if got.Status != "success" {
			t.Errorf("Status = %q", got.Status)
		}
		if got.Limit != 100 {
			t.Errorf("Limit = %d", got.Limit)
		}
		if got.Granularity != "daily" {
			t.Errorf("Granularity = %q", got.Granularity)
		}
	})
}

func TestFromProtoTokenUsageEvent(t *testing.T) {
	t.Run("nil_input", func(t *testing.T) {
		got := fromProtoTokenUsageEvent(nil)
		if got != (biz.TokenUsageEvent{}) {
			t.Fatalf("expected zero value, got %+v", got)
		}
	})

	t.Run("full_mapping", func(t *testing.T) {
		in := &v1.TokenUsageEvent{
			Id:                 "evt-1",
			OccurredAt:         "2026-01-01T00:00:00Z",
			ProviderCode:       "openai",
			ModelApiId:         "gpt-4o",
			CallCount:          5,
			InputTokens:        1000,
			OutputTokens:       500,
			TotalTokens:        1500,
			InputCostMicroUsd:  100,
			OutputCostMicroUsd: 50,
			TotalCostMicroUsd:  150,
			Status:             "success",
			LatencyMs:          2000,
			TimeToFirstTokenMs: 300,
			TokensPerSecond:    25.5,
		}
		got := fromProtoTokenUsageEvent(in)
		if got.ID != "evt-1" {
			t.Errorf("ID = %q", got.ID)
		}
		if got.OccurredAt != "2026-01-01T00:00:00Z" {
			t.Errorf("OccurredAt = %q", got.OccurredAt)
		}
		if got.ProviderCode != "openai" {
			t.Errorf("ProviderCode = %q", got.ProviderCode)
		}
		if got.ModelAPIID != "gpt-4o" {
			t.Errorf("ModelAPIID = %q", got.ModelAPIID)
		}
		if got.CallCount != 5 {
			t.Errorf("CallCount = %d", got.CallCount)
		}
		if got.InputTokens != 1000 {
			t.Errorf("InputTokens = %d", got.InputTokens)
		}
		if got.OutputTokens != 500 {
			t.Errorf("OutputTokens = %d", got.OutputTokens)
		}
		if got.TotalTokens != 1500 {
			t.Errorf("TotalTokens = %d", got.TotalTokens)
		}
		if got.InputCostMicroUSD != 100 {
			t.Errorf("InputCostMicroUSD = %d", got.InputCostMicroUSD)
		}
		if got.OutputCostMicroUSD != 50 {
			t.Errorf("OutputCostMicroUSD = %d", got.OutputCostMicroUSD)
		}
		if got.TotalCostMicroUSD != 150 {
			t.Errorf("TotalCostMicroUSD = %d", got.TotalCostMicroUSD)
		}
		if got.Status != "success" {
			t.Errorf("Status = %q", got.Status)
		}
		if got.LatencyMS != 2000 {
			t.Errorf("LatencyMS = %d", got.LatencyMS)
		}
		if got.TimeToFirstTokenMS != 300 {
			t.Errorf("TimeToFirstTokenMS = %d", got.TimeToFirstTokenMS)
		}
		if got.TokensPerSecond != 25.5 {
			t.Errorf("TokensPerSecond = %v", got.TokensPerSecond)
		}
	})
}

func TestToProtoUsageQuota(t *testing.T) {
	in := biz.UsageQuota{
		ID:              "q-1",
		ScopeType:       "agent",
		ScopeID:         "agent-1",
		MonthlyMicroUSD: 1000000,
		PeriodStart:     "2026-01-01",
		PeriodEnd:       "2026-01-31",
		CreatedAt:       "2026-01-01T00:00:00Z",
		UpdatedAt:       "2026-01-15T00:00:00Z",
	}
	got := toProtoUsageQuota(in)
	if got.Id != "q-1" {
		t.Errorf("Id = %q", got.Id)
	}
	if got.ScopeType != "agent" {
		t.Errorf("ScopeType = %q", got.ScopeType)
	}
	if got.ScopeId != "agent-1" {
		t.Errorf("ScopeId = %q", got.ScopeId)
	}
	if got.MonthlyMicroUsd != 1000000 {
		t.Errorf("MonthlyMicroUsd = %d", got.MonthlyMicroUsd)
	}
	if got.PeriodStart != "2026-01-01" {
		t.Errorf("PeriodStart = %q", got.PeriodStart)
	}
	if got.PeriodEnd != "2026-01-31" {
		t.Errorf("PeriodEnd = %q", got.PeriodEnd)
	}
}

func TestToProtoBudgetAlert(t *testing.T) {
	in := biz.BudgetAlert{
		ID:          "ba-1",
		ScopeType:   "agent",
		ScopeID:     "agent-1",
		AlertRatio:  0.8,
		Enabled:     true,
		LastFiredAt: "2026-01-10T00:00:00Z",
		CreatedAt:   "2026-01-01T00:00:00Z",
		UpdatedAt:   "2026-01-10T00:00:00Z",
	}
	got := toProtoBudgetAlert(in)
	if got.Id != "ba-1" {
		t.Errorf("Id = %q", got.Id)
	}
	if got.AlertRatio != 0.8 {
		t.Errorf("AlertRatio = %v", got.AlertRatio)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
}

func TestToProtoUsageSummary(t *testing.T) {
	in := biz.UsageSummary{
		CallCount:          100,
		RequestCount:       90,
		SuccessCount:       85,
		FailedCount:        5,
		CancelledCount:     0,
		InputTokens:        50000,
		OutputTokens:       25000,
		TotalTokens:        75000,
		TotalCostMicroUSD:  5000,
		AvgLatencyMS:       1500.5,
		AvgTokensPerSecond: 30.2,
		SuccessRate:        0.944,
	}
	got := toProtoUsageSummary(in)
	if got.CallCount != 100 {
		t.Errorf("CallCount = %d", got.CallCount)
	}
	if got.SuccessCount != 85 {
		t.Errorf("SuccessCount = %d", got.SuccessCount)
	}
	if got.FailedCount != 5 {
		t.Errorf("FailedCount = %d", got.FailedCount)
	}
	if got.InputTokens != 50000 {
		t.Errorf("InputTokens = %d", got.InputTokens)
	}
	if got.TotalCostMicroUsd != 5000 {
		t.Errorf("TotalCostMicroUsd = %d", got.TotalCostMicroUsd)
	}
	if got.AvgLatencyMs != 1500.5 {
		t.Errorf("AvgLatencyMs = %v", got.AvgLatencyMs)
	}
	if got.SuccessRate != 0.944 {
		t.Errorf("SuccessRate = %v", got.SuccessRate)
	}
}

func TestToProtoUsageTrendPoint(t *testing.T) {
	in := biz.UsageTrendPoint{
		DateKey:            "2026-01-01",
		CallCount:          50,
		InputTokens:        20000,
		OutputTokens:       10000,
		TotalTokens:        30000,
		TotalCostMicroUSD:  2000,
		SuccessCount:       48,
		FailedCount:        2,
		CancelledCount:     0,
		AvgLatencyMS:       1200.0,
		AvgTokensPerSecond: 25.0,
	}
	got := toProtoUsageTrendPoint(in)
	if got.DateKey != "2026-01-01" {
		t.Errorf("DateKey = %q", got.DateKey)
	}
	if got.CallCount != 50 {
		t.Errorf("CallCount = %d", got.CallCount)
	}
	if got.TotalCostMicroUsd != 2000 {
		t.Errorf("TotalCostMicroUsd = %d", got.TotalCostMicroUsd)
	}
}

func TestToProtoUsageBreakdownRow(t *testing.T) {
	in := biz.UsageBreakdownRow{
		ProviderCode:       "openai",
		ModelAPIID:         "gpt-4o",
		ModelDisplayName:   "GPT-4o",
		AgentID:            "agent-1",
		AgentKey:           "helper",
		CallCount:          200,
		InputTokens:        80000,
		OutputTokens:       40000,
		TotalTokens:        120000,
		TotalCostMicroUSD:  10000,
		AvgLatencyMS:       1800.0,
		AvgTokensPerSecond: 22.5,
		SuccessRate:        0.95,
	}
	got := toProtoUsageBreakdownRow(in)
	if got.ProviderCode != "openai" {
		t.Errorf("ProviderCode = %q", got.ProviderCode)
	}
	if got.ModelApiId != "gpt-4o" {
		t.Errorf("ModelApiId = %q", got.ModelApiId)
	}
	if got.AgentId != "agent-1" {
		t.Errorf("AgentId = %q", got.AgentId)
	}
	if got.CallCount != 200 {
		t.Errorf("CallCount = %d", got.CallCount)
	}
	if got.SuccessRate != 0.95 {
		t.Errorf("SuccessRate = %v", got.SuccessRate)
	}
}

func TestToProtoTokenUsageEvent(t *testing.T) {
	in := biz.TokenUsageEvent{
		ID:                 "evt-1",
		OccurredAt:         "2026-01-01T00:00:00Z",
		ProviderCode:       "openai",
		ModelAPIID:         "gpt-4o",
		CallCount:          3,
		InputTokens:        500,
		OutputTokens:       250,
		TotalTokens:        750,
		InputCostMicroUSD:  50,
		OutputCostMicroUSD: 25,
		TotalCostMicroUSD:  75,
		Status:             "success",
		LatencyMS:          1500,
		TokensPerSecond:    20.0,
	}
	got := toProtoTokenUsageEvent(in)
	if got.Id != "evt-1" {
		t.Errorf("Id = %q", got.Id)
	}
	if got.CallCount != 3 {
		t.Errorf("CallCount = %d", got.CallCount)
	}
	if got.InputTokens != 500 {
		t.Errorf("InputTokens = %d", got.InputTokens)
	}
	if got.LatencyMs != 1500 {
		t.Errorf("LatencyMs = %d", got.LatencyMs)
	}
}

func TestToProtoQuotaDashboard(t *testing.T) {
	in := biz.QuotaDashboard{
		ConfiguredCount:    5,
		TotalCapMicroUSD:   5000000,
		TotalSpentMicroUSD: 2000000,
		MaxUtilization:     0.85,
	}
	got := toProtoQuotaDashboard(in)
	if got.ConfiguredCount != 5 {
		t.Errorf("ConfiguredCount = %d", got.ConfiguredCount)
	}
	if got.TotalCapMicroUsd != 5000000 {
		t.Errorf("TotalCapMicroUsd = %d", got.TotalCapMicroUsd)
	}
	if got.TotalSpentMicroUsd != 2000000 {
		t.Errorf("TotalSpentMicroUsd = %d", got.TotalSpentMicroUsd)
	}
	if got.MaxUtilizationRatio != 0.85 {
		t.Errorf("MaxUtilizationRatio = %v", got.MaxUtilizationRatio)
	}
}

func TestToProtoUsageModelInsights(t *testing.T) {
	t.Run("empty", func(t *testing.T) {
		got := toProtoUsageModelInsights(nil)
		if len(got) != 0 {
			t.Fatalf("expected empty, got %d", len(got))
		}
	})

	t.Run("multiple", func(t *testing.T) {
		items := []biz.UsageModelInsight{
			{
				ProviderCode:       "openai",
				ModelAPIID:         "gpt-4o",
				ModelDisplayName:   "GPT-4o",
				CallCount:          100,
				TotalTokens:        50000,
				TotalCostMicroUSD:  3000,
				AvgLatencyMS:       1200.0,
				AvgTokensPerSecond: 28.0,
				SuccessRate:        0.97,
				Flags:              []string{"high_cost"},
			},
			{
				ProviderCode:      "anthropic",
				ModelAPIID:        "claude-3",
				ModelDisplayName:  "Claude 3",
				CallCount:         50,
				TotalTokens:       30000,
				TotalCostMicroUSD: 2000,
				SuccessRate:       0.92,
			},
		}
		got := toProtoUsageModelInsights(items)
		if len(got) != 2 {
			t.Fatalf("len = %d, want 2", len(got))
		}
		if got[0].ProviderCode != "openai" {
			t.Errorf("ProviderCode = %q", got[0].ProviderCode)
		}
		if got[1].ModelApiId != "claude-3" {
			t.Errorf("ModelApiId = %q", got[1].ModelApiId)
		}
	})
}

func TestToProtoUsageTrendPoints(t *testing.T) {
	items := []biz.UsageTrendPoint{
		{DateKey: "2026-01-01", CallCount: 10},
		{DateKey: "2026-01-02", CallCount: 20},
	}
	got := toProtoUsageTrendPoints(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].DateKey != "2026-01-01" {
		t.Errorf("DateKey = %q", got[0].DateKey)
	}
	if got[1].CallCount != 20 {
		t.Errorf("CallCount = %d", got[1].CallCount)
	}
}

func TestToProtoUsageBreakdownRows(t *testing.T) {
	items := []biz.UsageBreakdownRow{
		{ProviderCode: "openai", CallCount: 50},
		{ProviderCode: "anthropic", CallCount: 30},
	}
	got := toProtoUsageBreakdownRows(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ProviderCode != "openai" {
		t.Errorf("ProviderCode = %q", got[0].ProviderCode)
	}
	if got[1].CallCount != 30 {
		t.Errorf("CallCount = %d", got[1].CallCount)
	}
}

func TestToProtoTokenUsageEvents(t *testing.T) {
	items := []biz.TokenUsageEvent{
		{ID: "e1", CallCount: 1},
		{ID: "e2", CallCount: 2},
	}
	got := toProtoTokenUsageEvents(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Id != "e1" {
		t.Errorf("Id = %q", got[0].Id)
	}
	if got[1].CallCount != 2 {
		t.Errorf("CallCount = %d", got[1].CallCount)
	}
}

func TestToProtoBudgetAlerts(t *testing.T) {
	items := []biz.BudgetAlert{
		{ID: "ba-1", AlertRatio: 0.5, Enabled: true},
		{ID: "ba-2", AlertRatio: 0.9, Enabled: false},
	}
	got := toProtoBudgetAlerts(items)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Id != "ba-1" {
		t.Errorf("Id = %q", got[0].Id)
	}
	if !got[0].Enabled {
		t.Error("Enabled = false, want true")
	}
	if got[1].Enabled {
		t.Error("Enabled = true, want false")
	}
}
