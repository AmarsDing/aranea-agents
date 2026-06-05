package service_test

import (
	"testing"

	v1 "aranea-agents/api/kratos/agent/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/service"
)

func TestFromProtoRuntime_Nil(t *testing.T) {
	if got := service.FromProtoRuntime(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestToProtoRuntime_Nil(t *testing.T) {
	if got := service.ToProtoRuntime(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestFromProtoRuntime_RoundTrip(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		AgentId:                           "a1",
		ChannelId:                         "ch1",
		ChatId:                            "chat1",
		Workspace:                         "ws1",
		VariablesJson:                     `{"k":"v"}`,
		ModelInstructionsJson:             `{"gpt-4o":"inst"}`,
		ReasoningMode:                     "step",
		ReasoningLevel:                    "high",
		MemoryEnabled:                     true,
		MemoryMaxChunkLength:              512,
		MemoryMaxResults:                  8,
		MemoryMinScore:                    0.4,
		HeartbeatEnabled:                  true,
		HeartbeatIntervalMinutes:          15,
		L0RecentWindowTurns:               10,
		L0RecentWindowTokens:              2000,
		L0SummaryThreshold:                0.55,
		L0SummaryKeepTurns:                3,
		L0CompressProvider:                "openrouter",
		L0CompressModel:                   "gpt-4o-mini",
		MemoryWorkerProvider:              "openai",
		MemoryWorkerModel:                 "gpt-4o",
		L0TruncateStrategy:                "summary",
		L0InjectL1:                        true,
		L0InjectL3:                        false,
		L0InjectL4:                        true,
		L0L3MaxChunks:                     5,
		L0L4MaxPaths:                      3,
		L0SnapshotMode:                    "full",
		L1Enabled:                         true,
		L1BudgetTokens:                    8000,
		L1FieldMaxTokens:                  500,
		L1HistoryKeepRevisions:            2,
		L1DefaultSchemaId:                 "sch1",
		L1ArchiveOnIdleMinutes:            60,
		L2EpisodeEnabled:                  true,
		L2EpisodeMinImportance:            0.7,
		L2IndexEnabled:                    true,
		L2IndexEmbeddingModel:             "text-embedding-3",
		L2RecallEnabled:                   true,
		L2RecallMax:                       10,
		L2RetentionDays:                   90,
		L2ArchiveAfterDays:                180,
		L3Enabled:                         true,
		L3RecallTopK:                      5,
		L3RecallMinScore:                  0.3,
		L3RecallScopesJson:                `["global"]`,
		L3EmbeddingModel:                  "emb3",
		L3DecayIntervalHours:              24,
		L3ArchiveThreshold:                0.1,
		L3MaxPerRecallChars:               4000,
		L4Enabled:                         true,
		L4GraphInjectNeighbors:            true,
		L4GraphMaxNeighbors:               10,
		L4GraphMaxHops:                    3,
		L4IdentityInject:                  true,
		L4StrategyInject:                  false,
		ToolsEnabled:                      true,
		ToolsProfile:                      "coding",
		ToolsToolCallPrefix:               "/",
		ToolsAllowJson:                    `["*"]`,
		ToolsDenyJson:                     `[]`,
		ToolsConcurrentAllowJson:          `["search"]`,
		SkillRuntimeJson:                  `{"allow":["*"]}`,
		IntentPassEnabled:                 true,
		SkillLoadMode:                     "auto",
		SelfEvolve:                        true,
		SubagentsEnabled:                  true,
		SubagentsMaxConcurrency:           5,
		SubagentsMaxGenerationDepth:       2,
		SubagentsMaxChildrenPerAgent:      3,
		SubagentsArchiveAfterMinutes:      30,
		SubagentsMaxRetries:               2,
		SubagentsModelOverride:            "gpt-4o",
		EvolutionSelfEvolve:               true,
		EvolutionSkillEvolve:              true,
		EvolutionMetricsEnabled:           true,
		EvolutionSuggestionsEnabled:       true,
		GuardrailMaxChangePerPeriod:       0.15,
		GuardrailMinDataPoints:            50,
		GuardrailRollbackOnDeclinePercent: 10,
		EvoEnabled:                        true,
		EvoAutoApply:                      false,
		EvoMinEpisodes:                    20,
		EvoMinNegativeFeedback:            5,
		EvoThrottleHours:                  48,
		EvoProposalTtlDays:                7,
		EvoPersonaMaxChars:                2000,
		EvoSystemPromptMaxAppends:         3,
		ContextCompactionEnabled:          true,
		SessionSummaryEnabled:             true,
		OutputSchemaJson:                  `{"type":"object"}`,
		ModelSelector:                     "auto",
		PlannerKind:                       "react",
		PlannerConfigJson:                 `{"max_steps":5}`,
		ToolsRetryEnabled:                 true,
		ToolsRetryMaxAttempts:             3,
		ToolsRetryInitialIntervalMs:       1000,
		ToolsRetryBackoffFactor:           2.0,
		ToolsRetryMaxIntervalMs:           10000,
		ToolsRetryJitter:                  true,
		ToolsParallelEnabled:              true,
		ToolsStreamingEnabled:             false,
		RalphLoopMaxIterations:            10,
		RalphLoopCompletionPromise:        "done",
		RalphLoopVerifyCommand:            "test",
		RalphLoopVerifyTimeoutSeconds:     30,
		RalphLoopPromiseTagOpen:           "<promise>",
		RalphLoopPromiseTagClose:          "</promise>",
		RalphLoopVerifyWorkDir:            "/tmp",
		CodeExecutorType:                  "docker",
		CreatedAt:                         "2024-01-01",
		UpdatedAt:                         "2024-06-01",
	}

	bizObj := service.FromProtoRuntime(pb)
	if bizObj == nil {
		t.Fatal("expected non-nil biz object")
	}

	id := bizObj.GetIdentity()
	if id.AgentID != "a1" || id.ChannelID != "ch1" || id.ChatID != "chat1" {
		t.Fatalf("identity mismatch: %+v", id)
	}
	if id.Workspace != "ws1" || id.VariablesJSON != `{"k":"v"}` || id.ModelInstructionsJSON != `{"gpt-4o":"inst"}` {
		t.Fatalf("identity workspace/vars mismatch: %+v", id)
	}

	rsn := bizObj.GetReasoning()
	if rsn.Mode != "step" || rsn.Level != "high" {
		t.Fatalf("reasoning mismatch: %+v", rsn)
	}

	mem := bizObj.GetMemory()
	if !mem.Enabled || mem.MaxChunkLength != 512 || mem.MaxResults != 8 {
		t.Fatalf("memory mismatch: %+v", mem)
	}
	if mem.MinScore != 0.4 || !mem.HeartbeatEnabled || mem.HeartbeatIntervalMinutes != 15 {
		t.Fatalf("memory heartbeat mismatch: %+v", mem)
	}
	if mem.L0RecentWindowTurns != 10 || mem.L0RecentWindowTokens != 2000 {
		t.Fatalf("memory L0 window mismatch: %+v", mem)
	}
	if mem.L0SummaryThreshold != 0.55 || mem.L0SummaryKeepTurns != 3 {
		t.Fatalf("memory L0 summary mismatch: %+v", mem)
	}
	if mem.L0CompressProvider != "openrouter" || mem.L0CompressModel != "gpt-4o-mini" {
		t.Fatalf("memory L0 compress mismatch: %+v", mem)
	}
	if mem.MemoryWorkerProvider != "openai" || mem.MemoryWorkerModel != "gpt-4o" {
		t.Fatalf("memory worker mismatch: %+v", mem)
	}
	if mem.L0TruncateStrategy != "summary" || !mem.L0InjectL1 || mem.L0InjectL3 || !mem.L0InjectL4 {
		t.Fatalf("memory L0 inject mismatch: %+v", mem)
	}
	if mem.L0L3MaxChunks != 5 || mem.L0L4MaxPaths != 3 || mem.L0SnapshotMode != "full" {
		t.Fatalf("memory L0 snapshot mismatch: %+v", mem)
	}
	if !mem.L1Enabled || mem.L1BudgetTokens != 8000 || mem.L1FieldMaxTokens != 500 {
		t.Fatalf("memory L1 mismatch: %+v", mem)
	}
	if mem.L1HistoryKeepRevisions != 2 || mem.L1DefaultSchemaID != "sch1" || mem.L1ArchiveOnIdleMinutes != 60 {
		t.Fatalf("memory L1 detail mismatch: %+v", mem)
	}
	if !mem.L2EpisodeEnabled || mem.L2EpisodeMinImportance != 0.7 {
		t.Fatalf("memory L2 episode mismatch: %+v", mem)
	}
	if !mem.L2IndexEnabled || mem.L2IndexEmbeddingModel != "text-embedding-3" {
		t.Fatalf("memory L2 index mismatch: %+v", mem)
	}
	if !mem.L2RecallEnabled || mem.L2RecallMax != 10 || mem.L2RetentionDays != 90 || mem.L2ArchiveAfterDays != 180 {
		t.Fatalf("memory L2 recall mismatch: %+v", mem)
	}
	if !mem.L3Enabled || mem.L3RecallTopK != 5 || mem.L3RecallMinScore != 0.3 {
		t.Fatalf("memory L3 mismatch: %+v", mem)
	}
	if mem.L3RecallScopesJSON != `["global"]` || mem.L3EmbeddingModel != "emb3" {
		t.Fatalf("memory L3 detail mismatch: %+v", mem)
	}
	if mem.L3DecayIntervalHours != 24 || mem.L3ArchiveThreshold != 0.1 || mem.L3MaxPerRecallChars != 4000 {
		t.Fatalf("memory L3 decay mismatch: %+v", mem)
	}
	if !mem.L4Enabled || !mem.L4GraphInjectNeighbors || mem.L4GraphMaxNeighbors != 10 {
		t.Fatalf("memory L4 mismatch: %+v", mem)
	}
	if mem.L4GraphMaxHops != 3 || !mem.L4IdentityInject || mem.L4StrategyInject {
		t.Fatalf("memory L4 detail mismatch: %+v", mem)
	}

	tools := bizObj.GetTools()
	if !tools.Enabled || tools.Profile != "coding" || tools.ToolCallPrefix != "/" {
		t.Fatalf("tools mismatch: %+v", tools)
	}
	if tools.AllowJSON != `["*"]` || tools.DenyJSON != `[]` || tools.ConcurrentAllowJSON != `["search"]` {
		t.Fatalf("tools allow/deny mismatch: %+v", tools)
	}
	if !tools.RetryEnabled || tools.RetryMaxAttempts != 3 || tools.RetryInitialIntervalMs != 1000 {
		t.Fatalf("tools retry mismatch: %+v", tools)
	}
	if tools.RetryBackoffFactor != 2.0 || tools.RetryMaxIntervalMs != 10000 || !tools.RetryJitter {
		t.Fatalf("tools retry detail mismatch: %+v", tools)
	}
	if !tools.ParallelEnabled || tools.StreamingEnabled {
		t.Fatalf("tools parallel/streaming mismatch: %+v", tools)
	}

	skills := bizObj.GetSkills()
	if skills.RuntimeJSON != `{"allow":["*"]}` || !skills.IntentPassEnabled || skills.LoadMode != "auto" {
		t.Fatalf("skills mismatch: %+v", skills)
	}

	evo := bizObj.GetEvolution()
	if !evo.SelfEvolve || !evo.SubagentsEnabled || evo.SubagentsMaxConcurrency != 5 {
		t.Fatalf("evolution mismatch: %+v", evo)
	}
	if evo.SubagentsMaxGenerationDepth != 2 || evo.SubagentsMaxChildrenPerAgent != 3 {
		t.Fatalf("evolution subagents mismatch: %+v", evo)
	}
	if evo.SubagentsArchiveAfterMinutes != 30 || evo.SubagentsMaxRetries != 2 || evo.SubagentsModelOverride != "gpt-4o" {
		t.Fatalf("evolution subagents detail mismatch: %+v", evo)
	}
	if !evo.SkillEvolve || !evo.MetricsEnabled || !evo.SuggestionsEnabled {
		t.Fatalf("evolution flags mismatch: %+v", evo)
	}
	if evo.GuardrailMaxChangePerPeriod != 0.15 || evo.GuardrailMinDataPoints != 50 {
		t.Fatalf("evolution guardrail mismatch: %+v", evo)
	}
	if evo.GuardrailRollbackOnDeclinePercent != 10 {
		t.Fatalf("evolution guardrail rollback mismatch: %+v", evo)
	}
	if !evo.EvoEnabled || evo.EvoAutoApply || evo.EvoMinEpisodes != 20 {
		t.Fatalf("evolution evo mismatch: %+v", evo)
	}
	if evo.EvoMinNegativeFeedback != 5 || evo.EvoThrottleHours != 48 || evo.EvoProposalTTLDays != 7 {
		t.Fatalf("evolution evo detail mismatch: %+v", evo)
	}
	if evo.EvoPersonaMaxChars != 2000 || evo.EvoSystemPromptMaxAppends != 3 {
		t.Fatalf("evolution evo chars mismatch: %+v", evo)
	}

	ctx := bizObj.GetContext()
	if !ctx.CompactionEnabled || !ctx.SessionSummaryEnabled {
		t.Fatalf("context mismatch: %+v", ctx)
	}
	if ctx.OutputSchemaJSON != `{"type":"object"}` || ctx.ModelSelector != "auto" {
		t.Fatalf("context detail mismatch: %+v", ctx)
	}
	if ctx.PlannerKind != "react" || ctx.PlannerConfigJSON != `{"max_steps":5}` {
		t.Fatalf("context planner mismatch: %+v", ctx)
	}

	if bizObj.RalphLoopMaxIterations != 10 || bizObj.RalphLoopCompletionPromise != "done" {
		t.Fatalf("ralph loop mismatch: %+v", bizObj)
	}
	if bizObj.RalphLoopVerifyCommand != "test" || bizObj.RalphLoopVerifyTimeoutSeconds != 30 {
		t.Fatalf("ralph loop verify mismatch: %+v", bizObj)
	}
	if bizObj.RalphLoopPromiseTagOpen != "<promise>" || bizObj.RalphLoopPromiseTagClose != "</promise>" {
		t.Fatalf("ralph loop tags mismatch: %+v", bizObj)
	}
	if bizObj.RalphLoopVerifyWorkDir != "/tmp" {
		t.Fatalf("ralph loop workdir mismatch: got %q", bizObj.RalphLoopVerifyWorkDir)
	}
	if bizObj.CodeExecutorType != "docker" {
		t.Fatalf("code_executor_type mismatch: got %q, want %q", bizObj.CodeExecutorType, "docker")
	}
}

func TestToProtoRuntime_RoundTrip(t *testing.T) {
	bizObj := &biz.AgentRuntimeSettings{
		AgentID:                           "a1",
		ChannelID:                         "ch1",
		ChatID:                            "chat1",
		Workspace:                         "ws1",
		VariablesJSON:                     `{"k":"v"}`,
		ModelInstructionsJSON:             `{"gpt-4o":"inst"}`,
		ReasoningMode:                     "step",
		ReasoningLevel:                    "high",
		MemoryEnabled:                     true,
		MemoryMaxChunkLength:              512,
		MemoryMaxResults:                  8,
		MemoryMinScore:                    0.4,
		HeartbeatEnabled:                  true,
		HeartbeatIntervalMinutes:          15,
		L0RecentWindowTurns:               10,
		L0RecentWindowTokens:              2000,
		L0SummaryThreshold:                0.55,
		L0SummaryKeepTurns:                3,
		L0CompressProvider:                "openrouter",
		L0CompressModel:                   "gpt-4o-mini",
		MemoryWorkerProvider:              "openai",
		MemoryWorkerModel:                 "gpt-4o",
		L0TruncateStrategy:                "summary",
		L0InjectL1:                        true,
		L0InjectL3:                        false,
		L0InjectL4:                        true,
		L0L3MaxChunks:                     5,
		L0L4MaxPaths:                      3,
		L0SnapshotMode:                    "full",
		L1Enabled:                         true,
		L1BudgetTokens:                    8000,
		L1FieldMaxTokens:                  500,
		L1HistoryKeepRevisions:            2,
		L1DefaultSchemaID:                 "sch1",
		L1ArchiveOnIdleMinutes:            60,
		L2EpisodeEnabled:                  true,
		L2EpisodeMinImportance:            0.7,
		L2IndexEnabled:                    true,
		L2IndexEmbeddingModel:             "text-embedding-3",
		L2RecallEnabled:                   true,
		L2RecallMax:                       10,
		L2RetentionDays:                   90,
		L2ArchiveAfterDays:                180,
		L3Enabled:                         true,
		L3RecallTopK:                      5,
		L3RecallMinScore:                  0.3,
		L3RecallScopesJSON:                `["global"]`,
		L3EmbeddingModel:                  "emb3",
		L3DecayIntervalHours:              24,
		L3ArchiveThreshold:                0.1,
		L3MaxPerRecallChars:               4000,
		L4Enabled:                         true,
		L4GraphInjectNeighbors:            true,
		L4GraphMaxNeighbors:               10,
		L4GraphMaxHops:                    3,
		L4IdentityInject:                  true,
		L4StrategyInject:                  false,
		ToolsEnabled:                      true,
		ToolsProfile:                      "coding",
		ToolsToolCallPrefix:               "/",
		ToolsAllowJSON:                    `["*"]`,
		ToolsDenyJSON:                     `[]`,
		ToolsConcurrentAllowJSON:          `["search"]`,
		SkillRuntimeJSON:                  `{"allow":["*"]}`,
		IntentPassEnabled:                 true,
		SkillLoadMode:                     "auto",
		EvolutionSelfEvolve:               true,
		SubagentsEnabled:                  true,
		SubagentsMaxConcurrency:           5,
		SubagentsMaxGenerationDepth:       2,
		SubagentsMaxChildrenPerAgent:      3,
		SubagentsArchiveAfterMinutes:      30,
		SubagentsMaxRetries:               2,
		SubagentsModelOverride:            "gpt-4o",
		EvolutionSkillEvolve:              true,
		EvolutionMetricsEnabled:           true,
		EvolutionSuggestionsEnabled:       true,
		GuardrailMaxChangePerPeriod:       0.15,
		GuardrailMinDataPoints:            50,
		GuardrailRollbackOnDeclinePercent: 10,
		EvoEnabled:                        true,
		EvoAutoApply:                      false,
		EvoMinEpisodes:                    20,
		EvoMinNegativeFeedback:            5,
		EvoThrottleHours:                  48,
		EvoProposalTTLDays:                7,
		EvoPersonaMaxChars:                2000,
		EvoSystemPromptMaxAppends:         3,
		ContextCompactionEnabled:          true,
		SessionSummaryEnabled:             true,
		OutputSchemaJSON:                  `{"type":"object"}`,
		ModelSelector:                     "auto",
		PlannerKind:                       "react",
		PlannerConfigJSON:                 `{"max_steps":5}`,
		ToolsRetryEnabled:                 true,
		ToolsRetryMaxAttempts:             3,
		ToolsRetryInitialIntervalMs:       1000,
		ToolsRetryBackoffFactor:           2.0,
		ToolsRetryMaxIntervalMs:           10000,
		ToolsRetryJitter:                  true,
		ToolsParallelEnabled:              true,
		ToolsStreamingEnabled:             false,
		RalphLoopMaxIterations:            10,
		RalphLoopCompletionPromise:        "done",
		RalphLoopVerifyCommand:            "test",
		RalphLoopVerifyTimeoutSeconds:     30,
		RalphLoopPromiseTagOpen:           "<promise>",
		RalphLoopPromiseTagClose:          "</promise>",
		RalphLoopVerifyWorkDir:            "/tmp",
		CreatedAt:                         "2024-01-01",
		UpdatedAt:                         "2024-06-01",
	}

	pb := service.ToProtoRuntime(bizObj)
	if pb == nil {
		t.Fatal("expected non-nil proto")
	}
	if pb.GetAgentId() != "a1" {
		t.Fatalf("agent_id mismatch: %q", pb.GetAgentId())
	}
	if pb.GetChannelId() != "ch1" || pb.GetChatId() != "chat1" {
		t.Fatalf("channel/chat mismatch: ch=%q chat=%q", pb.GetChannelId(), pb.GetChatId())
	}
	if pb.GetWorkspace() != "ws1" {
		t.Fatalf("workspace mismatch: %q", pb.GetWorkspace())
	}
	if pb.GetReasoningMode() != "step" || pb.GetReasoningLevel() != "high" {
		t.Fatalf("reasoning mismatch: mode=%q level=%q", pb.GetReasoningMode(), pb.GetReasoningLevel())
	}
	if !pb.GetMemoryEnabled() || pb.GetMemoryMaxChunkLength() != 512 {
		t.Fatalf("memory mismatch: enabled=%v chunk=%d", pb.GetMemoryEnabled(), pb.GetMemoryMaxChunkLength())
	}
	if pb.GetMemoryMinScore() != 0.4 || !pb.GetHeartbeatEnabled() {
		t.Fatalf("memory detail mismatch: score=%v heartbeat=%v", pb.GetMemoryMinScore(), pb.GetHeartbeatEnabled())
	}
	if pb.GetL0RecentWindowTurns() != 10 || pb.GetL0RecentWindowTokens() != 2000 {
		t.Fatalf("L0 window mismatch: turns=%d tokens=%d", pb.GetL0RecentWindowTurns(), pb.GetL0RecentWindowTokens())
	}
	if pb.GetL0CompressProvider() != "openrouter" || pb.GetL0CompressModel() != "gpt-4o-mini" {
		t.Fatalf("L0 compress mismatch: provider=%q model=%q", pb.GetL0CompressProvider(), pb.GetL0CompressModel())
	}
	if pb.GetMemoryWorkerProvider() != "openai" || pb.GetMemoryWorkerModel() != "gpt-4o" {
		t.Fatalf("memory worker mismatch: provider=%q model=%q", pb.GetMemoryWorkerProvider(), pb.GetMemoryWorkerModel())
	}
	if !pb.GetToolsEnabled() || pb.GetToolsProfile() != "coding" {
		t.Fatalf("tools mismatch: enabled=%v profile=%q", pb.GetToolsEnabled(), pb.GetToolsProfile())
	}
	if !pb.GetToolsRetryEnabled() || pb.GetToolsRetryMaxAttempts() != 3 {
		t.Fatalf("tools retry mismatch: enabled=%v attempts=%d", pb.GetToolsRetryEnabled(), pb.GetToolsRetryMaxAttempts())
	}
	if pb.GetToolsRetryBackoffFactor() != 2.0 || pb.GetToolsRetryMaxIntervalMs() != 10000 {
		t.Fatalf("tools retry detail mismatch: factor=%v max=%d", pb.GetToolsRetryBackoffFactor(), pb.GetToolsRetryMaxIntervalMs())
	}
	if !pb.GetToolsParallelEnabled() || pb.GetToolsStreamingEnabled() {
		t.Fatalf("tools parallel/streaming mismatch: parallel=%v streaming=%v", pb.GetToolsParallelEnabled(), pb.GetToolsStreamingEnabled())
	}
	if pb.GetSkillRuntimeJson() != `{"allow":["*"]}` || !pb.GetIntentPassEnabled() || pb.GetSkillLoadMode() != "auto" {
		t.Fatalf("skills mismatch: json=%q intent=%v mode=%q", pb.GetSkillRuntimeJson(), pb.GetIntentPassEnabled(), pb.GetSkillLoadMode())
	}
	if !pb.GetEvoEnabled() || pb.GetEvoAutoApply() {
		t.Fatalf("evo mismatch: enabled=%v auto=%v", pb.GetEvoEnabled(), pb.GetEvoAutoApply())
	}
	if pb.GetEvoMinEpisodes() != 20 || pb.GetEvoMinNegativeFeedback() != 5 {
		t.Fatalf("evo detail mismatch: episodes=%d feedback=%d", pb.GetEvoMinEpisodes(), pb.GetEvoMinNegativeFeedback())
	}
	if pb.GetEvoThrottleHours() != 48 || pb.GetEvoProposalTtlDays() != 7 {
		t.Fatalf("evo throttle mismatch: hours=%d days=%d", pb.GetEvoThrottleHours(), pb.GetEvoProposalTtlDays())
	}
	if pb.GetEvoPersonaMaxChars() != 2000 || pb.GetEvoSystemPromptMaxAppends() != 3 {
		t.Fatalf("evo chars mismatch: persona=%d appends=%d", pb.GetEvoPersonaMaxChars(), pb.GetEvoSystemPromptMaxAppends())
	}
	if !pb.GetContextCompactionEnabled() || !pb.GetSessionSummaryEnabled() {
		t.Fatalf("context mismatch: compaction=%v summary=%v", pb.GetContextCompactionEnabled(), pb.GetSessionSummaryEnabled())
	}
	if pb.GetModelSelector() != "auto" || pb.GetPlannerKind() != "react" {
		t.Fatalf("context detail mismatch: selector=%q planner=%q", pb.GetModelSelector(), pb.GetPlannerKind())
	}
	if pb.GetRalphLoopMaxIterations() != 10 || pb.GetRalphLoopCompletionPromise() != "done" {
		t.Fatalf("ralph loop mismatch: iter=%d promise=%q", pb.GetRalphLoopMaxIterations(), pb.GetRalphLoopCompletionPromise())
	}
	if pb.GetRalphLoopVerifyCommand() != "test" || pb.GetRalphLoopVerifyTimeoutSeconds() != 30 {
		t.Fatalf("ralph loop verify mismatch: cmd=%q timeout=%d", pb.GetRalphLoopVerifyCommand(), pb.GetRalphLoopVerifyTimeoutSeconds())
	}
	if pb.GetRalphLoopPromiseTagOpen() != "<promise>" || pb.GetRalphLoopPromiseTagClose() != "</promise>" {
		t.Fatalf("ralph loop tags mismatch: open=%q close=%q", pb.GetRalphLoopPromiseTagOpen(), pb.GetRalphLoopPromiseTagClose())
	}
	if pb.GetRalphLoopVerifyWorkDir() != "/tmp" {
		t.Fatalf("ralph loop workdir mismatch: %q", pb.GetRalphLoopVerifyWorkDir())
	}
	if pb.GetCreatedAt() != "2024-01-01" || pb.GetUpdatedAt() != "2024-06-01" {
		t.Fatalf("timestamps mismatch: created=%q updated=%q", pb.GetCreatedAt(), pb.GetUpdatedAt())
	}
}

func TestFromProtoFile_Nil(t *testing.T) {
	got := service.FromProtoFile(nil)
	if got.ID != "" || got.AgentID != "" {
		t.Fatalf("expected zero-value AgentPromptFile, got %+v", got)
	}
}

func TestToProtoFile(t *testing.T) {
	b := biz.AgentPromptFile{
		ID:        "f1",
		AgentID:   "a1",
		Name:      "system.md",
		Body:      "You are helpful.",
		SortOrder: 1,
		CreatedAt: "2024-01-01",
		UpdatedAt: "2024-06-01",
	}
	pb := service.ToProtoFile(b)
	if pb.GetId() != "f1" || pb.GetAgentId() != "a1" {
		t.Fatalf("id mismatch: id=%q agent=%q", pb.GetId(), pb.GetAgentId())
	}
	if pb.GetName() != "system.md" || pb.GetBody() != "You are helpful." {
		t.Fatalf("name/body mismatch: name=%q body=%q", pb.GetName(), pb.GetBody())
	}
	if pb.GetSortOrder() != 1 {
		t.Fatalf("sort_order mismatch: %d", pb.GetSortOrder())
	}
}

func TestFromProtoFile_ToProtoFile_RoundTrip(t *testing.T) {
	pb := &v1.AgentPromptFile{
		Id:        "f2",
		AgentId:   "a2",
		Name:      "rules.md",
		Body:      "Follow rules.",
		SortOrder: 2,
		CreatedAt: "2024-02-01",
		UpdatedAt: "2024-07-01",
	}
	bizFile := service.FromProtoFile(pb)
	pb2 := service.ToProtoFile(bizFile)
	if pb2.GetId() != pb.GetId() || pb2.GetAgentId() != pb.GetAgentId() {
		t.Fatalf("round-trip id mismatch")
	}
	if pb2.GetName() != pb.GetName() || pb2.GetBody() != pb.GetBody() {
		t.Fatalf("round-trip name/body mismatch")
	}
	if pb2.GetSortOrder() != pb.GetSortOrder() {
		t.Fatalf("round-trip sort_order mismatch")
	}
}

func TestFromProtoA2AProxy_Nil(t *testing.T) {
	if got := service.FromProtoA2AProxy(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestToProtoA2AProxy_Nil(t *testing.T) {
	if got := service.ToProtoA2AProxy(nil); got != nil {
		t.Fatalf("expected nil, got %v", got)
	}
}

func TestFromProtoA2AProxy_EmptyURLs(t *testing.T) {
	pb := &v1.A2AProxyConfig{}
	if got := service.FromProtoA2AProxy(pb); got != nil {
		t.Fatalf("expected nil for empty URLs, got %v", got)
	}
}

func TestFromProtoA2AProxy_ToProtoA2AProxy_RoundTrip(t *testing.T) {
	pb := &v1.A2AProxyConfig{
		RemoteUrl:       "https://remote.example.com",
		AgentCardUrl:    "https://card.example.com",
		EnableStreaming:  true,
		AuthType:        "bearer",
		AuthConfigJson:  `{"token":"xxx"}`,
		TimeoutSeconds:  30,
	}
	bizCfg := service.FromProtoA2AProxy(pb)
	if bizCfg == nil {
		t.Fatal("expected non-nil")
	}
	if bizCfg.RemoteURL != "https://remote.example.com" || bizCfg.AgentCardURL != "https://card.example.com" {
		t.Fatalf("URL mismatch: remote=%q card=%q", bizCfg.RemoteURL, bizCfg.AgentCardURL)
	}
	if !bizCfg.EnableStreaming || bizCfg.AuthType != "bearer" {
		t.Fatalf("streaming/auth mismatch: streaming=%v auth=%q", bizCfg.EnableStreaming, bizCfg.AuthType)
	}
	if bizCfg.AuthConfigJSON != `{"token":"xxx"}` || bizCfg.TimeoutSeconds != 30 {
		t.Fatalf("auth config/timeout mismatch: json=%q timeout=%d", bizCfg.AuthConfigJSON, bizCfg.TimeoutSeconds)
	}

	pb2 := service.ToProtoA2AProxy(bizCfg)
	if pb2.GetRemoteUrl() != "https://remote.example.com" {
		t.Fatalf("round-trip remote URL mismatch: %q", pb2.GetRemoteUrl())
	}
	if pb2.GetAgentCardUrl() != "https://card.example.com" {
		t.Fatalf("round-trip card URL mismatch: %q", pb2.GetAgentCardUrl())
	}
	if !pb2.GetEnableStreaming() || pb2.GetAuthType() != "bearer" {
		t.Fatalf("round-trip streaming/auth mismatch")
	}
	if pb2.GetTimeoutSeconds() != 30 {
		t.Fatalf("round-trip timeout mismatch: %d", pb2.GetTimeoutSeconds())
	}
}

func TestFromProtoAgent_Nil(t *testing.T) {
	got := service.FromProtoAgent(nil)
	if got.ID != "" {
		t.Fatalf("expected zero-value Agent, got %+v", got)
	}
}

func TestToProtoAgent_BasicFields(t *testing.T) {
	b := biz.Agent{
		ID:                 "agent-1",
		AgentKey:           "fox",
		DisplayName:        "小狐",
		Provider:           "openrouter",
		Model:              "gpt-4o",
		Status:             "active",
		IsDefault:          true,
		IsFavorite:         false,
		Icon:               "pets",
		AgentDescription:   "desc",
		TaxonomyPositionID: "cat-1",
		PositionKey:        "pos-1",
		AgentVariant:       "standard",
		VariantDescription: "vdesc",
		SystemPromptMode:   "concat",
		ContextWindow:      128000,
		BudgetMonthlyCents: 1000,
		ConfigJSON:         `{"agent_kind":"llm"}`,
		CreatedAt:          "2024-01-01",
		UpdatedAt:          "2024-06-01",
		DeletedAt:          "",
		Kind:               "llm",
		Readonly:           true,
		A2AEndpointEnabled: true,
		LastRunStatus:      "completed",
		LastRunAt:          "2024-06-15",
		PendingEvolutionCount: 3,
		CreatedBy:          "admin-1",
	}
	pb := service.ToProtoAgent(b)
	if pb.GetId() != "agent-1" || pb.GetAgentKey() != "fox" {
		t.Fatalf("id/key mismatch: id=%q key=%q", pb.GetId(), pb.GetAgentKey())
	}
	if pb.GetDisplayName() != "小狐" || pb.GetProvider() != "openrouter" {
		t.Fatalf("display/provider mismatch: name=%q provider=%q", pb.GetDisplayName(), pb.GetProvider())
	}
	if pb.GetModel() != "gpt-4o" || pb.GetStatus() != "active" {
		t.Fatalf("model/status mismatch: model=%q status=%q", pb.GetModel(), pb.GetStatus())
	}
	if !pb.GetIsDefault() || pb.GetIsFavorite() {
		t.Fatalf("default/favorite mismatch: default=%v favorite=%v", pb.GetIsDefault(), pb.GetIsFavorite())
	}
	if pb.GetIcon() != "pets" || pb.GetAgentDescription() != "desc" {
		t.Fatalf("icon/desc mismatch: icon=%q desc=%q", pb.GetIcon(), pb.GetAgentDescription())
	}
	if pb.GetContextWindow() != 128000 || pb.GetBudgetMonthlyCents() != 1000 {
		t.Fatalf("context/budget mismatch: ctx=%d budget=%d", pb.GetContextWindow(), pb.GetBudgetMonthlyCents())
	}
	if pb.GetAgentKind() != "llm" {
		t.Fatalf("kind mismatch: %q", pb.GetAgentKind())
	}
	if !pb.GetReadonly() {
		t.Fatalf("readonly mismatch: %v", pb.GetReadonly())
	}
	if !pb.GetA2AEndpointEnabled() {
		t.Fatalf("a2a endpoint mismatch: %v", pb.GetA2AEndpointEnabled())
	}
	if pb.GetLastRunStatus() != "completed" || pb.GetLastRunAt() != "2024-06-15" {
		t.Fatalf("last run mismatch: status=%q at=%q", pb.GetLastRunStatus(), pb.GetLastRunAt())
	}
	if pb.GetPendingEvolutionCount() != 3 || pb.GetCreatedBy() != "admin-1" {
		t.Fatalf("evo/creator mismatch: evo=%d creator=%q", pb.GetPendingEvolutionCount(), pb.GetCreatedBy())
	}
}

func TestFromProtoAgent_ToProtoAgent_RoundTrip(t *testing.T) {
	pb := &v1.Agent{
		Id:                 "agent-2",
		AgentKey:           "dev",
		DisplayName:        "Dev",
		Provider:           "openai",
		Model:              "gpt-4o-mini",
		Status:             "active",
		IsDefault:          false,
		IsFavorite:         true,
		Icon:               "code",
		AgentDescription:   "dev agent",
		TaxonomyPositionId: "cat-2",
		PositionKey:        "pos-2",
		AgentVariant:       "pro",
		VariantDescription: "pro variant",
		SystemPromptMode:   "file",
		ContextWindow:      64000,
		BudgetMonthlyCents: 500,
		ConfigJson:         `{"agent_kind":"llm"}`,
		CreatedAt:          "2024-01-01",
		UpdatedAt:          "2024-06-01",
		AgentKind:          "llm",
		Readonly:           false,
		Files: []*v1.AgentPromptFile{
			{Id: "f1", AgentId: "agent-2", Name: "system.md", Body: "hello", SortOrder: 1},
		},
		A2AProxyConfig: &v1.A2AProxyConfig{
			RemoteUrl:      "https://remote.test",
			AgentCardUrl:   "https://card.test",
			EnableStreaming: true,
			AuthType:       "none",
			TimeoutSeconds: 10,
		},
	}

	b := service.FromProtoAgent(pb)
	if b.ID != "agent-2" || b.AgentKey != "dev" {
		t.Fatalf("id/key mismatch: id=%q key=%q", b.ID, b.AgentKey)
	}
	if b.DisplayName != "Dev" || b.Provider != "openai" {
		t.Fatalf("name/provider mismatch: name=%q provider=%q", b.DisplayName, b.Provider)
	}
	if !b.IsFavorite || b.IsDefault {
		t.Fatalf("favorite/default mismatch: fav=%v def=%v", b.IsFavorite, b.IsDefault)
	}
	if len(b.Files) != 1 || b.Files[0].Name != "system.md" {
		t.Fatalf("files mismatch: len=%d", len(b.Files))
	}
	if b.A2AProxy == nil || b.A2AProxy.RemoteURL != "https://remote.test" {
		t.Fatalf("a2a proxy mismatch: %+v", b.A2AProxy)
	}

	pb2 := service.ToProtoAgent(b)
	if pb2.GetId() != "agent-2" || pb2.GetAgentKey() != "dev" {
		t.Fatalf("round-trip id/key mismatch: id=%q key=%q", pb2.GetId(), pb2.GetAgentKey())
	}
	if pb2.GetDisplayName() != "Dev" {
		t.Fatalf("round-trip display name mismatch: %q", pb2.GetDisplayName())
	}
	if len(pb2.GetFiles()) != 1 || pb2.GetFiles()[0].GetName() != "system.md" {
		t.Fatalf("round-trip files mismatch: len=%d", len(pb2.GetFiles()))
	}
	if pb2.GetA2AProxyConfig() == nil || pb2.GetA2AProxyConfig().GetRemoteUrl() != "https://remote.test" {
		t.Fatalf("round-trip a2a proxy mismatch")
	}
}

func TestFromProtoCreate_Nil(t *testing.T) {
	got := service.FromProtoCreate(nil)
	if got.AgentKey != "" {
		t.Fatalf("expected zero-value Agent, got %+v", got)
	}
}

func TestFromProtoCreate_BasicFields(t *testing.T) {
	req := &v1.CreateAgentRequest{
		AgentKey:           "new-agent",
		DisplayName:        "New Agent",
		Provider:           "anthropic",
		Model:              "claude-3",
		Icon:               "star",
		AgentDescription:   "a new agent",
		TaxonomyPositionId: "cat-3",
		PositionKey:        "pos-3",
		AgentVariant:       "lite",
		VariantDescription: "lite variant",
		SystemPromptMode:   "concat",
		ContextWindow:      32000,
		BudgetMonthlyCents: 200,
		ConfigJson:         `{"agent_kind":"llm"}`,
		AgentKind:          "llm",
		Files: []*v1.AgentPromptFile{
			{Id: "f1", AgentId: "", Name: "system.md", Body: "prompt", SortOrder: 0},
		},
		A2AProxyConfig: &v1.A2AProxyConfig{
			RemoteUrl: "https://a2a.test",
		},
	}
	b := service.FromProtoCreate(req)
	if b.AgentKey != "new-agent" || b.DisplayName != "New Agent" {
		t.Fatalf("key/name mismatch: key=%q name=%q", b.AgentKey, b.DisplayName)
	}
	if b.Provider != "anthropic" || b.Model != "claude-3" {
		t.Fatalf("provider/model mismatch: provider=%q model=%q", b.Provider, b.Model)
	}
	if b.ContextWindow != 32000 || b.BudgetMonthlyCents != 200 {
		t.Fatalf("context/budget mismatch: ctx=%d budget=%d", b.ContextWindow, b.BudgetMonthlyCents)
	}
	if len(b.Files) != 1 || b.Files[0].Name != "system.md" {
		t.Fatalf("files mismatch: len=%d", len(b.Files))
	}
	if b.A2AProxy == nil || b.A2AProxy.RemoteURL != "https://a2a.test" {
		t.Fatalf("a2a proxy mismatch: %+v", b.A2AProxy)
	}
}

func TestBizEffectiveToolsToProto(t *testing.T) {
	in := biz.AgentEffectiveTools{
		ToolsEnabled: true,
		Profile:      "coding",
		Allow:        []string{"tool1", "tool2"},
		Deny:         []string{"tool3"},
		Items: []biz.EffectiveAgentTool{
			{
				ToolKey:        "search",
				DisplayName:    "Search",
				Category:       "web",
				Source:         "builtin",
				Enabled:        true,
				EffectiveState: "active",
				Reason:         "default",
			},
		},
	}
	pb := service.BizEffectiveToolsToProto(in)
	if !pb.GetToolsEnabled() || pb.GetProfile() != "coding" {
		t.Fatalf("tools enabled/profile mismatch: enabled=%v profile=%q", pb.GetToolsEnabled(), pb.GetProfile())
	}
	if len(pb.GetAllow()) != 2 || pb.GetAllow()[0] != "tool1" {
		t.Fatalf("allow mismatch: %v", pb.GetAllow())
	}
	if len(pb.GetDeny()) != 1 || pb.GetDeny()[0] != "tool3" {
		t.Fatalf("deny mismatch: %v", pb.GetDeny())
	}
	if len(pb.GetItems()) != 1 {
		t.Fatalf("items length mismatch: %d", len(pb.GetItems()))
	}
	item := pb.GetItems()[0]
	if item.GetToolKey() != "search" || item.GetDisplayName() != "Search" {
		t.Fatalf("item key/name mismatch: key=%q name=%q", item.GetToolKey(), item.GetDisplayName())
	}
	if item.GetCategory() != "web" || item.GetSource() != "builtin" {
		t.Fatalf("item category/source mismatch: cat=%q src=%q", item.GetCategory(), item.GetSource())
	}
	if !item.GetEnabled() || item.GetEffectiveState() != "active" || item.GetReason() != "default" {
		t.Fatalf("item state mismatch: enabled=%v state=%q reason=%q", item.GetEnabled(), item.GetEffectiveState(), item.GetReason())
	}
}

func TestBizEffectiveToolsToProto_Empty(t *testing.T) {
	in := biz.AgentEffectiveTools{}
	pb := service.BizEffectiveToolsToProto(in)
	if pb.GetToolsEnabled() || pb.GetProfile() != "" {
		t.Fatalf("expected zero values: enabled=%v profile=%q", pb.GetToolsEnabled(), pb.GetProfile())
	}
	if len(pb.GetAllow()) != 0 || len(pb.GetDeny()) != 0 || len(pb.GetItems()) != 0 {
		t.Fatalf("expected empty slices: allow=%d deny=%d items=%d", len(pb.GetAllow()), len(pb.GetDeny()), len(pb.GetItems()))
	}
}

func TestFromProtoIdentity(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		AgentId:               "a1",
		ChannelId:             "ch1",
		ChatId:                "chat1",
		Workspace:             "ws1",
		VariablesJson:         `{"x":1}`,
		ModelInstructionsJson: `{"gpt-4o":"be concise"}`,
	}
	cfg := service.FromProtoIdentity(pb)
	if cfg.AgentID != "a1" || cfg.ChannelID != "ch1" {
		t.Fatalf("identity mismatch: %+v", cfg)
	}
	if cfg.ChatID != "chat1" || cfg.Workspace != "ws1" {
		t.Fatalf("chat/workspace mismatch: %+v", cfg)
	}
	if cfg.VariablesJSON != `{"x":1}` || cfg.ModelInstructionsJSON != `{"gpt-4o":"be concise"}` {
		t.Fatalf("vars/instructions mismatch: %+v", cfg)
	}
}

func TestFromProtoReasoning(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		ReasoningMode:  "step",
		ReasoningLevel: "high",
	}
	cfg := service.FromProtoReasoning(pb)
	if cfg.Mode != "step" || cfg.Level != "high" {
		t.Fatalf("reasoning mismatch: %+v", cfg)
	}
}

func TestFromProtoMemory(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		MemoryEnabled:            true,
		MemoryMaxChunkLength:     256,
		MemoryMaxResults:         4,
		MemoryMinScore:           0.5,
		HeartbeatEnabled:         true,
		HeartbeatIntervalMinutes: 20,
	}
	cfg := service.FromProtoMemory(pb)
	if !cfg.Enabled || cfg.MaxChunkLength != 256 || cfg.MaxResults != 4 {
		t.Fatalf("memory mismatch: %+v", cfg)
	}
	if cfg.MinScore != 0.5 || !cfg.HeartbeatEnabled || cfg.HeartbeatIntervalMinutes != 20 {
		t.Fatalf("memory detail mismatch: %+v", cfg)
	}
}

func TestFromProtoTools(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		ToolsEnabled:             true,
		ToolsProfile:             "coding",
		ToolsToolCallPrefix:      "/",
		ToolsAllowJson:           `["*"]`,
		ToolsDenyJson:            `[]`,
		ToolsConcurrentAllowJson: `["search"]`,
		ToolsRetryEnabled:        true,
		ToolsRetryMaxAttempts:    3,
		ToolsRetryJitter:         true,
		ToolsParallelEnabled:     true,
		ToolsStreamingEnabled:    false,
	}
	cfg := service.FromProtoTools(pb)
	if !cfg.Enabled || cfg.Profile != "coding" {
		t.Fatalf("tools mismatch: %+v", cfg)
	}
	if cfg.AllowJSON != `["*"]` || cfg.DenyJSON != `[]` {
		t.Fatalf("tools allow/deny mismatch: %+v", cfg)
	}
	if !cfg.RetryEnabled || cfg.RetryMaxAttempts != 3 || !cfg.RetryJitter {
		t.Fatalf("tools retry mismatch: %+v", cfg)
	}
	if !cfg.ParallelEnabled || cfg.StreamingEnabled {
		t.Fatalf("tools parallel/streaming mismatch: %+v", cfg)
	}
}

func TestFromProtoSkills(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		SkillRuntimeJson:  `{"allow":["*"]}`,
		IntentPassEnabled: true,
		SkillLoadMode:     "auto",
	}
	cfg := service.FromProtoSkills(pb)
	if cfg.RuntimeJSON != `{"allow":["*"]}` || !cfg.IntentPassEnabled || cfg.LoadMode != "auto" {
		t.Fatalf("skills mismatch: %+v", cfg)
	}
}

