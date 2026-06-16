package reliability

import "testing"

func TestClassifier_Classify(t *testing.T) {
	c := NewClassifier[string]()

	// Register Critical events
	c.RegisterBulk(Critical, "tool_result", "error", "runner_completion", "checkpoint")

	// Register Important events
	c.RegisterBulk(Important, "state_delta", "token_usage", "run_status", "session_status_changed")

	// Informational is the default — no need to register

	tests := []struct {
		eventType string
		want      Tier
	}{
		{"tool_result", Critical},
		{"error", Critical},
		{"runner_completion", Critical},
		{"checkpoint", Critical},
		{"state_delta", Important},
		{"token_usage", Important},
		{"run_status", Important},
		{"session_status_changed", Important},
		{"text_delta", Informational}, // unregistered => fallback
		{"flow_log", Informational},   // unregistered => fallback
		{"unknown", Informational},    // unregistered => fallback
	}

	for _, tt := range tests {
		got := c.Classify(tt.eventType)
		if got != tt.want {
			t.Errorf("Classify(%q) = %v, want %v", tt.eventType, got, tt.want)
		}
	}
}

func TestClassifier_RequiresBlockUpTo(t *testing.T) {
	tests := []struct {
		tier Tier
		want bool
	}{
		{Critical, true},
		{Important, true},
		{Informational, false},
	}
	for _, tt := range tests {
		got := RequiresBlockUpTo(tt.tier)
		if got != tt.want {
			t.Errorf("RequiresBlockUpTo(%v) = %v, want %v", tt.tier, got, tt.want)
		}
	}
}

func TestClassifier_IsCriticalWBPF(t *testing.T) {
	tests := []struct {
		tier Tier
		want bool
	}{
		{Critical, true},
		{Important, false},
		{Informational, false},
	}
	for _, tt := range tests {
		got := IsCriticalWBPF(tt.tier)
		if got != tt.want {
			t.Errorf("IsCriticalWBPF(%v) = %v, want %v", tt.tier, got, tt.want)
		}
	}
}

func TestClassifier_TierString(t *testing.T) {
	tests := []struct {
		tier Tier
		want string
	}{
		{Critical, "critical"},
		{Important, "important"},
		{Informational, "informational"},
		{Tier(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.tier.String()
		if got != tt.want {
			t.Errorf("Tier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestClassifier_IsRegistered(t *testing.T) {
	c := NewClassifier[string]()
	c.Register("tool_result", Critical)

	if !c.IsRegistered("tool_result") {
		t.Error("IsRegistered(tool_result) = false, want true")
	}
	if c.IsRegistered("unknown") {
		t.Error("IsRegistered(unknown) = true, want false")
	}
}

func TestClassifier_Tiers(t *testing.T) {
	c := NewClassifier[string]()
	c.RegisterBulk(Critical, "tool_result", "error")
	c.RegisterBulk(Important, "state_delta")

	criticalTypes := c.Tiers(Critical)
	if len(criticalTypes) != 2 {
		t.Errorf("Tiers(Critical) returned %d items, want 2", len(criticalTypes))
	}

	importantTypes := c.Tiers(Important)
	if len(importantTypes) != 1 {
		t.Errorf("Tiers(Important) returned %d items, want 1", len(importantTypes))
	}

	infoTypes := c.Tiers(Informational)
	if len(infoTypes) != 0 {
		t.Errorf("Tiers(Informational) returned %d items, want 0", len(infoTypes))
	}
}

func TestClassifier_CustomFallback(t *testing.T) {
	c := NewClassifier[string](Important) // default to Important
	got := c.Classify("unknown")
	if got != Important {
		t.Errorf("Classify(unknown) with Important fallback = %v, want Important", got)
	}
}

func TestClassifier_IntKeyType(t *testing.T) {
	c := NewClassifier[int]()
	c.Register(1, Critical)
	c.Register(2, Important)

	if c.Classify(1) != Critical {
		t.Error("Classify(1) != Critical")
	}
	if c.Classify(2) != Important {
		t.Error("Classify(2) != Important")
	}
	if c.Classify(99) != Informational {
		t.Error("Classify(99) != Informational")
	}
}
