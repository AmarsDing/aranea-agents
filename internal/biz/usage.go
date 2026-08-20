package biz

import "aranea-agents/internal/biz/usage"

type (
	UsageQuery                = usage.Query
	UsageSummary              = usage.Summary
	UsageTrendPoint           = usage.TrendPoint
	UsageBreakdownRow         = usage.BreakdownRow
	UsageBreakdownQuery       = usage.BreakdownQuery
	UsageBreakdownResult      = usage.BreakdownResult
	TokenUsageEvent           = usage.TokenUsageEvent
	UsageOverview             = usage.Overview
	ModelPricingSnapshot      = usage.ModelPricingSnapshot
	UsageAnalyticsRepo        = usage.AnalyticsRepo
	UsageWriteRepo            = usage.WriteRepo
	UsageQuotaRepo            = usage.QuotaRepo
	UsageRepo                 = usage.Repo
	UsageUsecase              = usage.Usecase
	// UsageUsecaseRef mirrors usage.UsecaseRef (P1-2 late-binding cell).
	UsageUsecaseRef = usage.UsecaseRef
	UsageQuota                = usage.Quota
	UsageQuotaCheck           = usage.QuotaCheck
	BudgetAlert               = usage.BudgetAlert
	UsageAlertNotifier        = usage.AlertNotifier
	QuotaDashboard            = usage.QuotaDashboard
	UsageModelInsight         = usage.ModelInsight
	TeamQuotaReader           = usage.TeamQuotaReader
	SessionMetricsAccumulator = usage.SessionMetricsAccumulator
	CompletionUsageLinker     = usage.CompletionUsageLinker
	UsageEnvelopePublisher    = usage.UsageEnvelopePublisher
	TurnUsageInput            = usage.TurnUsageInput
	AuxLLMUsageInput          = usage.AuxLLMUsageInput
	CacheHitRatioStat         = usage.CacheHitRatioStat
	CacheHitRatioStatsRepo    = usage.CacheHitRatioStatsRepo
	ContextBudgetStats        = usage.ContextBudgetStats
	ContextBudgetComposition  = usage.ContextBudgetComposition
	ContextBudgetAgentStats   = usage.ContextBudgetAgentStats
	ContextBudgetTrendPoint   = usage.ContextBudgetTrendPoint
	ContextBudgetToolStat     = usage.ContextBudgetToolStat
	ContextBudgetGrain        = usage.ContextBudgetGrain
	ContextBudgetStatsRepo    = usage.ContextBudgetStatsRepo
)

const (
	UsageKindChatTurn   = usage.KindChatTurn
	UsageKindTeamMember = usage.KindTeamMember
	UsageKindTeamTurn   = usage.KindTeamTurn
	// Aux kinds mirror usage.KindAux* (P1-1, 2026-08-19).
	UsageKindAuxSubagent  = usage.KindAuxSubagent
	UsageKindAuxTitle     = usage.KindAuxTitle
	UsageKindAuxIntent    = usage.KindAuxIntent
	UsageKindAuxEvolution = usage.KindAuxEvolution
	// UsageKindAuxMemoryExtract mirrors usage.KindAuxMemoryExtract (P2-D).
	UsageKindAuxMemoryExtract = usage.KindAuxMemoryExtract
	// UsageKindAuxEmbedding mirrors usage.KindAuxEmbedding (P1-3).
	UsageKindAuxEmbedding = usage.KindAuxEmbedding
	QuotaScopeGlobal      = usage.QuotaScopeGlobal
	GlobalQuotaScopeID    = usage.GlobalQuotaScopeID
	// MinCacheablePromptTokens mirrors usage.MinCacheablePromptTokens.
	MinCacheablePromptTokens = usage.MinCacheablePromptTokens
	// MetadataKeyUsageSource mirrors usage.MetadataKeyUsageSource.
	MetadataKeyUsageSource = usage.MetadataKeyUsageSource
	// UsageSourceResponse mirrors usage.UsageSourceResponse.
	UsageSourceResponse = usage.UsageSourceResponse
	// MetadataKeyUsageAttribution mirrors usage.MetadataKeyUsageAttribution (P2-1).
	MetadataKeyUsageAttribution = usage.MetadataKeyUsageAttribution
	// UsageAttributionRunLevelAnchorFallback mirrors usage.UsageAttributionRunLevelAnchorFallback (P2-1).
	UsageAttributionRunLevelAnchorFallback = usage.UsageAttributionRunLevelAnchorFallback
	// UsageAttributionMemberLevelStream mirrors usage.UsageAttributionMemberLevelStream (P2-1b).
	UsageAttributionMemberLevelStream = usage.UsageAttributionMemberLevelStream
	// UsageAttributionStreamAnchorRemainder mirrors usage.UsageAttributionStreamAnchorRemainder (P2-1b).
	UsageAttributionStreamAnchorRemainder = usage.UsageAttributionStreamAnchorRemainder
)

var (
	NewUsageUsecase      = usage.NewUsecase
	NormalizeUsageStatus = usage.NormalizeStatus
	ApplyTokenUsageCosts = usage.ApplyTokenUsageCosts
	MapUsageRepoErr      = usage.MapRepoErr
	// MergeUsageSourceMetadata mirrors usage.MergeUsageSourceMetadata.
	MergeUsageSourceMetadata = usage.MergeUsageSourceMetadata
	// MergeLLMRoundsMetadata mirrors usage.MergeLLMRoundsMetadata (P1-C).
	MergeLLMRoundsMetadata = usage.MergeLLMRoundsMetadata
	// MergeUsageAttributionMetadata mirrors usage.MergeUsageAttributionMetadata (P2-1).
	MergeUsageAttributionMetadata = usage.MergeUsageAttributionMetadata
	// NewUsageUsecaseRef mirrors usage.NewUsecaseRef.
	NewUsageUsecaseRef = usage.NewUsecaseRef
)