func TestFromProtoEvolution(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		SelfEvolve:                        true,
		SubagentsEnabled:                  true,
		SubagentsMaxConcurrency:           10,
		SubagentsMaxGenerationDepth:       2,
		SubagentsMaxChildrenPerAgent:      5,
		SubagentsArchiveAfterMinutes:      60,
		SubagentsMaxRetries:               3,
		SubagentsModelOverride:            "gpt-4o",
		EvolutionSkillEvolve:              true,
		EvolutionMetricsEnabled:           true,
		EvolutionSuggestionsEnabled:       true,
		GuardrailMaxChangePerPeriod:       0.2,
		GuardrailMinDataPoints:            100,
		GuardrailRollbackOnDeclinePercent: 15,
		EvoEnabled:                        true,
		EvoAutoApply:                      true,
		EvoMinEpisodes:                    50,
		EvoMinNegativeFeedback:            10,
		EvoThrottleHours:                  24,
		EvoProposalTtlDays:                14,
		EvoPersonaMaxChars:                3000,
		EvoSystemPromptMaxAppends:         5,
	}
	cfg := service.FromProtoEvolution(pb)
	if !cfg.SelfEvolve || !cfg.SubagentsEnabled || cfg.SubagentsMaxConcurrency != 10 {
		t.Fatalf("evolution mismatch: %+v", cfg)
	}
	if cfg.SubagentsMaxGenerationDepth != 2 || cfg.SubagentsMaxChildrenPerAgent != 5 {
		t.Fatalf("evolution subagents depth/children mismatch: %+v", cfg)
	}
	if cfg.SubagentsArchiveAfterMinutes != 60 || cfg.SubagentsMaxRetries != 3 {
		t.Fatalf("evolution subagents archive/retry mismatch: %+v", cfg)
	}
	if cfg.SubagentsModelOverride != "gpt-4o" {
		t.Fatalf("evolution model override mismatch: %q", cfg.SubagentsModelOverride)
	}
	if !cfg.SkillEvolve || !cfg.MetricsEnabled || !cfg.SuggestionsEnabled {
		t.Fatalf("evolution flags mismatch: %+v", cfg)
	}
	if cfg.GuardrailMaxChangePerPeriod != 0.2 || cfg.GuardrailMinDataPoints != 100 {
		t.Fatalf("evolution guardrail mismatch: %+v", cfg)
	}
	if cfg.GuardrailRollbackOnDeclinePercent != 15 {
		t.Fatalf("evolution guardrail rollback mismatch: %d", cfg.GuardrailRollbackOnDeclinePercent)
	}
	if !cfg.EvoEnabled || !cfg.EvoAutoApply || cfg.EvoMinEpisodes != 50 {
		t.Fatalf("evolution evo mismatch: %+v", cfg)
	}
	if cfg.EvoMinNegativeFeedback != 10 || cfg.EvoThrottleHours != 24 || cfg.EvoProposalTTLDays != 14 {
		t.Fatalf("evolution evo detail mismatch: %+v", cfg)
	}
	if cfg.EvoPersonaMaxChars != 3000 || cfg.EvoSystemPromptMaxAppends != 5 {
		t.Fatalf("evolution evo chars mismatch: %+v", cfg)
	}
}

