package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestApplyIdentity(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	cfg := biz.IdentityCfg{
		AgentID:               "agent-1",
		ChannelID:             "ch-1",
		ChatID:                "chat-1",
		Workspace:             "ws-1",
		VariablesJSON:         `{"k":"v"}`,
		ModelInstructionsJSON: `{"gpt-4o":"instruct"}`,
	}
	s.ApplyIdentity(cfg)
	got := s.GetIdentity()
	if got.AgentID != "agent-1" {
		t.Errorf("AgentID = %q, want %q", got.AgentID, "agent-1")
	}
	if got.ChannelID != "ch-1" {
		t.Errorf("ChannelID = %q, want %q", got.ChannelID, "ch-1")
	}
	if got.ChatID != "chat-1" {
		t.Errorf("ChatID = %q, want %q", got.ChatID, "chat-1")
	}
	if got.Workspace != "ws-1" {
		t.Errorf("Workspace = %q, want %q", got.Workspace, "ws-1")
	}
	if got.VariablesJSON != `{"k":"v"}` {
		t.Errorf("VariablesJSON = %q, want %q", got.VariablesJSON, `{"k":"v"}`)
	}
	if got.ModelInstructionsJSON != `{"gpt-4o":"instruct"}` {
		t.Errorf("ModelInstructionsJSON = %q, want %q", got.ModelInstructionsJSON, `{"gpt-4o":"instruct"}`)
	}
}

func TestApplyReasoning(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	cfg := biz.ReasoningCfg{Mode: "chain_of_thought", Level: "high"}
	s.ApplyReasoning(cfg)
	got := s.GetReasoning()
	if got.Mode != "chain_of_thought" {
		t.Errorf("Mode = %q, want %q", got.Mode, "chain_of_thought")
	}
	if got.Level != "high" {
		t.Errorf("Level = %q, want %q", got.Level, "high")
	}
}

func TestApplyMemory(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	cfg := biz.MemoryCfg{
		Enabled:                  true,
		MaxChunkLength:           512,
		MaxResults:               10,
		MinScore:                 0.75,
		HeartbeatEnabled:         true,
		HeartbeatIntervalMinutes: 5,
		L0RecentWindowTurns:      20,
		L0RecentWindowTokens:     4096,
		L0SummaryThreshold:       0.8,
		L0SummaryKeepTurns:       4,
		L0CompressMinGapSec:      300,
		L0CompressProvider:       "openai",
		L0CompressModel:          "gpt-4o-mini",
		MemoryWorkerProvider:     "anthropic",
		MemoryWorkerModel:        "claude-3",
		L0TruncateStrategy:       "rolling",
		L0InjectL1:               true,
		L0InjectL3:               true,
		L0InjectL4:               false,
		L0L3MaxChunks:            5,
		L0L4MaxPaths:             3,
		L0SnapshotMode:           "full",
		L1Enabled:                true,
		L1BudgetTokens:           2000,
		L1FieldMaxTokens:         500,
		L1HistoryKeepRevisions:   3,
		L1DefaultSchemaID:        "schema-1",
		L1ArchiveOnIdleMinutes:   60,
		L2EpisodeEnabled:         true,
		L2EpisodeMinImportance:   0.5,
		L2IndexEnabled:           true,
		L2IndexEmbeddingModel:    "text-embedding-3",
		L2RecallEnabled:          true,
		L2RecallMax:              8,
		L2RetentionDays:          90,
		L2ArchiveAfterDays:       180,
		L3Enabled:                true,
		L3RecallTopK:             5,
		L3RecallMinScore:         0.6,
		L3RecallScopesJSON:       `["global"]`,
		L3EmbeddingModel:         "text-embedding-3-small",
		L3DecayIntervalHours:     24,
		L3ArchiveThreshold:       0.3,
		L3MaxPerRecallChars:      2000,
		L4Enabled:                true,
		L4GraphInjectNeighbors:   true,
		L4GraphMaxNeighbors:      10,
		L4GraphMaxHops:           3,
		L4IdentityInject:         true,
		L4StrategyInject:         false,
		L4DecayIntervalHours:     48,
		L4DecayOverridesJSON:     `{"personality":720}`,
	}
	s.ApplyMemory(cfg)
	got := s.GetMemory()
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
	if got.MaxChunkLength != 512 {
		t.Errorf("MaxChunkLength = %d, want 512", got.MaxChunkLength)
	}
	if got.L0CompressProvider != "openai" {
		t.Errorf("L0CompressProvider = %q, want %q", got.L0CompressProvider, "openai")
	}
	if got.L4DecayOverridesJSON != `{"personality":720}` {
		t.Errorf("L4DecayOverridesJSON = %q, want %q", got.L4DecayOverridesJSON, `{"personality":720}`)
	}
	if !got.L0InjectL1 {
		t.Error("L0InjectL1 = false, want true")
	}
	if got.L0InjectL4 {
		t.Error("L0InjectL4 = true, want false")
	}
}

