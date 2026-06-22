package data

import (
	"reflect"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func TestEntRuntimeToBiz_Nil(t *testing.T) {
	got := entRuntimeToBiz(nil)
	if got != (biz.AgentRuntimeSettings{}) {
		t.Fatalf("expected zero value for nil input, got %+v", got)
	}
}

func TestEntRuntimeToBiz_FullRoundTrip(t *testing.T) {
	e := &ent.AgentRuntimeSetting{
		ID:                                "agent-1",
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
		L0CompressMinGapSec:               5,
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
		L4DecayIntervalHours:              12,
		L4DecayOverridesJSON:              `{"factor":0.5}`,
		ToolsEnabled:                      true,
		ToolsProfile:                      "coding",
		ToolsToolCallPrefix:               "/",
		ToolsAllowJSON:                    `["*"]`,
		ToolsDenyJSON:                     `[]`,
		ToolsConcurrentAllowJSON:          `["search"]`,
		ToolsRetryEnabled:                 true,
		ToolsRetryMaxAttempts:             3,
		ToolsRetryInitialIntervalMs:       1000,
		ToolsRetryBackoffFactor:           2.0,
		ToolsRetryMaxIntervalMs:           10000,
		ToolsRetryJitter:                  true,
		ToolsParallelEnabled:              true,
		ToolsStreamingEnabled:             false,
		ToolsCircuitBreakerEnabled:        true,
		ToolsCircuitBreakerOverridesJSON:  `{"t1":{"threshold":5}}`,
		ToolsDeferredJSON:                 `["web_search"]`,
		ToolsCommandSafetyEnabled:         true,
		SkillRuntimeJSON:                  `{"allow":["*"]}`,
		SkillLoadMode:                     "auto",
		IntentPassEnabled:                 true,
		SelfEvolve:                        true,
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
		MicroCompactEnabled:               true,
		MemoryCompactEnabled:              false,
		ToolResultGateEnabled:             true,
		CompressLlmCacheEnabled:           true,
		CompressLlmCacheMaxEntries:        256,
		CompressLlmCacheTTLSec:            600,
		CompressionBufferRatio:            0.15,
		SoftTriggerRatio:                  0.70,
		HardTriggerRatio:                  0.90,
		SessionSummaryEnabled:             true,
		OutputSchemaJSON:                  `{"type":"object"}`,
		ModelSelector:                     "auto",
		PlannerKind:                       "react",
		PlannerConfigJSON:                 `{"max_steps":5}`,
		CodeExecutorType:                  "docker",
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

	got := entRuntimeToBiz(e)

	// Identity
	id := got.GetIdentity()
	if id.AgentID != "agent-1" || id.ChannelID != "ch1" || id.ChatID != "chat1" {
		t.Fatalf("identity mismatch: %+v", id)
	}
	if id.Workspace != "ws1" || id.VariablesJSON != `{"k":"v"}` || id.ModelInstructionsJSON != `{"gpt-4o":"inst"}` {
		t.Fatalf("identity workspace/vars mismatch: %+v", id)
	}

	// Reasoning
	rsn := got.GetReasoning()
	if rsn.Mode != "step" || rsn.Level != "high" {
		t.Fatalf("reasoning mismatch: %+v", rsn)
	}

	// Memory
	mem := got.GetMemory()
	if !mem.Enabled || mem.MaxChunkLength != 512 || mem.MaxResults != 8 {
		t.Fatalf("memory mismatch: %+v", mem)
	}
	if mem.MinScore != 0.4 || !mem.HeartbeatEnabled || mem.HeartbeatIntervalMinutes != 15 {
		t.Fatalf("memory heartbeat mismatch: %+v", mem)
	}
	if mem.L0RecentWindowTurns != 10 || mem.L0RecentWindowTokens != 2000 {
		t.Fatalf("memory L0 window mismatch: %+v", mem)
	}
	if mem.L0CompressMinGapSec != 5 {
		t.Fatalf("memory L0 compress min gap mismatch: got %d", mem.L0CompressMinGapSec)
	}
	if mem.L0CompressProvider != "openrouter" || mem.L0CompressModel != "gpt-4o-mini" {
		t.Fatalf("memory L0 compress mismatch: %+v", mem)
	}
	if mem.MemoryWorkerProvider != "openai" || mem.MemoryWorkerModel != "gpt-4o" {
		t.Fatalf("memory worker mismatch: %+v", mem)
	}
	if !mem.L1Enabled || mem.L1BudgetTokens != 8000 {
		t.Fatalf("memory L1 mismatch: %+v", mem)
	}
	if !mem.L2EpisodeEnabled || mem.L2EpisodeMinImportance != 0.7 {
		t.Fatalf("memory L2 episode mismatch: %+v", mem)
	}
	if !mem.L3Enabled || mem.L3RecallTopK != 5 {
		t.Fatalf("memory L3 mismatch: %+v", mem)
	}
	if mem.L3RecallScopesJSON != `["global"]` {
		t.Fatalf("memory L3 scopes mismatch: got %q", mem.L3RecallScopesJSON)
	}
	if !mem.L4Enabled || !mem.L4GraphInjectNeighbors {
		t.Fatalf("memory L4 mismatch: %+v", mem)
	}
	if mem.L4DecayIntervalHours != 12 {
		t.Fatalf("memory L4 decay interval mismatch: got %d", mem.L4DecayIntervalHours)
	}
	if mem.L4DecayOverridesJSON != `{"factor":0.5}` {
		t.Fatalf("memory L4 decay overrides mismatch: got %q", mem.L4DecayOverridesJSON)
	}

	// Tools
	tools := got.GetTools()
	if !tools.Enabled || tools.Profile != "coding" {
		t.Fatalf("tools mismatch: %+v", tools)
	}
	if tools.AllowJSON != `["*"]` || tools.DenyJSON != `[]` {
		t.Fatalf("tools allow/deny mismatch: %+v", tools)
	}
	if !tools.RetryEnabled || tools.RetryMaxAttempts != 3 {
		t.Fatalf("tools retry mismatch: %+v", tools)
	}
	if !tools.CircuitBreakerEnabled || tools.CircuitBreakerOverridesJSON != `{"t1":{"threshold":5}}` {
		t.Fatalf("tools circuit breaker mismatch: %+v", tools)
	}
	if tools.DeferredJSON != `["web_search"]` {
		t.Fatalf("tools deferred mismatch: got %q", tools.DeferredJSON)
	}
	if !tools.CommandSafetyEnabled {
		t.Fatalf("tools command safety mismatch: %+v", tools)
	}

	// Skills
	skills := got.GetSkills()
	if skills.RuntimeJSON != `{"allow":["*"]}` || !skills.IntentPassEnabled || skills.LoadMode != "auto" {
		t.Fatalf("skills mismatch: %+v", skills)
	}

	// Evolution
	evo := got.GetEvolution()
	if !evo.SelfEvolve || !evo.SubagentsEnabled || evo.SubagentsMaxConcurrency != 5 {
		t.Fatalf("evolution mismatch: %+v", evo)
	}
	if !evo.EvoEnabled || evo.EvoAutoApply || evo.EvoMinEpisodes != 20 {
		t.Fatalf("evolution evo mismatch: %+v", evo)
	}
	if evo.EvoProposalTTLDays != 7 {
		t.Fatalf("evolution evo proposal TTL mismatch: got %d", evo.EvoProposalTTLDays)
	}

	// Context
	ctx := got.GetContext()
	if !ctx.CompactionEnabled || !ctx.SessionSummaryEnabled {
		t.Fatalf("context mismatch: %+v", ctx)
	}
	if ctx.OutputSchemaJSON != `{"type":"object"}` || ctx.ModelSelector != "auto" {
		t.Fatalf("context detail mismatch: %+v", ctx)
	}
	if ctx.PlannerKind != "react" || ctx.PlannerConfigJSON != `{"max_steps":5}` {
		t.Fatalf("context planner mismatch: %+v", ctx)
	}
	if !ctx.MicroCompactEnabled || ctx.MemoryCompactEnabled {
		t.Fatalf("context compact flags mismatch: %+v", ctx)
	}
	if !ctx.ToolResultGateEnabled {
		t.Fatalf("context tool result gate mismatch: %+v", ctx)
	}
	if !ctx.CompressLLMCacheEnabled || ctx.CompressLLMCacheMaxEntries != 256 || ctx.CompressLLMCacheTTLSec != 600 {
		t.Fatalf("context compress cache mismatch: %+v", ctx)
	}

	// Direct fields
	if got.CodeExecutorType != "docker" {
		t.Fatalf("code_executor_type mismatch: got %q, want %q", got.CodeExecutorType, "docker")
	}
	if got.RalphLoopMaxIterations != 10 || got.RalphLoopCompletionPromise != "done" {
		t.Fatalf("ralph loop mismatch: %+v", got)
	}
	if got.RalphLoopVerifyCommand != "test" || got.RalphLoopVerifyTimeoutSeconds != 30 {
		t.Fatalf("ralph loop verify mismatch: %+v", got)
	}
	if got.RalphLoopPromiseTagOpen != "<promise>" || got.RalphLoopPromiseTagClose != "</promise>" {
		t.Fatalf("ralph loop tags mismatch: %+v", got)
	}
	if got.RalphLoopVerifyWorkDir != "/tmp" {
		t.Fatalf("ralph loop workdir mismatch: got %q", got.RalphLoopVerifyWorkDir)
	}
}

func TestEntAgentToBiz_Nil(t *testing.T) {
	lg := loggateway.NewNoop()
	got := entAgentToBiz(nil, lg)
	want := biz.Agent{}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("expected zero value for nil input, got %+v", got)
	}
}

func TestEntAgentToBiz_BasicFields(t *testing.T) {
	lg := loggateway.NewNoop()
	a := &ent.Agent{
		ID:                 "a1",
		AgentKey:           "test-agent",
		DisplayName:        "Test Agent",
		Provider:           "openai",
		Model:              "gpt-4o",
		Status:             "active",
		IsDefault:          true,
		IsFavorite:         false,
		Icon:               "robot",
		AgentDescription:   "A test agent",
		PositionID:         "pos1",
		PositionKey:        "pk1",
		AgentVariant:       "v1",
		VariantDescription: "variant desc",
		SystemPromptMode:   "complete",
		ContextWindow:      128,
		BudgetMonthlyCents: 1000,
		ConfigJSON:         `{"kind":"llm"}`,
		CreatedBy:          "admin",
		Readonly:           false,
		Kind:               "user",
		Source:             "user",
		CreatedAt:          "2024-01-01",
		UpdatedAt:          "2024-06-01",
		DeletedAt:          "",
		RolesJSON:          `["admin"]`,
	}

	got := entAgentToBiz(a, lg)
	biz.HydrateAgentKind(&got)
	if got.ID != "a1" || got.AgentKey != "test-agent" {
		t.Fatalf("agent id/key mismatch: %+v", got)
	}
	if got.DisplayName != "Test Agent" || got.Provider != "openai" || got.Model != "gpt-4o" {
		t.Fatalf("agent display/provider/model mismatch: %+v", got)
	}
	if got.Status != "active" || !biz.BoolVal(got.IsDefault) || biz.BoolVal(got.IsFavorite) {
		t.Fatalf("agent status/favorite mismatch: %+v", got)
	}
	if got.PositionID != "pos1" || got.PositionKey != "pk1" {
		t.Fatalf("agent taxonomy mismatch: %+v", got)
	}
	if got.Readonly {
		t.Fatalf("agent readonly mismatch: got %v", got.Readonly)
	}
	if len(got.Roles) != 1 || got.Roles[0] != "admin" {
		t.Fatalf("agent roles mismatch: got %v", got.Roles)
	}
	if got.Source != "user" {
		t.Fatalf("agent source mismatch: got %q", got.Source)
	}
	if got.Kind != "user" {
		t.Fatalf("agent kind mismatch: got %q", got.Kind)
	}
	if got.AgentKind != "llm" {
		t.Fatalf("agent agent_kind mismatch: got %q", got.AgentKind)
	}
}

func TestFromEntIdentity(t *testing.T) {
	e := &ent.AgentRuntimeSetting{
		ID:                    "a1",
		ChannelID:             "ch1",
		ChatID:                "chat1",
		Workspace:             "ws1",
		VariablesJSON:         `{"x":1}`,
		ModelInstructionsJSON: `{"gpt-4o":"be concise"}`,
	}
	got := fromEntIdentity(e)
	if got.AgentID != "a1" || got.ChannelID != "ch1" {
		t.Fatalf("identity agent/channel mismatch: %+v", got)
	}
	if got.VariablesJSON != `{"x":1}` || got.ModelInstructionsJSON != `{"gpt-4o":"be concise"}` {
		t.Fatalf("identity json mismatch: %+v", got)
	}
}

func TestFromEntReasoning(t *testing.T) {
	e := &ent.AgentRuntimeSetting{ReasoningMode: "step", ReasoningLevel: "high"}
	got := fromEntReasoning(e)
	if got.Mode != "step" || got.Level != "high" {
		t.Fatalf("reasoning mismatch: %+v", got)
	}
}

func TestFromEntSkills(t *testing.T) {
	e := &ent.AgentRuntimeSetting{
		SkillRuntimeJSON:  `{"allow":["*"]}`,
		SkillLoadMode:     "progressive",
		IntentPassEnabled: true,
	}
	got := fromEntSkills(e)
	if got.RuntimeJSON != `{"allow":["*"]}` || got.LoadMode != "progressive" || !got.IntentPassEnabled {
		t.Fatalf("skills mismatch: %+v", got)
	}
}

func TestFromEntContext(t *testing.T) {
	e := &ent.AgentRuntimeSetting{
		ContextCompactionEnabled:   true,
		MicroCompactEnabled:        true,
		MemoryCompactEnabled:       false,
		ToolResultGateEnabled:      true,
		CompressLlmCacheEnabled:    true,
		CompressLlmCacheMaxEntries: 128,
		CompressLlmCacheTTLSec:     300,
		CompressionBufferRatio:     0.20,
		SoftTriggerRatio:           0.75,
		HardTriggerRatio:           0.95,
		SessionSummaryEnabled:      true,
		OutputSchemaJSON:           `{"type":"object"}`,
		ModelSelector:              "auto",
		PlannerKind:                "react",
		PlannerConfigJSON:          `{"max_steps":5}`,
	}
	got := fromEntContext(e)
	if !got.CompactionEnabled || !got.MicroCompactEnabled || got.MemoryCompactEnabled {
		t.Fatalf("context compact flags mismatch: %+v", got)
	}
	if !got.CompressLLMCacheEnabled || got.CompressLLMCacheMaxEntries != 128 || got.CompressLLMCacheTTLSec != 300 {
		t.Fatalf("context compress cache mismatch: %+v", got)
	}
	if got.CompressionBufferRatio != 0.20 || got.SoftTriggerRatio != 0.75 || got.HardTriggerRatio != 0.95 {
		t.Fatalf("context compression ratios mismatch: %+v", got)
	}
	if got.PlannerKind != "react" || got.PlannerConfigJSON != `{"max_steps":5}` {
		t.Fatalf("context planner mismatch: %+v", got)
	}
}
