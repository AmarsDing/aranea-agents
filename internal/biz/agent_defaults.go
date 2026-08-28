package biz

// DefaultAgentRuntimeSettings returns built-in defaults for missing agent_runtime_settings rows.
// Fields are grouped by domain (subagents / tools / memory L0–L4 / evolution / skills / context)
// to keep the flat literal scannable; domains mirror AgentRuntimeSettings doc in agent_runtime_settings.go.
func DefaultAgentRuntimeSettings() AgentRuntimeSettings {
	return AgentRuntimeSettings{
		// --- Subagents (evolution domain) ---
		SelfEvolve:                   false,
		SubagentsEnabled:             true,
		SubagentsMaxConcurrency:      5,
		SubagentsMaxGenerationDepth:  1,
		SubagentsMaxChildrenPerAgent: 5,
		SubagentsArchiveAfterMinutes: 60,
		SubagentsMaxRetries:          2,
		SubagentsStoredResultRunes:   4000,
		SubagentsStoredSummaryRunes:  240,

		// --- Tools: profile + allow/deny ---
		ToolsEnabled:             true,
		ToolsProfile:             "coding",
		ToolsAllowJSON:           "[]",
		ToolsDenyJSON:            DefaultToolsDenyFrameworkMemory,
		ToolsConcurrentAllowJSON: "[]",

		// --- Tools: retry / parallel / streaming ---
		ToolsRetryEnabled:           true,
		ToolsRetryMaxAttempts:       2,
		ToolsRetryInitialIntervalMs: 500,
		ToolsRetryBackoffFactor:     2.0,
		ToolsRetryMaxIntervalMs:     5000,
		ToolsRetryJitter:            true,
		// P2-B1: 默认开启并行工具执行。安全性由项目层 ToolDecorator
		// (internal/tools/decorator.go) 保证：超时走 ToolsExecutionTimeoutSec；
		// 结果预算 10KB；Exclusive 按 family / 目标文件互斥。ConcurrentSafe
		// 工具可并行且启用确定性缓存。详见 ADR:
		// docs/reports/2026-06-15-review-adr-tool-parallel-execution.md
		ToolsParallelEnabled:  true,
		ToolsStreamingEnabled: false,

		// --- Memory: base + heartbeat ---
		MemoryEnabled:            true,
		MemoryMaxChunkLength:     1000,
		MemoryMaxResults:         6,
		MemoryMinScore:           0.35,
		HeartbeatIntervalMinutes: 30,

		// --- Memory L0 ---
		L0RecentWindowTurns:  12,
		L0RecentWindowTokens: 0,
		L0SummaryThreshold:   0.6,
		L0SummaryKeepTurns:   4,
		L0CompressMinGapSec:  600,
		// hybrid: compress LLM sees user/assistant only — tool dumps stay out
		// of the rolling summary (drop_tool_results + keep recent turns).
		L0TruncateStrategy: "hybrid",
		L0InjectL1:         true,
		L0InjectL3:         true,
		// 单机默认不预灌 L4：图路径按需工具取，避免每轮打图谱。
		// 存量行显式 true 不受影响——defaultBool 只兜底零值。
		L0InjectL4:        false,
		L0L3MaxChunks:     5,
		L0L4MaxPaths:      3,
		L0SnapshotMode:    "on_warning",
		L0SnapshotEnabled: true,

		// --- Memory L1 ---
		L1Enabled:              true,
		L1BudgetTokens:         8192,
		L1FieldMaxTokens:       2048,
		L1HistoryKeepRevisions: 10,
		L1HistoryEnabled:       false,
		L1ArchiveOnIdleMinutes: 60,

		// --- Memory L2 ---
		L2EpisodeEnabled:       true,
		L2EpisodeMinImportance: 0.3,
		L2IndexEnabled:         true,
		// FR-12/P2: L2 召回默认开（评审 V7：默认关 L2 召回是「层数多≠能力强」
		// 根因之一；standard 档位 = profile 卡 + L2/L3 召回）。
		L2RecallEnabled:    true,
		L2RecallMax:        3,
		L2RetentionDays:    90,
		L2ArchiveAfterDays: 30,

		// --- Memory L3 ---
		L3Enabled:    true,
		L3RecallTopK: 5,
		// P0-4 (2026-08-08): 0.55 会误杀典型相关命中（加权 Total≈0.4-0.5）。
		L3RecallMinScore:     0.35,
		L3RecallScopesJSON:   `["agent","user","team","workspace"]`,
		L3DecayIntervalHours: 24,
		L3ArchiveThreshold:   0.2,
		L3MaxPerRecallChars:  1500,
		// FR-12/P2: 召回块 token 预算默认 standard 档（800）。
		L3RecallBudgetTokens: MemoryRecallBudgetStandard,
		// 2026-08-20：L2 独立召回预算，默认与 L3 同档（800）。
		L2RecallBudgetTokens: MemoryRecallBudgetStandard,
		// 2026-08-28 Wave 2：L3 provenance 默认关（token 噪声）；记忆管家/评测显式开启。
		L3InjectProvenance: false,

		// --- Memory L4 ---
		L4Enabled:              true,
		L4GraphInjectNeighbors: true,
		L4GraphMaxNeighbors:    6,
		L4GraphMaxHops:         2,
		L4IdentityInject:       true,
		L4StrategyInject:       false,
		L4DecayIntervalHours:   168,

		// --- Evolution: metrics / guardrail / evo loop ---
		EvolutionSelfEvolve:               false,
		EvolutionSkillEvolve:              false,
		EvolutionMetricsEnabled:           false,
		EvolutionSuggestionsEnabled:       false,
		GuardrailMaxChangePerPeriod:       0.1,
		GuardrailMinDataPoints:            100,
		GuardrailRollbackOnDeclinePercent: 20,
		EvoEnabled:                        false,
		EvoAutoApply:                      false,
		EvoMinEpisodes:                    20,
		EvoMinNegativeFeedback:            3,
		EvoThrottleHours:                  24,
		EvoProposalTTLDays:                14,
		EvoPersonaMaxChars:                1500,
		EvoSystemPromptMaxAppends:         5,

		// --- Skills ---
		SkillRuntimeJSON: "{}",
		// 2026-08-21 对齐 Cursor 渐进披露：空值不再落框架 turn，默认
		// progressive（routed slug 紧凑提示 + skill_load 按需取正文）。
		// 存量行显式存储的值（如 "turn"）不受影响——defaultString 只兜底空值。
		SkillLoadMode:     SkillLoadModeProgressive,
		IntentPassEnabled: true,
		// 包B（session-eval-20260825 B1）：skip 快路径默认开（现状）；管理层
		// agent 经 SQL 置 false（P-INTENT-SKIP，宁重勿轻 R4）。
		IntentSkipEnabled: true,
		// Empty = auto: Factory prefers docker when the daemon is up.
		CodeExecutorType: "",

		// --- Context / compression ---
		PlannerConfigJSON:    "{}",
		ClarificationEnabled: true,
		// 2026-08-21 全链路审查 A3：reply reminder 默认关。多工具任务中每次
		// 工具调用后 reminder hook 触发额外一轮模型调用（实测约 +3.5s/轮），
		// 收益场景（长链工具任务防跑偏）由模板/评测显式开启。存量行显式
		// 存储的值不受影响——bool 默认只作用于新建行。
		ReplyReminderEnabled:      false,
		VerificationTruncateChars: 2000,
		CompressionBufferRatio:    DefaultCompressionBufferRatio,
		SoftTriggerRatio:          DefaultSoftTriggerRatio,
		HardTriggerRatio:          DefaultHardTriggerRatio,
		// N2 (2026-08-13 链路审查): 压缩级联默认开。默认关导致框架摘要消费侧
		// （AddSessionSummary cutoff）与请求级 compaction 从未生效，历史无上限
		// 增长（__spirit__ 实测平均 prompt 60K tokens）。存量行由数据迁移
		// 20260813 compression_default_on 翻转。
		ContextCompactionEnabled: true,
		MemoryCompactEnabled:     true,
		SessionSummaryEnabled:    true,
	}
}
