package session

import (
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"
)

const (
	DefaultCompressMinGap = 10 * time.Minute
	compressRunTimeout    = 8 * time.Minute

	defaultCompressThreshold = 0.6
	forcedCompressThreshold  = 0.35
	defaultKeepTurns         = 4

	defaultCompressionBufferRatio = 0.15
	defaultSoftTriggerRatio       = 0.70
	defaultHardTriggerRatio       = 0.90
)

func compressThresholdAndKeep(ag biz.Agent) (threshold float64, keepTurns int) {
	threshold = defaultCompressThreshold
	keepTurns = defaultKeepTurns
	if ag.Settings == nil {
		return threshold, keepTurns
	}
	if ag.Settings.L0SummaryThreshold > 0 {
		threshold = ag.Settings.L0SummaryThreshold
	}
	if ag.Settings.L0SummaryKeepTurns > 0 {
		keepTurns = ag.Settings.L0SummaryKeepTurns
	} else if ag.Settings.L0RecentWindowTurns > 0 {
		keepTurns = ag.Settings.L0RecentWindowTurns
	}
	return threshold, keepTurns
}

func sessionCompressEnabled(ag biz.Agent) bool {
	if ag.Settings == nil {
		return true
	}
	if ag.Settings.ContextCompactionEnabled {
		return true
	}
	return strings.ToLower(strings.TrimSpace(ag.Settings.L0SnapshotMode)) != "off"
}

func microCompactEnabled(ag biz.Agent) bool {
	if !sessionCompressEnabled(ag) {
		return false
	}
	if ag.Settings == nil {
		return true
	}
	return ag.Settings.MicroCompactEnabled
}

func memoryCompactEnabled(ag biz.Agent) bool {
	if !sessionCompressEnabled(ag) {
		return false
	}
	if ag.Settings == nil {
		return true
	}
	return ag.Settings.MemoryCompactEnabled
}

func sessionCompressThreshold(ag biz.Agent) float64 {
	threshold, _ := compressThresholdAndKeep(ag)
	mode := "on_warning"
	if ag.Settings != nil {
		mode = strings.ToLower(strings.TrimSpace(ag.Settings.L0SnapshotMode))
	}
	if mode == "always" && threshold > forcedCompressThreshold {
		return forcedCompressThreshold
	}
	return threshold
}

func truncateStrategy(ag biz.Agent) string {
	if ag.Settings == nil {
		return "summary"
	}
	s := strings.ToLower(strings.TrimSpace(ag.Settings.L0TruncateStrategy))
	if s == "" {
		return "summary"
	}
	return s
}

func filterMessagesForTruncateStrategy(msgs []biz.ChatMessage, strategy string) []biz.ChatMessage {
	strategy = strings.ToLower(strings.TrimSpace(strategy))
	if strategy != "drop_tool_results" && strategy != "hybrid" {
		return msgs
	}
	out := make([]biz.ChatMessage, 0, len(msgs))
	for _, m := range msgs {
		if strings.EqualFold(strings.TrimSpace(m.Role), "tool") {
			continue
		}
		out = append(out, m)
	}
	return out
}

func compressProviderModel(sess biz.Session, ag biz.Agent) (prov, mod string) {
	if ag.Settings != nil {
		p := strings.TrimSpace(ag.Settings.L0CompressProvider)
		m := strings.TrimSpace(ag.Settings.L0CompressModel)
		if p != "" && m != "" {
			return p, m
		}
	}
	return strutil.FirstNonEmpty(sess.DefaultProvider, ag.Provider), strutil.FirstNonEmpty(sess.DefaultModel, ag.Model)
}

func compressMinGapFromAgent(ag biz.Agent) time.Duration {
	if ag.Settings != nil && ag.Settings.L0CompressMinGapSec > 0 {
		return time.Duration(ag.Settings.L0CompressMinGapSec) * time.Second
	}
	return DefaultCompressMinGap
}

func compressDebounceActive(lastSummaryRFC3339 string, minGap time.Duration, now time.Time) bool {
	if minGap <= 0 {
		return false
	}
	ts := strings.TrimSpace(lastSummaryRFC3339)
	if ts == "" {
		return false
	}
	t, err := time.Parse(time.RFC3339, ts)
	if err != nil {
		return false
	}
	return now.Sub(t) < minGap
}

func atFullContextUsage(sess biz.Session) bool {
	if sess.ContextUsedRatio >= 1.0 {
		return true
	}
	if sess.LastContextWindowTokens > 0 && sess.ContextUsedTokens >= sess.LastContextWindowTokens {
		return true
	}
	return false
}

// profileBasedDefault returns the estimated reserved_system tokens for a given ToolsProfile.
func profileBasedDefault(profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "coding", "full":
		return 15000
	case "research":
		return 12000
	case "chat_only", "minimal":
		return 4000
	default:
		return 8000
	}
}

// calculateReservedSystem estimates the system prompt tokens that are not compressible.
// It uses the agent's ToolsProfile to determine a profile-based default.
func calculateReservedSystem(ag biz.Agent) int {
	if ag.Settings != nil && ag.Settings.L0SummaryThreshold > 0 {
		// Future: read from latest L0 snapshot segments_json
		// For now, use profile-based defaults
	}
	profile := ""
	if ag.Settings != nil {
		profile = ag.Settings.ToolsProfile
	}
	return profileBasedDefault(profile)
}

// compressionBufferRatio returns the buffer ratio from agent settings or the default.
func compressionBufferRatio(ag biz.Agent) float64 {
	if ag.Settings != nil && ag.Settings.CompressionBufferRatio > 0 {
		return ag.Settings.CompressionBufferRatio
	}
	return defaultCompressionBufferRatio
}

// effectiveBudget calculates the usable token budget for conversation content.
// effective_budget = contextWindow - reserved_system - compression_buffer
func effectiveBudget(contextWindow, reservedSystem int, bufferRatio float64) int {
	buf := int(float64(contextWindow) * bufferRatio)
	budget := contextWindow - reservedSystem - buf
	if budget < 0 {
		return 0
	}
	return budget
}

// softTriggerTokens returns the token count at which async compression should trigger.
func softTriggerTokens(ag biz.Agent, contextWindow int) int {
	reserved := calculateReservedSystem(ag)
	budget := effectiveBudget(contextWindow, reserved, compressionBufferRatio(ag))
	ratio := defaultSoftTriggerRatio
	if ag.Settings != nil && ag.Settings.SoftTriggerRatio > 0 {
		ratio = ag.Settings.SoftTriggerRatio
	}
	return reserved + int(float64(budget)*ratio) + int(float64(contextWindow)*compressionBufferRatio(ag))
}

// hardTriggerTokens returns the token count at which sync compression should trigger.
func hardTriggerTokens(ag biz.Agent, contextWindow int) int {
	reserved := calculateReservedSystem(ag)
	budget := effectiveBudget(contextWindow, reserved, compressionBufferRatio(ag))
	ratio := defaultHardTriggerRatio
	if ag.Settings != nil && ag.Settings.HardTriggerRatio > 0 {
		ratio = ag.Settings.HardTriggerRatio
	}
	return reserved + int(float64(budget)*ratio) + int(float64(contextWindow)*compressionBufferRatio(ag))
}

// effectiveBudgetRatio converts a token count to a ratio against contextWindow,
// accounting for reserved_system and compression_buffer.
func effectiveBudgetRatio(usedTokens, contextWindow int, ag biz.Agent) float64 {
	if contextWindow <= 0 {
		return 0
	}
	return float64(usedTokens) / float64(contextWindow)
}
