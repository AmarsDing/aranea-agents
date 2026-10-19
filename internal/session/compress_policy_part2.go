package session

import (
	"aranea-agents/internal/biz"
)

// softTriggerTokensWithRatioPolicy returns the soft trigger token count using CompressPolicy and explicit buffer ratio.
func softTriggerTokensWithRatioPolicy(p CompressPolicy, contextWindow int, bufferRatio float64) int {
	reserved := calculateReservedSystemPolicy(p, p.Profile.ToolsProfile)
	budget := effectiveBudget(contextWindow, reserved, bufferRatio)
	return reserved + int(float64(budget)*p.Threshold.SoftTriggerRatio) + int(float64(contextWindow)*bufferRatio)
}

// softTriggerTokensWithRatio returns the soft trigger token count using an explicit buffer ratio.
func softTriggerTokensWithRatio(ag biz.Agent, contextWindow int, bufferRatio float64) int {
	reserved := calculateReservedSystem(ag)
	budget := effectiveBudget(contextWindow, reserved, bufferRatio)
	ratio := biz.DefaultSoftTriggerRatio
	if ag.Settings != nil && ag.Settings.SoftTriggerRatio > 0 {
		ratio = ag.Settings.SoftTriggerRatio
	}
	return reserved + int(float64(budget)*ratio) + int(float64(contextWindow)*bufferRatio)
}

// hardTriggerTokensWithRatioPolicy returns the hard trigger token count using CompressPolicy and explicit buffer ratio.
func hardTriggerTokensWithRatioPolicy(p CompressPolicy, contextWindow int, bufferRatio float64) int {
	reserved := calculateReservedSystemPolicy(p, p.Profile.ToolsProfile)
	budget := effectiveBudget(contextWindow, reserved, bufferRatio)
	return reserved + int(float64(budget)*p.Threshold.HardTriggerRatio) + int(float64(contextWindow)*bufferRatio)
}

// hardTriggerTokensWithRatio returns the hard trigger token count using an explicit buffer ratio.
func hardTriggerTokensWithRatio(ag biz.Agent, contextWindow int, bufferRatio float64) int {
	reserved := calculateReservedSystem(ag)
	budget := effectiveBudget(contextWindow, reserved, bufferRatio)
	ratio := biz.DefaultHardTriggerRatio
	if ag.Settings != nil && ag.Settings.HardTriggerRatio > 0 {
		ratio = ag.Settings.HardTriggerRatio
	}
	return reserved + int(float64(budget)*ratio) + int(float64(contextWindow)*bufferRatio)
}

// adaptiveBufferEnabledPolicy returns true if adaptive buffer adjustment is enabled based on CompressPolicy.
func adaptiveBufferEnabledPolicy(p CompressPolicy) bool {
	return p.Switches.AdaptiveBufferEnabled
}

// adaptiveBufferEnabled returns true if adaptive buffer adjustment is enabled for the agent.
// Defaults to true when not explicitly disabled.
func adaptiveBufferEnabled(ag biz.Agent) bool {
	return adaptiveBufferEnabledPolicy(CompressPolicyFromAgent(ag))
}

// effectiveBudgetRatio converts a token count to a ratio against contextWindow,
// accounting for reserved_system and compression_buffer.
func effectiveBudgetRatio(usedTokens, contextWindow int, ag biz.Agent) float64 {
	if contextWindow <= 0 {
		return 0
	}
	return float64(usedTokens) / float64(contextWindow)
}
