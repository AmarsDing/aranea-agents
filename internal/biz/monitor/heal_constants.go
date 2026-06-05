package monitor

import "time"

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
