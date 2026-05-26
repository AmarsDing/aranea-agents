package jobs

import (
	"testing"
	"time"
)

// TestDefaultCooldownInterval verifies env-var override and fallback to 1h default.
func TestDefaultCooldownInterval(t *testing.T) {
	t.Run("default when env unset", func(t *testing.T) {
		t.Setenv("MONITOR_ALERT_COOLDOWN_INTERVAL", "")
		if got := defaultCooldownInterval(); got != time.Hour {
			t.Errorf("expected 1h default, got %s", got)
		}
	})
	t.Run("reads valid env", func(t *testing.T) {
		t.Setenv("MONITOR_ALERT_COOLDOWN_INTERVAL", "30m")
		if got := defaultCooldownInterval(); got != 30*time.Minute {
			t.Errorf("expected 30m, got %s", got)
		}
	})
	t.Run("ignores invalid env", func(t *testing.T) {
		t.Setenv("MONITOR_ALERT_COOLDOWN_INTERVAL", "not-a-duration")
		if got := defaultCooldownInterval(); got != time.Hour {
			t.Errorf("expected fallback 1h, got %s", got)
		}
	})
}

// TestDefaultCooldownMaxAge verifies env-var override and fallback to 24h default.
func TestDefaultCooldownMaxAge(t *testing.T) {
	t.Run("default when env unset", func(t *testing.T) {
		t.Setenv("MONITOR_ALERT_COOLDOWN_MAX_AGE", "")
		if got := defaultCooldownMaxAge(); got != 24*time.Hour {
			t.Errorf("expected 24h default, got %s", got)
		}
	})
	t.Run("reads valid env", func(t *testing.T) {
		t.Setenv("MONITOR_ALERT_COOLDOWN_MAX_AGE", "48h")
		if got := defaultCooldownMaxAge(); got != 48*time.Hour {
			t.Errorf("expected 48h, got %s", got)
		}
	})
	t.Run("ignores zero duration env", func(t *testing.T) {
		t.Setenv("MONITOR_ALERT_COOLDOWN_MAX_AGE", "0s")
		if got := defaultCooldownMaxAge(); got != 24*time.Hour {
			t.Errorf("expected fallback 24h for zero duration, got %s", got)
		}
	})
}

// TestNewMonitorAlertCooldownCleanup_respectsZeroArgs verifies that passing 0, 0
// causes the constructor to apply env/default values (not store 0).
func TestNewMonitorAlertCooldownCleanup_respectsZeroArgs(t *testing.T) {
	t.Setenv("MONITOR_ALERT_COOLDOWN_INTERVAL", "")
	t.Setenv("MONITOR_ALERT_COOLDOWN_MAX_AGE", "")
	w := NewMonitorAlertCooldownCleanup(0, 0, nil, nil)
	if w.interval != time.Hour {
		t.Errorf("expected interval=1h from default, got %s", w.interval)
	}
	if w.maxAge != 24*time.Hour {
		t.Errorf("expected maxAge=24h from default, got %s", w.maxAge)
	}
}
