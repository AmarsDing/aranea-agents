package session

import (
	"strings"
	"testing"
)

func TestValidateDepth_SpiritAllowsChild(t *testing.T) {
	parent := Session{AgentDepth: 0} // spirit session
	if err := ValidateDepth(parent, 1, DepthValidationConfig{SpiritMaxDepth: 2}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDepth_SpiritRejectsExceedsMax(t *testing.T) {
	parent := Session{AgentDepth: 2}
	err := ValidateDepth(parent, 3, DepthValidationConfig{SpiritMaxDepth: 2})
	if err == nil {
		t.Fatal("expected error when childDepth > SpiritMaxDepth")
	}
	if !strings.Contains(err.Error(), "spirit max") {
		t.Fatalf("expected spirit max error, got %v", err)
	}
}

func TestValidateDepth_AgentRelativeDepthOK(t *testing.T) {
	// Parent agent session at depth 2, agent max relative depth = 1.
	// Child at depth 3 has relative depth 1 (3-2=1), within limit.
	parent := Session{AgentDepth: 2, MemberAgentKey: "agent-1"}
	if err := ValidateDepth(parent, 3, DepthValidationConfig{
		SpiritMaxDepth:        4,
		AgentMaxRelativeDepth: 1,
	}); err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}

func TestValidateDepth_AgentRelativeDepthExceeded(t *testing.T) {
	// Parent agent at depth 2, agent max relative depth = 1.
	// Child at depth 4 has relative depth 2 (4-2=2), exceeds limit.
	parent := Session{AgentDepth: 2, MemberAgentKey: "agent-1"}
	err := ValidateDepth(parent, 4, DepthValidationConfig{
		SpiritMaxDepth:        5,
		AgentMaxRelativeDepth: 1,
	})
	if err == nil {
		t.Fatal("expected error when relative depth exceeds agent max")
	}
	if !strings.Contains(err.Error(), "agent max") {
		t.Fatalf("expected agent max error, got %v", err)
	}
}

func TestValidateDepth_NonAgentParentSkipsRelativeCheck(t *testing.T) {
	// Team session (no MemberAgentKey) — relative depth check skipped.
	// Even if AgentMaxRelativeDepth is 0, team session creation succeeds.
	parent := Session{AgentDepth: 1} // team session, no MemberAgentKey
	if err := ValidateDepth(parent, 2, DepthValidationConfig{
		SpiritMaxDepth:        3,
		AgentMaxRelativeDepth: 0,
	}); err != nil {
		t.Fatalf("expected nil for non-agent parent, got %v", err)
	}
}

func TestValidateDepth_AgentMaxRelativeZeroBlocksSubAgent(t *testing.T) {
	// Agent parent with SubagentsMaxGenerationDepth=0 means no sub-agents.
	// But the check is skipped when AgentMaxRelativeDepth=0 (legacy default),
	// so child creation is allowed (no agent-level enforcement).
	// This preserves backward compat with agents that have no subagent config.
	parent := Session{AgentDepth: 2, MemberAgentKey: "agent-1"}
	if err := ValidateDepth(parent, 3, DepthValidationConfig{
		SpiritMaxDepth:        4,
		AgentMaxRelativeDepth: 0,
	}); err != nil {
		t.Fatalf("expected nil when AgentMaxRelativeDepth=0 (disabled), got %v", err)
	}
}