func TestFromProtoContext(t *testing.T) {
	pb := &v1.AgentRuntimeSettings{
		ContextCompactionEnabled: true,
		SessionSummaryEnabled:    true,
		OutputSchemaJson:         `{"type":"object"}`,
		ModelSelector:            "auto",
		PlannerKind:              "react",
		PlannerConfigJson:        `{"max_steps":5}`,
	}
	cfg := service.FromProtoContext(pb)
	if !cfg.CompactionEnabled || !cfg.SessionSummaryEnabled {
		t.Fatalf("context mismatch: %+v", cfg)
	}
	if cfg.OutputSchemaJSON != `{"type":"object"}` || cfg.ModelSelector != "auto" {
		t.Fatalf("context detail mismatch: %+v", cfg)
	}
	if cfg.PlannerKind != "react" || cfg.PlannerConfigJSON != `{"max_steps":5}` {
		t.Fatalf("context planner mismatch: %+v", cfg)
	}
}

func TestFromProtoAgent_WithSettings(t *testing.T) {
	pb := &v1.Agent{
		Id:        "a1",
		AgentKey:  "test",
		AgentKind: "llm",
		Settings: &v1.AgentRuntimeSettings{
			AgentId:      "a1",
			MemoryEnabled: true,
			ToolsEnabled:  true,
		},
	}
	b := service.FromProtoAgent(pb)
	if b.Settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if !b.Settings.MemoryEnabled || !b.Settings.ToolsEnabled {
		t.Fatalf("settings mismatch: %+v", b.Settings)
	}
}

