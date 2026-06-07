package service

import (
	"aranea-agents/internal/biz"
)

// SpiritTeamMode is an alias for biz.SpiritTeamMode for backward compatibility
// within the service package. New code should use biz.SpiritTeamMode directly.
type SpiritTeamMode = biz.SpiritTeamMode

const (
	// SpiritModeCoordinator builds a coordinator-led team (default for complex tasks).
	SpiritModeCoordinator SpiritTeamMode = biz.SpiritModeCoordinator
	// SpiritModeSwarm builds a swarm-style team for collaborative tasks.
	SpiritModeSwarm SpiritTeamMode = biz.SpiritModeSwarm
	// SpiritModeDirect routes directly to a single agent without team construction.
	SpiritModeDirect SpiritTeamMode = biz.SpiritModeDirect
)

// SpiritModeDecision is an alias for biz.SpiritModeDecision for backward compatibility.
type SpiritModeDecision = biz.SpiritModeDecision

// SpiritModeConfig is an alias for biz.SpiritModeConfig for backward compatibility.
type SpiritModeConfig = biz.SpiritModeConfig

// SelectSpiritMode delegates to biz.SelectSpiritMode.
func SelectSpiritMode(complexityLevel string, taskDescription string, agentCount int) SpiritModeDecision {
	return biz.SelectSpiritMode(complexityLevel, taskDescription, agentCount)
}

// ResolveSpiritMode delegates to biz.ResolveSpiritMode.
func ResolveSpiritMode(cfg SpiritModeConfig) SpiritModeDecision {
	return biz.ResolveSpiritMode(cfg)
}