func TestApplyTools(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	cfg := biz.ToolsCfg{
		Enabled:                true,
		Profile:                "full",
		ToolCallPrefix:         "tool_",
		AllowJSON:              `["search"]`,
		DenyJSON:               `["delete"]`,
		ConcurrentAllowJSON:    `["search","calc"]`,
		RetryEnabled:           true,
		RetryMaxAttempts:       3,
		RetryInitialIntervalMs: 500,
		RetryBackoffFactor:     2.0,
		RetryMaxIntervalMs:     10000,
		RetryJitter:            true,
		ParallelEnabled:        true,
		StreamingEnabled:       true,
	}
	s.ApplyTools(cfg)
	got := s.GetTools()
	if !got.Enabled {
		t.Error("Enabled = false, want true")
	}
	if got.Profile != "full" {
		t.Errorf("Profile = %q, want %q", got.Profile, "full")
	}
	if got.RetryMaxAttempts != 3 {
		t.Errorf("RetryMaxAttempts = %d, want 3", got.RetryMaxAttempts)
	}
	if !got.StreamingEnabled {
		t.Error("StreamingEnabled = false, want true")
	}
}

func TestApplySkills(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	cfg := biz.SkillsCfg{
		RuntimeJSON:       `{"whitelist":["s1"]}`,
		LoadMode:          "auto",
		IntentPassEnabled: true,
	}
	s.ApplySkills(cfg)
	got := s.GetSkills()
	if got.RuntimeJSON != `{"whitelist":["s1"]}` {
		t.Errorf("RuntimeJSON = %q, want %q", got.RuntimeJSON, `{"whitelist":["s1"]}`)
	}
	if got.LoadMode != "auto" {
		t.Errorf("LoadMode = %q, want %q", got.LoadMode, "auto")
	}
	if !got.IntentPassEnabled {
		t.Error("IntentPassEnabled = false, want true")
	}
}

func TestApplyEvolution(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	cfg := biz.EvolutionCfg{
		SelfEvolve:                        true,
		SubagentsEnabled:                  true,
		SubagentsMaxConcurrency:           4,
		SubagentsMaxGenerationDepth:       3,
		SubagentsMaxChildrenPerAgent:      5,
		SubagentsArchiveAfterMinutes:      120,
		SubagentsMaxRetries:               2,
		SubagentsModelOverride:            "gpt-4o",
		SkillEvolve:                       true,
		MetricsEnabled:                    true,
		SuggestionsEnabled:                true,
		GuardrailMaxChangePerPeriod:       0.3,
		GuardrailMinDataPoints:            10,
		GuardrailRollbackOnDeclinePercent: 80,
		EvoEnabled:                        true,
		EvoAutoApply:                      false,
		EvoMinEpisodes:                    50,
		EvoMinNegativeFeedback:            5,
		EvoThrottleHours:                  24,
		EvoProposalTTLDays:                7,
		EvoPersonaMaxChars:                2000,
		EvoSystemPromptMaxAppends:         3,
	}
	s.ApplyEvolution(cfg)
	got := s.GetEvolution()
	if !got.SelfEvolve {
		t.Error("SelfEvolve = false, want true")
	}
	if got.SubagentsMaxConcurrency != 4 {
		t.Errorf("SubagentsMaxConcurrency = %d, want 4", got.SubagentsMaxConcurrency)
	}
	if got.GuardrailMaxChangePerPeriod != 0.3 {
		t.Errorf("GuardrailMaxChangePerPeriod = %f, want 0.3", got.GuardrailMaxChangePerPeriod)
	}
	if got.EvoPersonaMaxChars != 2000 {
		t.Errorf("EvoPersonaMaxChars = %d, want 2000", got.EvoPersonaMaxChars)
	}
	if got.EvoAutoApply {
		t.Error("EvoAutoApply = true, want false")
	}
}

