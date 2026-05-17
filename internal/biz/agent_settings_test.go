package biz_test

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestAgentRuntimeSettings_DomainAccessors(t *testing.T) {
	s := &biz.AgentRuntimeSettings{
		AgentID:                  "agent1",
		ChannelID:                "ch1",
		MemoryEnabled:            true,
		MemoryMaxChunkLength:     512,
		ToolsEnabled:             true,
		ToolsRetryEnabled:        true,
		ToolsRetryMaxAttempts:    3,
		SkillRuntimeJSON:         `{"allow":["*"]}`,
		IntentPassEnabled:        true,
		SelfEvolve:               true,
		EvolutionSkillEvolve:     true,
		ContextCompactionEnabled: true,
		PlannerKind:              "react",
	}

	// Identity domain.
	id := s.GetIdentity()
	if id.AgentID != "agent1" || id.ChannelID != "ch1" {
		t.Fatalf("Identity: got %+v", id)
	}

	// Memory domain.
	mem := s.GetMemory()
	if !mem.Enabled || mem.MaxChunkLength != 512 {
		t.Fatalf("Memory: got %+v", mem)
	}

	// Tools domain.
	tools := s.GetTools()
	if !tools.Enabled || !tools.RetryEnabled || tools.RetryMaxAttempts != 3 {
		t.Fatalf("Tools: got %+v", tools)
	}

	// Skills domain.
	skills := s.GetSkills()
	if !skills.IntentPassEnabled || skills.RuntimeJSON == "" {
		t.Fatalf("Skills: got %+v", skills)
	}

	// Evolution domain.
	evo := s.GetEvolution()
	if !evo.SelfEvolve || !evo.SkillEvolve {
		t.Fatalf("Evolution: got %+v", evo)
	}

	// Context domain.
	ctx := s.GetContext()
	if !ctx.CompactionEnabled || ctx.PlannerKind != "react" {
		t.Fatalf("Context: got %+v", ctx)
	}
}

func TestAgentRuntimeSettings_ZeroValue(t *testing.T) {
	var s biz.AgentRuntimeSettings
	id := s.GetIdentity()
	if id.AgentID != "" {
		t.Fatalf("expected empty AgentID")
	}
	if s.GetMemory().Enabled {
		t.Fatalf("expected memory disabled by default")
	}
}
