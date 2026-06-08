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

	// Reserved system tokens by ToolsProfile.
	reservedTokensCoding   = 15000
	reservedTokensResearch = 12000
	reservedTokensChatOnly = 4000
	reservedTokensDefault  = 8000

	// Max characters for L1 field value text in MemoryCompact.
	maxFieldTextChars = 200
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
		return reservedTokensCoding
	case "research":
		return reservedTokensResearch
	case "chat_only", "minimal":
		return reservedTokensChatOnly
	default:
		return reservedTokensDefault
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
	return biz.DefaultCompressionBufferRatio
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
	ratio := biz.DefaultSoftTriggerRatio
	if ag.Settings != nil && ag.Settings.SoftTriggerRatio > 0 {
		ratio = ag.Settings.SoftTriggerRatio
	}
	return reserved + int(float64(budget)*ratio) + int(float64(contextWindow)*compressionBufferRatio(ag))
}

// hardTriggerTokens returns the token count at which sync compression should trigger.
func hardTriggerTokens(ag biz.Agent, contextWindow int) int {
	reserved := calculateReservedSystem(ag)
	budget := effectiveBudget(contextWindow, reserved, compressionBufferRatio(ag))
	ratio := biz.DefaultHardTriggerRatio
	if ag.Settings != nil && ag.Settings.HardTriggerRatio > 0 {
		ratio = ag.Settings.HardTriggerRatio
	}
	return reserved + int(float64(budget)*ratio) + int(float64(contextWindow)*compressionBufferRatio(ag))
}

// ConversationMode represents the detected conversation pattern.
type ConversationMode int

const (
	ConversationModeMixed  ConversationMode = iota // 0.5 <= tool_call_count/turn_count <= 2.0
	ConversationModeCoding                         // tool_call_count/turn_count > 2.0
	ConversationModeChat                           // tool_call_count/turn_count < 0.5
)

// DetectConversationMode determines the conversation mode based on tool call density.
func DetectConversationMode(toolCallCount, turnCount int) ConversationMode {
	if turnCount <= 0 {
		return ConversationModeMixed
	}
	density := float64(toolCallCount) / float64(turnCount)
	if density > 2.0 {
		return ConversationModeCoding
	}
	if density < 0.5 {
		return ConversationModeChat
	}
	return ConversationModeMixed
}

// Adaptive buffer ratio bounds.
const (
	adaptiveBufferMinRatio = 0.10
	adaptiveBufferMaxRatio = 0.25
)

// AdaptiveBufferState tracks token increments for adaptive buffer ratio adjustment.
// Thread-safety: instances are stored in Compressor.adaptiveBuffer (sync.Map) and accessed
// only within runCompress, which is serialized per-session by tryStartCompress CAS lock.
// Therefore no additional mutex is needed on the state fields.
type AdaptiveBufferState struct {
	LastUsedTokens      int
	ConsecutiveLowCount int
	CurrentRatio        float64
}

// NewAdaptiveBufferState creates a new adaptive buffer state with the given initial ratio.
func NewAdaptiveBufferState(initialRatio float64) *AdaptiveBufferState {
	if initialRatio < adaptiveBufferMinRatio {
		initialRatio = adaptiveBufferMinRatio
	}
	if initialRatio > adaptiveBufferMaxRatio {
		initialRatio = adaptiveBufferMaxRatio
	}
	return &AdaptiveBufferState{CurrentRatio: initialRatio}
}

// UpdateAdaptiveBuffer adjusts the buffer ratio based on token increment and conversation mode.
// Returns the new effective ratio (clamped to [0.10, 0.25]).
func (s *AdaptiveBufferState) UpdateAdaptiveBuffer(usedTokens, contextWindow int, mode ConversationMode) float64 {
	increment := usedTokens - s.LastUsedTokens
	buffer := float64(contextWindow) * s.CurrentRatio

	if increment > int(buffer*0.70) {
		s.CurrentRatio += 0.02
		s.ConsecutiveLowCount = 0
	} else if increment < int(buffer*0.30) {
		s.ConsecutiveLowCount++
		if s.ConsecutiveLowCount >= 5 {
			s.CurrentRatio -= 0.01
			s.ConsecutiveLowCount = 0
		}
	} else {
		s.ConsecutiveLowCount = 0
	}

	// Apply conversation mode bias.
	// Coding mode: ensure minimum buffer for fast-growing contexts.
	// Chat mode: cap buffer since contexts grow slowly (one-time adjustment, not cumulative).
	switch mode {
	case ConversationModeCoding:
		if s.CurrentRatio < 0.18 {
			s.CurrentRatio = 0.18
		}
	case ConversationModeChat:
		if s.CurrentRatio > 0.12 {
			s.CurrentRatio = 0.12
		}
	}

	// Clamp ratio.
	if s.CurrentRatio < adaptiveBufferMinRatio {
		s.CurrentRatio = adaptiveBufferMinRatio
	}
	if s.CurrentRatio > adaptiveBufferMaxRatio {
		s.CurrentRatio = adaptiveBufferMaxRatio
	}

	s.LastUsedTokens = usedTokens
	return s.CurrentRatio
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

// adaptiveBufferEnabled returns true if adaptive buffer adjustment is enabled for the agent.
// Defaults to true when not explicitly disabled.
func adaptiveBufferEnabled(ag biz.Agent) bool {
	if ag.Settings == nil {
		return true
	}
	return ag.Settings.CompressionBufferAdaptive
}

// effectiveBudgetRatio converts a token count to a ratio against contextWindow,
// accounting for reserved_system and compression_buffer.
func effectiveBudgetRatio(usedTokens, contextWindow int, ag biz.Agent) float64 {
	if contextWindow <= 0 {
		return 0
	}
	return float64(usedTokens) / float64(contextWindow)
}
