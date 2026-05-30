package service_test

import (
	"testing"

	usagev1 "aranea-agents/api/kratos/usage/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestToProtoDataset(t *testing.T) {
	tests := []struct {
		name string
		in   biz.EvalDataset
	}{
		{
			name: "full",
			in: biz.EvalDataset{
				ID: "ds1", Name: "dataset1", Description: "desc1",
				CaseCount: 10, Workspace: "ws1",
				CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
			},
		},
		{name: "zero", in: biz.EvalDataset{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoDataset(tt.in)
			if got.Id != tt.in.ID {
				t.Errorf("Id = %q, want %q", got.Id, tt.in.ID)
			}
			if got.Name != tt.in.Name {
				t.Errorf("Name = %q, want %q", got.Name, tt.in.Name)
			}
			if got.CaseCount != int32(tt.in.CaseCount) {
				t.Errorf("CaseCount = %d, want %d", got.CaseCount, tt.in.CaseCount)
			}
			if got.Workspace != tt.in.Workspace {
				t.Errorf("Workspace = %q, want %q", got.Workspace, tt.in.Workspace)
			}
		})
	}
}

func TestToProtoRun(t *testing.T) {
	in := biz.EvalRun{
		ID: "r1", DatasetID: "ds1", AgentID: "a1", Status: "completed",
		TotalCases: 5, CompletedCases: 5,
		ExactMatchScore: 0.8, ContainsMatchScore: 0.9, LLMJudgeScore: 0.7,
		ToolCallAccuracy: 0.85, PassAtK: 0.6, PassHatK: 0.65,
		TriggerSource: "manual", NumRuns: 1,
		ScoresJSON: `{"f1":0.8}`, ErrorMessage: "",
		StartedAt: "2024-01-01", FinishedAt: "2024-01-01", CreatedAt: "2024-01-01",
	}
	got := service.ToProtoRun(in)
	if got.Id != "r1" {
		t.Errorf("Id = %q, want %q", got.Id, "r1")
	}
	if got.DatasetId != "ds1" {
		t.Errorf("DatasetId = %q, want %q", got.DatasetId, "ds1")
	}
	if got.TotalCases != 5 {
		t.Errorf("TotalCases = %d, want 5", got.TotalCases)
	}
	if got.NumRuns != 1 {
		t.Errorf("NumRuns = %d, want 1", got.NumRuns)
	}
	if got.ScoresJson != `{"f1":0.8}` {
		t.Errorf("ScoresJson = %q, want %q", got.ScoresJson, `{"f1":0.8}`)
	}
}

func TestToProtoCaseResult(t *testing.T) {
	hp := true
	hs := float32(0.9)
	tests := []struct {
		name string
		in   biz.EvalCaseResult
	}{
		{
			name: "with_human_fields",
			in: biz.EvalCaseResult{
				ID: "cr1", RunID: "r1", CaseID: "c1",
				ActualOutput: "out", ExactMatch: true, ContainsMatch: true,
				LLMJudgeScore: 0.8, ToolCallAccuracy: 0.9,
				ErrorMessage: "", CreatedAt: "2024-01-01",
				HumanPass: &hp, HumanScore: &hs, HumanComment: "good",
				AnnotatedAt: "2024-01-02", AnnotatedBy: "admin",
				ScoresJSON: `{"s":1}`,
			},
		},
		{
			name: "nil_human_fields",
			in: biz.EvalCaseResult{
				ID: "cr2", RunID: "r2", CaseID: "c2",
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.ToProtoCaseResult(tt.in)
			if got.Id != tt.in.ID {
				t.Errorf("Id = %q, want %q", got.Id, tt.in.ID)
			}
			if tt.in.HumanPass != nil {
				if got.HumanPass == nil || !*got.HumanPass {
					t.Errorf("HumanPass should be true")
				}
			} else {
				if got.HumanPass != nil {
					t.Errorf("HumanPass should be nil")
				}
			}
			if tt.in.HumanScore != nil {
				if got.HumanScore == nil || *got.HumanScore != *tt.in.HumanScore {
					t.Errorf("HumanScore mismatch")
				}
			} else {
				if got.HumanScore != nil {
					t.Errorf("HumanScore should be nil")
				}
			}
		})
	}
}

