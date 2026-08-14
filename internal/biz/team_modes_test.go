package biz

import (
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestTeamOrchestrationModesAccepted(t *testing.T) {
	modes := []string{
		"sequential",
		"parallel",
		"coordinator",
		"critic_loop",
		"swarm",
		"adaptive",
	}
	for _, mode := range modes {
		t.Run(mode, func(t *testing.T) {
			raw := `{"version":1,"mode":"` + mode + `","members":[]}`
			if err := validateTeamDefinition(raw); err != nil {
				t.Fatalf("mode %q: %v", mode, err)
			}
		})
	}
}

func TestTeamOrchestrationModeRejected(t *testing.T) {
	err := validateTeamDefinition(`{"version":1,"mode":"invalid","members":[]}`)
	if err == nil {
		t.Fatal("expected error for invalid mode")
	}
	ae, ok := apierror.From(err)
	if !ok {
		t.Fatalf("expected apierror, got %T", err)
	}
	if ae.Domain != "TEAM" {
		t.Fatalf("unexpected domain: %v", ae.Domain)
	}
}

func TestTeamOrchestrationModeRejectsEngineAndSourceNames(t *testing.T) {
	// graph/native are runtime_engine; custom/preset are graph source — not modes.
	for _, mode := range []string{"graph", "native", "custom", "preset"} {
		t.Run(mode, func(t *testing.T) {
			err := validateTeamDefinition(`{"version":1,"mode":"` + mode + `","members":[]}`)
			if err == nil {
				t.Fatalf("expected error for non-mode value %q", mode)
			}
		})
	}
}

func TestTeamCriticLoopRequiresRoles(t *testing.T) {
	err := validateTeamDefinition(`{"version":1,"mode":"critic_loop","members":[{"agent_id":"a1","role":"generator","enabled":true}]}`)
	if err == nil {
		t.Fatal("expected critic_loop role validation error")
	}
}

func TestTeamParallelRequiresSynthesizer(t *testing.T) {
	err := validateTeamDefinition(`{"version":1,"mode":"parallel","members":[{"agent_id":"a1","role":"worker","enabled":true},{"agent_id":"a2","role":"worker","enabled":true}]}`)
	if err == nil {
		t.Fatal("expected parallel synthesizer validation error")
	}
}

func TestTeamSwarmMembersOptional(t *testing.T) {
	raw := `{"version":1,"mode":"swarm","members":[{"agent_id":"a1","enabled":true}]}`
	if err := validateTeamDefinition(raw); err != nil {
		t.Fatalf("swarm with member: %v", err)
	}
}

func TestTeamSequentialMultipleMembersAccepted(t *testing.T) {
	raw := `{"version":1,"mode":"sequential","members":[{"agent_id":"a1","enabled":true},{"agent_id":"a2","enabled":true}]}`
	if err := validateTeamDefinition(raw); err != nil {
		t.Fatalf("sequential: %v", err)
	}
}

func TestTeamAdaptiveModeAccepted(t *testing.T) {
	raw := `{"version":1,"mode":"adaptive","members":[{"agent_id":"a1","enabled":true}]}`
	if err := validateTeamDefinition(raw); err != nil {
		t.Fatalf("adaptive: %v", err)
	}
}