func TestApplyContext(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	cfg := biz.ContextCfg{
		CompactionEnabled:     true,
		SessionSummaryEnabled: true,
		OutputSchemaJSON:      `{"type":"object"}`,
		ModelSelector:         "auto",
		PlannerKind:           "builtin",
		PlannerConfigJSON:     `{"max_steps":5}`,
	}
	s.ApplyContext(cfg)
	got := s.GetContext()
	if !got.CompactionEnabled {
		t.Error("CompactionEnabled = false, want true")
	}
	if got.ModelSelector != "auto" {
		t.Errorf("ModelSelector = %q, want %q", got.ModelSelector, "auto")
	}
	if got.PlannerKind != "builtin" {
		t.Errorf("PlannerKind = %q, want %q", got.PlannerKind, "builtin")
	}
}

func TestGetCodeExecutor_NilReceiver(t *testing.T) {
	var s *biz.AgentRuntimeSettings
	got := s.GetCodeExecutor()
	if got.Type != "local" {
		t.Errorf("nil receiver Type = %q, want %q", got.Type, "local")
	}
}

func TestGetCodeExecutor_EmptyType(t *testing.T) {
	s := &biz.AgentRuntimeSettings{CodeExecutorType: ""}
	got := s.GetCodeExecutor()
	if got.Type != "local" {
		t.Errorf("empty Type = %q, want %q", got.Type, "local")
	}
}

func TestGetCodeExecutor_ExplicitType(t *testing.T) {
	s := &biz.AgentRuntimeSettings{CodeExecutorType: "docker"}
	got := s.GetCodeExecutor()
	if got.Type != "docker" {
		t.Errorf("Type = %q, want %q", got.Type, "docker")
	}
}

func TestGetCodeExecutor_WhitespaceType(t *testing.T) {
	s := &biz.AgentRuntimeSettings{CodeExecutorType: "  e2b  "}
	got := s.GetCodeExecutor()
	if got.Type != "e2b" {
		t.Errorf("Type = %q, want %q", got.Type, "e2b")
	}
}

func TestApplyGetRoundTrip(t *testing.T) {
	s := &biz.AgentRuntimeSettings{}
	identity := biz.IdentityCfg{AgentID: "a", ChannelID: "c", ChatID: "h", Workspace: "w", VariablesJSON: "{}", ModelInstructionsJSON: "{}"}
	s.ApplyIdentity(identity)
	if got := s.GetIdentity(); got != identity {
		t.Errorf("Identity round-trip: got %+v, want %+v", got, identity)
	}

	reasoning := biz.ReasoningCfg{Mode: "cot", Level: "max"}
	s.ApplyReasoning(reasoning)
	if got := s.GetReasoning(); got != reasoning {
		t.Errorf("Reasoning round-trip: got %+v, want %+v", got, reasoning)
	}

	skills := biz.SkillsCfg{RuntimeJSON: "rj", LoadMode: "manual", IntentPassEnabled: true}
	s.ApplySkills(skills)
	if got := s.GetSkills(); got != skills {
		t.Errorf("Skills round-trip: got %+v, want %+v", got, skills)
	}

	context := biz.ContextCfg{CompactionEnabled: true, SessionSummaryEnabled: false, OutputSchemaJSON: "sj", ModelSelector: "ms", PlannerKind: "pk", PlannerConfigJSON: "pc"}
	s.ApplyContext(context)
	if got := s.GetContext(); got != context {
		t.Errorf("Context round-trip: got %+v, want %+v", got, context)
	}
}

func TestGetSkillRuntimeJSON(t *testing.T) {
	s := &biz.AgentRuntimeSettings{SkillRuntimeJSON: `{"test":true}`}
	if got := s.GetSkillRuntimeJSON(); got != `{"test":true}` {
		t.Errorf("GetSkillRuntimeJSON = %q, want %q", got, `{"test":true}`)
	}
}