func TestFromProtoUsageQuery(t *testing.T) {
	tests := []struct {
		name string
		in   *usagev1.UsageQuery
		want biz.UsageQuery
	}{
		{
			name: "full",
			in: &usagev1.UsageQuery{
				Range: "7d", StartDate: "2024-01-01", EndDate: "2024-01-07",
				ProviderCode: "openai", ModelApiId: "gpt-4",
				AgentId: "a1", TeamId: "t1", UsageKind: "chat",
				Status: "success", Limit: 100, Granularity: "day",
			},
			want: biz.UsageQuery{
				Range: "7d", StartDate: "2024-01-01", EndDate: "2024-01-07",
				ProviderCode: "openai", ModelAPIID: "gpt-4",
				AgentID: "a1", TeamID: "t1", UsageKind: "chat",
				Status: "success", Limit: 100, Granularity: "day",
			},
		},
		{
			name: "nil",
			in:   nil,
			want: biz.UsageQuery{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.FromProtoUsageQuery(tt.in)
			if got.Range != tt.want.Range {
				t.Errorf("Range = %q, want %q", got.Range, tt.want.Range)
			}
			if got.ProviderCode != tt.want.ProviderCode {
				t.Errorf("ProviderCode = %q, want %q", got.ProviderCode, tt.want.ProviderCode)
			}
			if got.ModelAPIID != tt.want.ModelAPIID {
				t.Errorf("ModelAPIID = %q, want %q", got.ModelAPIID, tt.want.ModelAPIID)
			}
			if got.Limit != tt.want.Limit {
				t.Errorf("Limit = %d, want %d", got.Limit, tt.want.Limit)
			}
		})
	}
}

func TestFromProtoTokenUsageEvent(t *testing.T) {
	tests := []struct {
		name string
		in   *usagev1.TokenUsageEvent
	}{
		{
			name: "full",
			in: &usagev1.TokenUsageEvent{
				Id: "e1", OccurredAt: "2024-01-01", DateKey: "2024-01-01",
				ProviderCode: "openai", ModelApiId: "gpt-4",
				InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
				Status: "success",
			},
		},
		{name: "nil", in: nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := service.FromProtoTokenUsageEvent(tt.in)
			if tt.in == nil {
				if got.ID != "" {
					t.Errorf("expected zero value for nil input")
				}
				return
			}
			if got.ID != tt.in.Id {
				t.Errorf("ID = %q, want %q", got.ID, tt.in.Id)
			}
			if got.InputTokens != int(tt.in.InputTokens) {
				t.Errorf("InputTokens = %d, want %d", got.InputTokens, tt.in.InputTokens)
			}
			if got.TotalTokens != int(tt.in.TotalTokens) {
				t.Errorf("TotalTokens = %d, want %d", got.TotalTokens, tt.in.TotalTokens)
			}
		})
	}
}

func TestToProtoUsageQuota(t *testing.T) {
	in := biz.UsageQuota{
		ID: "q1", ScopeType: "agent", ScopeID: "a1",
		MonthlyMicroUSD: 1000000, PeriodStart: "2024-01", PeriodEnd: "2024-01",
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
	}
	got := service.ToProtoUsageQuota(in)
	if got.Id != "q1" {
		t.Errorf("Id = %q, want %q", got.Id, "q1")
	}
	if got.ScopeType != "agent" {
		t.Errorf("ScopeType = %q, want %q", got.ScopeType, "agent")
	}
	if got.MonthlyMicroUsd != 1000000 {
		t.Errorf("MonthlyMicroUsd = %d, want 1000000", got.MonthlyMicroUsd)
	}
}

func TestToProtoBudgetAlert(t *testing.T) {
	in := biz.BudgetAlert{
		ID: "ba1", ScopeType: "agent", ScopeID: "a1",
		AlertRatio: 0.8, Enabled: true, LastFiredAt: "2024-01-01",
		CreatedAt: "2024-01-01", UpdatedAt: "2024-01-02",
	}
	got := service.ToProtoBudgetAlert(in)
	if got.Id != "ba1" {
		t.Errorf("Id = %q, want %q", got.Id, "ba1")
	}
	if got.AlertRatio != 0.8 {
		t.Errorf("AlertRatio = %f, want 0.8", got.AlertRatio)
	}
	if !got.Enabled {
		t.Errorf("Enabled = false, want true")
	}
}

