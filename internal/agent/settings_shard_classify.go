package agent

import (
	"reflect"

	"aranea-agents/internal/biz"
)

// settings_shard_classify.go — P0-2 阶段A 步骤1：AgentRuntimeSettings 字段归类表。
//
// 背景：BuildCacheKey 原把整个 Settings JSON 折进指纹，任何字段（含构建路径
// 根本不消费的 cron/turn 级字段）变化都触发 3-7s 全量重建。本表把全部字段
// 分三桶（FR-2.3 保守默认：拿不准的一律 full_rebuild，行为与现状一致）：
//
//	classFullRebuild     构建产物（toolsets/hooks/agent 实例）消费该字段 → 进指纹
//	classResolverManaged P1-2 resolver 托管（运行时每调用读取）→ 不进指纹、零重建
//	classNoRebuild       构建路径确凿不消费（cron/每轮 DB 重读子系统消费）→ 不进指纹
//
// 正确性论据（2026-08-16 逐字段 grep 审计）：
//   - no_rebuild 桶字段在 internal/agent + internal/service 构建路径零引用；
//     消费方是 cron job（每 tick 读 DB）或 turn 路径（每轮加载 agent 配置），
//     均不经过缓存 agent 的构建时快照。
//   - 反例警示：EvoPersonaMaxChars 经 ResolveMemoryRuntimePolicy 构建时固化、
//     SessionSummaryEnabled/Reasoning*/Workspace/Subagents* 在 trpc_build/prompt
//     组装消费——均已归 full_rebuild，不得移入 no_rebuild。
//
// 守卫：TestSettingsFieldClassification_Guard 反射枚举全部字段强制登记，
// 新增字段不归类则测试红（report-05 风险①化解②）。

type settingsFieldClass int

const (
	classFullRebuild settingsFieldClass = iota
	classResolverManaged
	classNoRebuild
)