func TestFromProtoAgent_WithNilSettings(t *testing.T) {
	pb := &v1.Agent{
		Id:        "a1",
		AgentKey:  "test",
		AgentKind: "llm",
	}
	b := service.FromProtoAgent(pb)
	if b.Settings != nil {
		t.Fatalf("expected nil settings, got %+v", b.Settings)
	}
}

func TestToProtoAgent_WithSettings(t *testing.T) {
	b := biz.Agent{
		ID:       "a1",
		AgentKey: "test",
		Kind:     "llm",
		Settings: &biz.AgentRuntimeSettings{
			AgentID:       "a1",
			MemoryEnabled: true,
			ToolsEnabled:  true,
		},
	}
	pb := service.ToProtoAgent(b)
	if pb.GetSettings() == nil {
		t.Fatal("expected non-nil settings")
	}
	if !pb.GetSettings().GetMemoryEnabled() || !pb.GetSettings().GetToolsEnabled() {
		t.Fatalf("settings mismatch: %+v", pb.GetSettings())
	}
}

func TestToProtoAgent_WithNilSettings(t *testing.T) {
	b := biz.Agent{
		ID:       "a1",
		AgentKey: "test",
		Kind:     "llm",
	}
	pb := service.ToProtoAgent(b)
	if pb.GetSettings() != nil {
		t.Fatalf("expected nil settings, got %+v", pb.GetSettings())
	}
}