func TestToProtoUsageSummary(t *testing.T) {
	in := biz.UsageSummary{
		CallCount: 100, RequestCount: 80, SuccessCount: 75, FailedCount: 5,
		CancelledCount: 0, InputTokens: 10000, OutputTokens: 5000,
		TotalTokens: 15000, TotalCostMicroUSD: 500,
		AvgLatencyMS: 123.4, AvgTokensPerSecond: 50.5, SuccessRate: 0.9375,
	}
	got := service.ToProtoUsageSummary(in)
	if got.CallCount != 100 {
		t.Errorf("CallCount = %d, want 100", got.CallCount)
	}
	if got.TotalCostMicroUsd != 500 {
		t.Errorf("TotalCostMicroUsd = %d, want 500", got.TotalCostMicroUsd)
	}
	if got.SuccessRate != 0.9375 {
		t.Errorf("SuccessRate = %f, want 0.9375", got.SuccessRate)
	}
}

func TestToProtoUsageTrendPoint(t *testing.T) {
	in := biz.UsageTrendPoint{
		DateKey: "2024-01-01", CallCount: 50, InputTokens: 5000,
		OutputTokens: 2500, TotalTokens: 7500, TotalCostMicroUSD: 200,
		SuccessCount: 45, FailedCount: 5, CancelledCount: 0,
		AvgLatencyMS: 100.0, AvgTokensPerSecond: 60.0,
	}
	got := service.ToProtoUsageTrendPoint(in)
	if got.DateKey != "2024-01-01" {
		t.Errorf("DateKey = %q, want %q", got.DateKey, "2024-01-01")
	}
	if got.CallCount != 50 {
		t.Errorf("CallCount = %d, want 50", got.CallCount)
	}
}

func TestToProtoUsageBreakdownRow(t *testing.T) {
	in := biz.UsageBreakdownRow{
		ProviderCode: "openai", ModelAPIID: "gpt-4", ModelDisplayName: "GPT-4",
		AgentID: "a1", AgentKey: "ak1", CallCount: 30,
		InputTokens: 3000, OutputTokens: 1500, TotalTokens: 4500,
		TotalCostMicroUSD: 100, AvgLatencyMS: 80.0,
		AvgTokensPerSecond: 55.0, SuccessRate: 0.95,
	}
	got := service.ToProtoUsageBreakdownRow(in)
	if got.ProviderCode != "openai" {
		t.Errorf("ProviderCode = %q, want %q", got.ProviderCode, "openai")
	}
	if got.ModelApiId != "gpt-4" {
		t.Errorf("ModelApiId = %q, want %q", got.ModelApiId, "gpt-4")
	}
}

func TestToProtoTokenUsageEvent(t *testing.T) {
	in := biz.TokenUsageEvent{
		ID: "e1", OccurredAt: "2024-01-01", ProviderCode: "openai",
		ModelAPIID: "gpt-4", CallCount: 5,
		InputTokens: 100, OutputTokens: 50, TotalTokens: 150,
		InputCostMicroUSD: 10, OutputCostMicroUSD: 5, TotalCostMicroUSD: 15,
		Status: "success",
	}
	got := service.ToProtoTokenUsageEvent(in)
	if got.Id != "e1" {
		t.Errorf("Id = %q, want %q", got.Id, "e1")
	}
	if got.CallCount != 5 {
		t.Errorf("CallCount = %d, want 5", got.CallCount)
	}
	if got.TotalCostMicroUsd != 15 {
		t.Errorf("TotalCostMicroUsd = %d, want 15", got.TotalCostMicroUsd)
	}
}

func TestToProtoQuotaDashboard(t *testing.T) {
	in := biz.QuotaDashboard{
		ConfiguredCount: 3, TotalCapMicroUSD: 5000000,
		TotalSpentMicroUSD: 2000000, MaxUtilization: 0.85,
	}
	got := service.ToProtoQuotaDashboard(in)
	if got.ConfiguredCount != 3 {
		t.Errorf("ConfiguredCount = %d, want 3", got.ConfiguredCount)
	}
	if got.MaxUtilizationRatio != 0.85 {
		t.Errorf("MaxUtilizationRatio = %f, want 0.85", got.MaxUtilizationRatio)
	}
}

