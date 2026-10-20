package session

import (
	"aranea-agents/internal/biz"
	"testing"
)

func TestAdaptiveBufferEnabled(t *testing.T) {
	t.Run("nil_settings_defaults_true", func(t *testing.T) {
		if !adaptiveBufferEnabled(biz.Agent{}) {
			t.Error("nil settings should default to true")
		}
	})

	t.Run("explicitly_enabled", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressionBufferAdaptive: true}}
		if !adaptiveBufferEnabled(ag) {
			t.Error("explicitly enabled should return true")
		}
	})

	t.Run("explicitly_disabled", func(t *testing.T) {
		ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{CompressionBufferAdaptive: false}}
		if adaptiveBufferEnabled(ag) {
			t.Error("explicitly disabled should return false")
		}
	})
}
