package service

import (
	"fmt"
)

// SpiritTeamMode determines how the Spirit agent builds its team.
type SpiritTeamMode string

const (
	// SpiritModeCoordinator builds a coordinator-led team (default for complex tasks).
	SpiritModeCoordinator SpiritTeamMode = "coordinator"
	// SpiritModeSwarm builds a swarm-style team for collaborative tasks.
	SpiritModeSwarm SpiritTeamMode = "swarm"
	// SpiritModeDirect routes directly to a single agent without team construction.
	SpiritModeDirect SpiritTeamMode = "direct"
)

// SpiritModeDecision captures the result of Spirit mode selection.
type SpiritModeDecision struct {
	Mode        SpiritTeamMode
	TargetAgent string // Only set when Mode == SpiritModeDirect
	Reasoning   string
}

// SelectSpiritMode determines the appropriate team construction mode
// based on complexity assessment and task characteristics.
func SelectSpiritMode(complexityLevel string, taskDescription string, agentCount int) SpiritModeDecision {
	switch complexityLevel {
	case "simple":
		return SpiritModeDecision{
			Mode:      SpiritModeDirect,
			Reasoning: "简单任务，Spirit 直接回答",
		}
	case "moderate":
		return SpiritModeDecision{
			Mode:        SpiritModeDirect,
			TargetAgent: "", // Will be filled by plan_and_execute
			Reasoning:   "中等复杂度，委派单一 Agent",
		}
	case "complex":
		if agentCount >= 4 {
			return SpiritModeDecision{
				Mode:      SpiritModeCoordinator,
				Reasoning: fmt.Sprintf("复杂任务，%d 个 Agent 需要协调编排", agentCount),
			}
		}
		return SpiritModeDecision{
			Mode:      SpiritModeCoordinator,
			Reasoning: "复杂任务，使用编排管家协调",
		}
	default:
		return SpiritModeDecision{
			Mode:      SpiritModeCoordinator,
			Reasoning: "未知复杂度，使用安全默认值 coordinator",
		}
	}
}

// SpiritModeConfig holds the configuration needed for Spirit mode selection.
type SpiritModeConfig struct {
	ComplexityLevel string
	TaskDescription string
	AgentCount      int
}

// ResolveSpiritMode is the entry point for Spirit mode selection.
// It can be called from chat_orchestrator_turn.go to determine
// how to construct the Spirit agent's team.
func ResolveSpiritMode(cfg SpiritModeConfig) SpiritModeDecision {
	return SelectSpiritMode(cfg.ComplexityLevel, cfg.TaskDescription, cfg.AgentCount)
}