// settingsFieldClassification 是唯一权威归类表。键 = AgentRuntimeSettings 字段名。
// 顺序与 struct 声明一致，便于对照审计。
var settingsFieldClassification = map[string]settingsFieldClass{
	"AgentID":               classFullRebuild,
	"SelfEvolve":            classNoRebuild, // deprecated；evolution cron 消费
	"SubagentsEnabled":      classFullRebuild,
	"SubagentsMaxConcurrency":      classFullRebuild,
	"SubagentsMaxGenerationDepth":  classFullRebuild,
	"SubagentsMaxChildrenPerAgent": classFullRebuild,
	"SubagentsArchiveAfterMinutes": classFullRebuild,
	"SubagentsMaxRetries":          classFullRebuild,
	"SubagentsModelOverride":       classFullRebuild,
	"SubagentsStoredResultRunes":   classFullRebuild,
	"SubagentsStoredSummaryRunes":  classFullRebuild,
	"ToolsEnabled":            classFullRebuild,
	"ToolsProfile":            classFullRebuild,
	"ToolsToolCallPrefix":     classFullRebuild,
	"ToolsAllowJSON":          classFullRebuild,
	"ToolsDenyJSON":           classFullRebuild,
	"ToolsConcurrentAllowJSON": classFullRebuild,
	"MemoryEnabled":        classFullRebuild, // ResolveMemoryRuntimePolicy 构建时固化
	"MemoryMaxChunkLength": classFullRebuild,
	"MemoryMaxResults":     classFullRebuild,
	"MemoryMinScore":       classFullRebuild,
	"HeartbeatEnabled":         classNoRebuild, // heartbeat cron 每 tick 读 DB
	"HeartbeatIntervalMinutes": classNoRebuild,
	"EvolutionSelfEvolve":          classNoRebuild, // evolution cron
	"EvolutionSkillEvolve":         classNoRebuild,
	"EvolutionMetricsEnabled":      classNoRebuild,
	"EvolutionSuggestionsEnabled":  classNoRebuild,
	"GuardrailMaxChangePerPeriod":       classNoRebuild, // learning-loop cron
	"GuardrailMinDataPoints":            classNoRebuild,
	"GuardrailRollbackOnDeclinePercent": classNoRebuild,
	// L0–L4：全部经 ResolveMemoryRuntimePolicy / memory hooks 构建时固化。
	"L0RecentWindowTurns":   classFullRebuild,
	"L0RecentWindowTokens":  classFullRebuild,
	"L0SummaryThreshold":    classFullRebuild,
	"L0SummaryKeepTurns":    classFullRebuild,
	"L0CompressMinGapSec":   classFullRebuild,
	"L0CompressProvider":    classFullRebuild,
	"L0CompressModel":       classFullRebuild,
	"MemoryWorkerProvider":  classFullRebuild,
	"MemoryWorkerModel":     classFullRebuild,
	"L0TruncateStrategy":    classFullRebuild,
	"L0InjectL1":            classFullRebuild,
	"L0InjectL3":            classFullRebuild,
	"L0InjectL4":            classFullRebuild,
	"L0L3MaxChunks":         classFullRebuild,
	"L0L4MaxPaths":          classFullRebuild,
	"L0SnapshotMode":        classFullRebuild,
	"L0SnapshotEnabled":     classFullRebuild,
	"L1Enabled":                classFullRebuild,
	"L1BudgetTokens":           classFullRebuild,
	"L1FieldMaxTokens":         classFullRebuild,
	"L1HistoryKeepRevisions":   classFullRebuild,
	"L1DefaultSchemaID":        classFullRebuild,
	"L1HistoryEnabled":         classFullRebuild,
	"L1ArchiveOnIdleMinutes":   classFullRebuild,
	"L2EpisodeEnabled":         classFullRebuild,
	"L2EpisodeMinImportance":   classFullRebuild,
	"L2IndexEnabled":           classFullRebuild,
	"L2IndexEmbeddingModel":    classFullRebuild,
	"L2RecallEnabled":          classFullRebuild,
	"L2RecallMax":              classFullRebuild,
	"L2RetentionDays":          classFullRebuild,
	"L2ArchiveAfterDays":       classFullRebuild,
	"L3Enabled":                classFullRebuild,
	"L3RecallTopK":             classFullRebuild,
	"L3RecallMinScore":         classFullRebuild,
	"L3RecallScopesJSON":       classFullRebuild,
	"L3EmbeddingModel":         classFullRebuild,
	"L3DecayIntervalHours":     classFullRebuild,
	"L3ArchiveThreshold":       classFullRebuild,
	"L3MaxPerRecallChars":      classFullRebuild,
	"L3RecallBudgetTokens":     classFullRebuild,
	"L4Enabled":                classFullRebuild,
	"L4GraphInjectNeighbors":   classFullRebuild,
	"L4GraphMaxNeighbors":      classFullRebuild,
	"L4GraphMaxHops":           classFullRebuild,
	"L4IdentityInject":         classFullRebuild,
	"L4StrategyInject":         classFullRebuild,
	"L4DecayIntervalHours":     classFullRebuild,
	"L4DecayOverridesJSON":     classFullRebuild,
	"EvoEnabled":              classNoRebuild, // evo cron
	"EvoAutoApply":            classNoRebuild,
	"EvoMinEpisodes":          classNoRebuild,
	"EvoMinNegativeFeedback":  classNoRebuild,
	"EvoThrottleHours":        classNoRebuild,
	"EvoProposalTTLDays":      classNoRebuild,
	"EvoPersonaMaxChars":      classFullRebuild, // ResolveMemoryRuntimePolicy 消费（L4PersonaMaxChars）
	"EvoSystemPromptMaxAppends": classNoRebuild, // evo 子系统 JSON 配置消费，构建路径零引用
	"SkillRuntimeJSON":    classFullRebuild, // skill runtime 工具装配
	"IntentPassEnabled":   classNoRebuild,   // intent.ShouldRun 每轮经 DB 加载的 ag 判定
	"ChannelID":           classNoRebuild,   // channel gateway 消费，构建路径零引用
	"ChatID":              classNoRebuild,
	"Workspace":               classFullRebuild, // memory_inject/a2a invoker 构建时固化
	"ReasoningMode":           classFullRebuild, // trpc_build.go GetReasoning()
	"ReasoningLevel":          classFullRebuild,
	"VariablesJSON":           classFullRebuild, // GetIdentity() 保守
	"ModelInstructionsJSON":   classFullRebuild, // GetIdentity() 保守
	"ContextCompactionEnabled":   classFullRebuild,
	"MemoryCompactEnabled":       classFullRebuild,
	"ToolResultGateEnabled":      classFullRebuild,
	"CompressLLMCacheEnabled":    classFullRebuild,
	"CompressLLMCacheMaxEntries": classFullRebuild,
	"CompressLLMCacheTTLSec":     classFullRebuild,
	"CompressionBufferRatio":     classFullRebuild,
	"CompressionBufferAdaptive":  classFullRebuild,
	"SoftTriggerRatio":           classFullRebuild,
	"HardTriggerRatio":           classFullRebuild,
	"SessionSummaryEnabled": classFullRebuild, // trpc_build.go:506 构建路径消费
	"SkillLoadMode":         classFullRebuild,
	"CodeExecutorType":      classFullRebuild,
	"OutputSchemaJSON":      classFullRebuild,
	"ModelSelector":         classFullRebuild,
	"ToolsRetryEnabled":           classFullRebuild,
	"ToolsRetryMaxAttempts":       classFullRebuild,
	"ToolsRetryInitialIntervalMs": classFullRebuild,
	"ToolsRetryBackoffFactor":     classFullRebuild,
	"ToolsRetryMaxIntervalMs":     classFullRebuild,
	"ToolsRetryJitter":            classFullRebuild,
	"ToolsParallelEnabled":        classFullRebuild,
	"ToolsStreamingEnabled":       classFullRebuild,
	"ToolsCircuitBreakerEnabled":       classFullRebuild,
	"ToolsCircuitBreakerOverridesJSON": classFullRebuild,
	"ToolsDeferredJSON":                classFullRebuild,
	"ToolsCommandSafetyEnabled":        classFullRebuild,
	"ToolsExecutionTimeoutSec": classResolverManaged, // P1-2：policyResolver 每调用读取
	"MaxLLMCalls":              classFullRebuild,
	"MaxToolIterations":        classFullRebuild,
	"EnableTokenTailoring":          classFullRebuild,
	"TokenTailoringStrategy":        classFullRebuild,
	"TokenTailoringSafetyMargin":    classFullRebuild,
	"PlannerKind":                   classFullRebuild,
	"PlannerConfigJSON":             classFullRebuild,
	"RalphLoopMaxIterations":        classFullRebuild,
	"RalphLoopCompletionPromise":    classFullRebuild,
	"RalphLoopVerifyCommand":        classFullRebuild,
	"RalphLoopVerifyTimeoutSeconds": classFullRebuild,
	"RalphLoopPromiseTagOpen":       classFullRebuild,
	"RalphLoopPromiseTagClose":      classFullRebuild,
	"RalphLoopVerifyWorkDir":        classFullRebuild,
	"ForgetConfigJSON":          classNoRebuild, // memory butler 子系统
	"ToolWeightJSON":            classNoRebuild, // 权重分析子系统
	"DreamSnapshotJSON":         classNoRebuild, // dream cycle 子系统
	"VerificationTruncateChars": classNoRebuild, // team 验证门 prompt 组装（非 agent 构建路径）
	"ClarificationEnabled":      classNoRebuild, // chat_clarify_gate 每轮 DB 加载判定
	"ReplyReminderEnabled":      classFullRebuild, // callback_chain 中注册 reply-reminder hooks（BUILD 产物）
	"CreatedAt":                 classNoRebuild, // 元数据，不变
	"UpdatedAt":                 classNoRebuild, // 元数据；settings 行任何更新都会 bump，进指纹则白收窄
}

// settingsFingerprintView 返回仅保留 full_rebuild 桶字段值的副本——
// resolver_managed（P1-2）与 no_rebuild 字段清零，不进构建指纹。
// 语义覆盖 P1-2 的 policyStrippedSettings（resolver 字段亦在此清零）。
func settingsFingerprintView(s *biz.AgentRuntimeSettings) *biz.AgentRuntimeSettings {
	if s == nil {
		return nil
	}
	cp := *s
	v := reflect.ValueOf(&cp).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		if settingsFieldClassification[t.Field(i).Name] != classFullRebuild {
			v.Field(i).Set(reflect.Zero(t.Field(i).Type))
		}
	}
	return &cp
}
