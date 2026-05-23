package biz

import "aranea-agents/internal/biz/usage"

type (
	UsageQuery           = usage.Query
	UsageSummary         = usage.Summary
	UsageTrendPoint      = usage.TrendPoint
	UsageBreakdownRow    = usage.BreakdownRow
	TokenUsageEvent      = usage.TokenUsageEvent
	UsageOverview        = usage.Overview
	ModelPricingSnapshot = usage.ModelPricingSnapshot
	UsageAnalyticsRepo   = usage.AnalyticsRepo
	UsageWriteRepo       = usage.WriteRepo
	UsageQuotaRepo       = usage.QuotaRepo
	UsageRepo            = usage.Repo
	UsageUsecase         = usage.Usecase
	UsageQuota           = usage.Quota
	UsageQuotaCheck      = usage.QuotaCheck
	BudgetAlert          = usage.BudgetAlert
	UsageAlertNotifier   = usage.AlertNotifier
	QuotaDashboard       = usage.QuotaDashboard
	UsageModelInsight    = usage.ModelInsight
)

const (
	UsageKindChatTurn   = usage.KindChatTurn
	UsageKindTeamMember = usage.KindTeamMember
	UsageKindTeamTurn   = usage.KindTeamTurn
	QuotaScopeGlobal    = usage.QuotaScopeGlobal
	GlobalQuotaScopeID  = usage.GlobalQuotaScopeID
)

var (
	NewUsageUsecase      = usage.NewUsecase
	NormalizeUsageStatus = usage.NormalizeStatus
	ApplyTokenUsageCosts = usage.ApplyTokenUsageCosts
	MapUsageRepoErr      = usage.MapRepoErr
)