func TestFromProtoCreate_WithSettings(t *testing.T) {
	enabled := true
	req := &v1.CreateAgentRequest{
		AgentKey:  "new",
		AgentKind: "llm",
		Settings: &v1.AgentRuntimeSettings{
			AgentId:       "new",
			MemoryEnabled: true,
		},
	}
	b := service.FromProtoCreate(req)
	_ = enabled
	if b.Settings == nil {
		t.Fatal("expected non-nil settings")
	}
	if !b.Settings.MemoryEnabled {
		t.Fatalf("settings mismatch: %+v", b.Settings)
	}
}

func TestFromProtoA2AProxy_OnlyRemoteURL(t *testing.T) {
	pb := &v1.A2AProxyConfig{
		RemoteUrl: "https://remote.test",
	}
	cfg := service.FromProtoA2AProxy(pb)
	if cfg == nil {
		t.Fatal("expected non-nil when RemoteURL is set")
	}
	if cfg.RemoteURL != "https://remote.test" {
		t.Fatalf("remote URL mismatch: %q", cfg.RemoteURL)
	}
}

func TestFromProtoA2AProxy_OnlyCardURL(t *testing.T) {
	pb := &v1.A2AProxyConfig{
		AgentCardUrl: "https://card.test",
	}
	cfg := service.FromProtoA2AProxy(pb)
	if cfg == nil {
		t.Fatal("expected non-nil when AgentCardURL is set")
	}
	if cfg.AgentCardURL != "https://card.test" {
		t.Fatalf("card URL mismatch: %q", cfg.AgentCardURL)
	}
}

