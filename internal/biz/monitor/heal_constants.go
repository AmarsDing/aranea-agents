package monitor

import (
	"time"

	"aranea-agents/internal/conf"
)

const (
	SelfHealMinConfidence = 0.7
	SelfHealMaxHistory    = 1000
	SelfHealCooldownSec   = 300
)

var SeverityCooldown = map[string]time.Duration{
	"critical": 30 * time.Minute,
	"high":     10 * time.Minute,
	"medium":   5 * time.Minute,
	"low":      2 * time.Minute,
}

func GetSeverityCooldown(severity string) time.Duration {
	if d, ok := SeverityCooldown[severity]; ok {
		return d
	}
	return SeverityCooldown["medium"]
}

// ResolveSelfHealConfig returns resolved self-heal config values from *conf.Runtime.
// The existing package-level constants remain as fallback defaults for callers
// that do not yet have access to *conf.Runtime.
func ResolveSelfHealConfig(r *conf.Runtime) (minConfidence float64, maxHistory int32, severityCooldown map[string]time.Duration) {
	cfg := r.SelfHealConfig()
	severityCooldown = map[string]time.Duration{
		"critical": cfg.SeverityCooldownCritical,
		"high":     cfg.SeverityCooldownHigh,
		"medium":   cfg.SeverityCooldownMedium,
		"low":      cfg.SeverityCooldownLow,
	}
	return cfg.MinConfidence, cfg.MaxHistory, severityCooldown
}
