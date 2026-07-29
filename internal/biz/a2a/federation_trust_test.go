package a2a

import (
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestTrustManager_Check(t *testing.T) {
	m := NewTrustManager(loggateway.NewNoop())
	tests := []struct {
		name       string
		trustLevel string
		want       bool
	}{
		{name: "trusted_allowed", trustLevel: TrustLevelTrusted, want: true},
		{name: "neutral_allowed", trustLevel: TrustLevelNeutral, want: true},
		{name: "untrusted_denied", trustLevel: TrustLevelUntrusted, want: false},
		{name: "empty_denied_fail_closed", trustLevel: "", want: false},
		{name: "unknown_denied_fail_closed", trustLevel: "partner", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.Check(tt.trustLevel); got != tt.want {
				t.Errorf("Check(%q) = %v, want %v", tt.trustLevel, got, tt.want)
			}
		})
	}
}

func TestTrustManager_NilLoggerSafe(t *testing.T) {
	m := NewTrustManager(nil)
	if !m.Check(TrustLevelTrusted) {
		t.Error("Check(trusted) = false, want true")
	}
	// Unknown level with nil logger must not panic (Warn path is nil-safe).
	if m.Check("bogus") {
		t.Error("Check(bogus) = true, want false")
	}
}