func TestFromProtoAgent_WithA2AProxy(t *testing.T) {
	pb := &v1.Agent{
		Id:        "a1",
		AgentKey:  "proxy-agent",
		AgentKind: "a2a_proxy",
		A2AProxyConfig: &v1.A2AProxyConfig{
			RemoteUrl:      "https://remote.test",
			AgentCardUrl:   "https://card.test",
			EnableStreaming: true,
			AuthType:       "bearer",
			TimeoutSeconds: 15,
		},
	}
	b := service.FromProtoAgent(pb)
	if b.A2AProxy == nil {
		t.Fatal("expected non-nil A2AProxy")
	}
	if b.A2AProxy.RemoteURL != "https://remote.test" || b.A2AProxy.AgentCardURL != "https://card.test" {
		t.Fatalf("A2A proxy URLs mismatch: %+v", b.A2AProxy)
	}
	if !b.A2AProxy.EnableStreaming || b.A2AProxy.AuthType != "bearer" {
		t.Fatalf("A2A proxy streaming/auth mismatch: %+v", b.A2AProxy)
	}
	if b.A2AProxy.TimeoutSeconds != 15 {
		t.Fatalf("A2A proxy timeout mismatch: %d", b.A2AProxy.TimeoutSeconds)
	}
}

func TestBizEffectiveToolsToProto_SliceIsolation(t *testing.T) {
	in := biz.AgentEffectiveTools{
		Allow: []string{"a"},
		Deny:  []string{"b"},
	}
	pb := service.BizEffectiveToolsToProto(in)
	pb.Allow[0] = "mutated"
	if in.Allow[0] == "mutated" {
		t.Fatal("Allow slice should be isolated (copy), not shared")
	}
	pb.Deny[0] = "mutated"
	if in.Deny[0] == "mutated" {
		t.Fatal("Deny slice should be isolated (copy), not shared")
	}
}