func TestToProtoUsageModelInsights(t *testing.T) {
	in := []biz.UsageModelInsight{
		{
			ProviderCode: "openai", ModelAPIID: "gpt-4", ModelDisplayName: "GPT-4",
			CallCount: 100, TotalTokens: 50000, TotalCostMicroUSD: 1000,
			AvgLatencyMS: 120.0, AvgTokensPerSecond: 40.0, SuccessRate: 0.95,
			Flags: []string{"high_cost"},
		},
	}
	got := service.ToProtoUsageModelInsights(in)
	if len(got) != 1 {
		t.Fatalf("len = %d, want 1", len(got))
	}
	if got[0].ProviderCode != "openai" {
		t.Errorf("ProviderCode = %q, want %q", got[0].ProviderCode, "openai")
	}
	if got[0].CallCount != 100 {
		t.Errorf("CallCount = %d, want 100", got[0].CallCount)
	}
}

func TestToProtoUsageTrendPoints(t *testing.T) {
	in := []biz.UsageTrendPoint{
		{DateKey: "2024-01-01", CallCount: 10},
		{DateKey: "2024-01-02", CallCount: 20},
	}
	got := service.ToProtoUsageTrendPoints(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].DateKey != "2024-01-01" {
		t.Errorf("DateKey[0] = %q, want %q", got[0].DateKey, "2024-01-01")
	}
	if got[1].CallCount != 20 {
		t.Errorf("CallCount[1] = %d, want 20", got[1].CallCount)
	}
}

func TestToProtoUsageBreakdownRows(t *testing.T) {
	in := []biz.UsageBreakdownRow{
		{ProviderCode: "openai", CallCount: 10},
		{ProviderCode: "anthropic", CallCount: 5},
	}
	got := service.ToProtoUsageBreakdownRows(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].ProviderCode != "openai" {
		t.Errorf("ProviderCode[0] = %q, want %q", got[0].ProviderCode, "openai")
	}
}

func TestToProtoTokenUsageEvents(t *testing.T) {
	in := []biz.TokenUsageEvent{
		{ID: "e1", Status: "success"},
		{ID: "e2", Status: "failed"},
	}
	got := service.ToProtoTokenUsageEvents(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Id != "e1" {
		t.Errorf("Id[0] = %q, want %q", got[0].Id, "e1")
	}
}

func TestToProtoBudgetAlerts(t *testing.T) {
	in := []biz.BudgetAlert{
		{ID: "ba1", AlertRatio: 0.5},
		{ID: "ba2", AlertRatio: 0.8},
	}
	got := service.ToProtoBudgetAlerts(in)
	if len(got) != 2 {
		t.Fatalf("len = %d, want 2", len(got))
	}
	if got[0].Id != "ba1" {
		t.Errorf("Id[0] = %q, want %q", got[0].Id, "ba1")
	}
}

func TestTokenUsageEvent_RoundTrip(t *testing.T) {
	original := biz.TokenUsageEvent{
		ID: "rt1", OccurredAt: "2024-01-01", DateKey: "2024-01-01",
		ProviderCode: "openai", ModelAPIID: "gpt-4",
		CallCount: 3, InputTokens: 100, OutputTokens: 50,
		TotalTokens: 150, TotalCostMicroUSD: 15,
		Status: "success",
	}
	pb := service.ToProtoTokenUsageEvent(original)
	back := service.FromProtoTokenUsageEvent(pb)
	if back.ID != original.ID {
		t.Errorf("roundtrip ID = %q, want %q", back.ID, original.ID)
	}
	if back.CallCount != original.CallCount {
		t.Errorf("roundtrip CallCount = %d, want %d", back.CallCount, original.CallCount)
	}
	if back.TotalCostMicroUSD != original.TotalCostMicroUSD {
		t.Errorf("roundtrip TotalCostMicroUSD = %d, want %d", back.TotalCostMicroUSD, original.TotalCostMicroUSD)
	}
}
