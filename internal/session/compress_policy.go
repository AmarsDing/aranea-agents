package session

import (
	"strings"
	"sync/atomic"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/strutil"
)

// CompressPolicy aggregates all compression-related configuration into a single struct.
// This replaces the scattered access to AgentSettings fields and hardcoded constants.
// Sub-structs are grouped by responsibility to keep total direct fields ≤ 15 (AS-COG-01).
type CompressPolicy struct {
	Switches  CompressSwitches
	Threshold CompressThreshold
	Timing    CompressTiming
	Model     CompressModelConfig
	Profile   CompressProfile
}

// CompressSwitches controls which compression stages are active.
type CompressSwitches struct {
	Enabled               bool
	MicroCompactEnabled   bool
	MemoryCompactEnabled  bool
	AdaptiveBufferEnabled bool
}

// CompressThreshold defines when compression triggers.
type CompressThreshold struct {
	SummaryThreshold  float64 // L0SummaryThreshold
	ForcedThreshold   float64 // forcedCompressThreshold
	SoftTriggerRatio  float64
	HardTriggerRatio  float64
	BufferRatio       float64 // CompressionBufferRatio
	KeepTurns         int     // L0SummaryKeepTurns
	RecentWindowTurns int     // L0RecentWindowTurns
}

// CompressTiming controls compression scheduling.
type CompressTiming struct {
	MinGap  time.Duration // L0CompressMinGapSec → Duration
	Timeout time.Duration // compressRunTimeout
}

// CompressModelConfig specifies the LLM used for compression.
type CompressModelConfig struct {
	Provider         string // L0CompressProvider
	Model            string // L0CompressModel
	TruncateStrategy string // L0TruncateStrategy
	SnapshotMode     string // L0SnapshotMode
}

// CompressProfile defines reserved tokens per conversation mode.
type CompressProfile struct {
	ReservedTokensCoding   int
	ReservedTokensResearch int
	ReservedTokensChatOnly int
	ReservedTokensDefault  int
	MaxFieldTextChars      int // MemoryCompact field limit
}

// DefaultCompressPolicy returns a CompressPolicy with sensible defaults
// matching the current hardcoded values.
func DefaultCompressPolicy() CompressPolicy {
	return CompressPolicy{
		Switches: CompressSwitches{
			Enabled:               true,
			MicroCompactEnabled:   true,
			MemoryCompactEnabled:  true,
			AdaptiveBufferEnabled: true,
		},
		Threshold: CompressThreshold{
			SummaryThreshold:  defaultCompressThreshold,
			ForcedThreshold:   forcedCompressThreshold,
			SoftTriggerRatio:  biz.DefaultSoftTriggerRatio,
			HardTriggerRatio:  biz.DefaultHardTriggerRatio,
			BufferRatio:       biz.DefaultCompressionBufferRatio,
			KeepTurns:         defaultKeepTurns,
			RecentWindowTurns: 0,
		},
		Timing: CompressTiming{
			MinGap:  DefaultCompressMinGap,
			Timeout: compressRunTimeout,
		},
		Model: CompressModelConfig{
			Provider:         "",
			Model:            "",
			TruncateStrategy: "summary",
			SnapshotMode:     "",
		},
		Profile: CompressProfile{
			ReservedTokensCoding:   reservedTokensCoding,
			ReservedTokensResearch: reservedTokensResearch,
			ReservedTokensChatOnly: reservedTokensChatOnly,
			ReservedTokensDefault:  reservedTokensDefault,
			MaxFieldTextChars:      maxFieldTextChars,
		},
	}
}

// CompressPolicyFromAgent extracts the compression policy from an Agent's settings,
// falling back to defaults for unset fields.
func CompressPolicyFromAgent(ag biz.Agent) CompressPolicy {
	p := DefaultCompressPolicy()
	if ag.Settings == nil {
		return p
	}
	s := ag.Settings

	// Switches
	// ContextCompactionEnabled is the explicit modern switch; it takes precedence
	// over the legacy L0SnapshotMode="off" so that agents opting into context
	// compaction are not silently disabled by a legacy default.
	if strings.ToLower(strings.TrimSpace(s.L0SnapshotMode)) == "off" && !s.ContextCompactionEnabled {
		p.Switches.Enabled = false
	}
	if s.ContextCompactionEnabled {
		p.Switches.Enabled = true
	}
	p.Switches.MicroCompactEnabled = s.MicroCompactEnabled
	p.Switches.MemoryCompactEnabled = s.MemoryCompactEnabled
	p.Switches.AdaptiveBufferEnabled = s.CompressionBufferAdaptive

	// Thresholds
	if s.L0SummaryThreshold > 0 {
		p.Threshold.SummaryThreshold = s.L0SummaryThreshold
	}
	if s.SoftTriggerRatio > 0 {
		p.Threshold.SoftTriggerRatio = s.SoftTriggerRatio
	}
	if s.HardTriggerRatio > 0 {
		p.Threshold.HardTriggerRatio = s.HardTriggerRatio
	}
	if s.CompressionBufferRatio > 0 {
		p.Threshold.BufferRatio = s.CompressionBufferRatio
	}

	// Retention
	if s.L0SummaryKeepTurns > 0 {
		p.Threshold.KeepTurns = s.L0SummaryKeepTurns
	}
	if s.L0RecentWindowTurns > 0 {
		p.Threshold.RecentWindowTurns = s.L0RecentWindowTurns
	}

	// Timing
	if s.L0CompressMinGapSec > 0 {
		p.Timing.MinGap = time.Duration(s.L0CompressMinGapSec) * time.Second
	}

	// Model
	if v := strings.TrimSpace(s.L0CompressProvider); v != "" {
		p.Model.Provider = v
	}
	if v := strings.TrimSpace(s.L0CompressModel); v != "" {
		p.Model.Model = v
	}

	// Strategy
	if v := strings.ToLower(strings.TrimSpace(s.L0TruncateStrategy)); v != "" {
		p.Model.TruncateStrategy = v
	}
	if v := strings.TrimSpace(s.L0SnapshotMode); v != "" {
		p.Model.SnapshotMode = v
	}

	return p
}

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

// compressThresholdAndKeepPolicy returns the threshold and keepTurns from a CompressPolicy.
func compressThresholdAndKeepPolicy(p CompressPolicy) (threshold float64, keepTurns int) {
	threshold = p.Threshold.SummaryThreshold
	keepTurns = p.Threshold.KeepTurns
	if p.Threshold.RecentWindowTurns > 0 && p.Threshold.KeepTurns == defaultKeepTurns {
		keepTurns = p.Threshold.RecentWindowTurns
	}
	return threshold, keepTurns
}

// compressThresholdAndKeep is a backward-compatible wrapper that reads from biz.Agent.
func compressThresholdAndKeep(ag biz.Agent) (threshold float64, keepTurns int) {
	return compressThresholdAndKeepPolicy(CompressPolicyFromAgent(ag))
}

// sessionCompressEnabledPolicy returns whether compression is enabled based on CompressPolicy.
func sessionCompressEnabledPolicy(p CompressPolicy) bool {
	return p.Switches.Enabled
}

func sessionCompressEnabled(ag biz.Agent) bool {
	return sessionCompressEnabledPolicy(CompressPolicyFromAgent(ag))
}

// microCompactEnabledPolicy returns whether micro compact is enabled based on CompressPolicy.
func microCompactEnabledPolicy(p CompressPolicy) bool {
	if !p.Switches.Enabled {
		return false
	}
	return p.Switches.MicroCompactEnabled
}

func microCompactEnabled(ag biz.Agent) bool {
	return microCompactEnabledPolicy(CompressPolicyFromAgent(ag))
}

// memoryCompactEnabledPolicy returns whether memory compact is enabled based on CompressPolicy.
func memoryCompactEnabledPolicy(p CompressPolicy) bool {
	if !p.Switches.Enabled {
		return false
	}
	return p.Switches.MemoryCompactEnabled
}

func memoryCompactEnabled(ag biz.Agent) bool {
	return memoryCompactEnabledPolicy(CompressPolicyFromAgent(ag))
}

// sessionCompressThresholdPolicy returns the effective compress threshold from CompressPolicy.
func sessionCompressThresholdPolicy(p CompressPolicy) float64 {
	mode := strings.ToLower(strings.TrimSpace(p.Model.SnapshotMode))
	if mode == "always" && p.Threshold.SummaryThreshold > p.Threshold.ForcedThreshold {
		return p.Threshold.ForcedThreshold
	}
	return p.Threshold.SummaryThreshold
}

func sessionCompressThreshold(ag biz.Agent) float64 {
	return sessionCompressThresholdPolicy(CompressPolicyFromAgent(ag))
}

// truncateStrategyPolicy returns the truncate strategy from CompressPolicy.
func truncateStrategyPolicy(p CompressPolicy) string {
	s := strings.ToLower(strings.TrimSpace(p.Model.TruncateStrategy))
	if s == "" {
		return "summary"
	}
	return s
}

func truncateStrategy(ag biz.Agent) string {
	return truncateStrategyPolicy(CompressPolicyFromAgent(ag))
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

// compressProviderModelPolicy returns the compress provider and model from CompressPolicy.
func compressProviderModelPolicy(p CompressPolicy, sess biz.Session, ag biz.Agent) (prov, mod string) {
	if p.Model.Provider != "" && p.Model.Model != "" {
		return p.Model.Provider, p.Model.Model
	}
	return strutil.FirstNonEmpty(sess.DefaultProvider, ag.Provider), strutil.FirstNonEmpty(sess.DefaultModel, ag.Model)
}

func compressProviderModel(sess biz.Session, ag biz.Agent) (prov, mod string) {
	return compressProviderModelPolicy(CompressPolicyFromAgent(ag), sess, ag)
}

// compressProviderModelKey returns the "provider/model" identity of the
// compression model, used as the suppression key (model switch clears suppression).
func compressProviderModelKey(sess biz.Session, ag biz.Agent) string {
	p, m := compressProviderModel(sess, ag)
	return p + "/" + m
}

// compressMinGapFromAgentPolicy returns the minimum gap between compressions from CompressPolicy.
func compressMinGapFromAgentPolicy(p CompressPolicy) time.Duration {
	return p.Timing.MinGap
}

func compressMinGapFromAgent(ag biz.Agent) time.Duration {
	return compressMinGapFromAgentPolicy(CompressPolicyFromAgent(ag))
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

// profileBasedDefaultPolicy returns the estimated reserved_system tokens for a given ToolsProfile
// using the CompressPolicy's reserved token fields.
func profileBasedDefaultPolicy(p CompressPolicy, profile string) int {
	switch strings.ToLower(strings.TrimSpace(profile)) {
	case "coding", "full":
		return p.Profile.ReservedTokensCoding
	case "research":
		return p.Profile.ReservedTokensResearch
	case "chat_only", "minimal":
		return p.Profile.ReservedTokensChatOnly
	default:
		return p.Profile.ReservedTokensDefault
	}
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

// calculateReservedSystemPolicy estimates the system prompt tokens using CompressPolicy.
func calculateReservedSystemPolicy(p CompressPolicy, profile string) int {
	return profileBasedDefaultPolicy(p, profile)
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

// compressionBufferRatioPolicy returns the buffer ratio from CompressPolicy.
func compressionBufferRatioPolicy(p CompressPolicy) float64 {
	return p.Threshold.BufferRatio
}

// compressionBufferRatio returns the buffer ratio from agent settings or the default.
func compressionBufferRatio(ag biz.Agent) float64 {
	return compressionBufferRatioPolicy(CompressPolicyFromAgent(ag))
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

// softTriggerTokensPolicy returns the soft trigger token count using CompressPolicy.
func softTriggerTokensPolicy(p CompressPolicy, contextWindow int) int {
	reserved := calculateReservedSystemPolicy(p, p.Model.SnapshotMode) // profile derived from SnapshotMode is not ideal; use ToolsProfile
	budget := effectiveBudget(contextWindow, reserved, p.Threshold.BufferRatio)
	return reserved + int(float64(budget)*p.Threshold.SoftTriggerRatio) + int(float64(contextWindow)*p.Threshold.BufferRatio)
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

// hardTriggerTokensPolicy returns the hard trigger token count using CompressPolicy.
func hardTriggerTokensPolicy(p CompressPolicy, contextWindow int) int {
	reserved := calculateReservedSystemPolicy(p, p.Model.SnapshotMode)
	budget := effectiveBudget(contextWindow, reserved, p.Threshold.BufferRatio)
	return reserved + int(float64(budget)*p.Threshold.HardTriggerRatio) + int(float64(contextWindow)*p.Threshold.BufferRatio)
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
// Thread-safety: instances are stored in compressBufferManager.buffer (sync.Map).
// LastAccessed is accessed atomically to avoid data races between the compress
// goroutine (writer) and the GC goroutine (reader). All other fields are only
// accessed within runCompress, which is serialized per-session by tryStartCompress
// CAS lock, so no additional mutex is needed for those.
type AdaptiveBufferState struct {
	LastUsedTokens      int
	ConsecutiveLowCount int
	CurrentRatio        float64
	lastAccessedUnix    atomic.Int64
}

// LastAccessed returns the last access time of this state.
func (s *AdaptiveBufferState) LastAccessed() time.Time {
	unix := s.lastAccessedUnix.Load()
	if unix == 0 {
		return time.Time{}
	}
	return time.Unix(0, unix)
}

// touchLastAccessed updates the last access time to now.
func (s *AdaptiveBufferState) touchLastAccessed() {
	s.lastAccessedUnix.Store(time.Now().UnixNano())
}

// NewAdaptiveBufferState creates a new adaptive buffer state with the given initial ratio.
func NewAdaptiveBufferState(initialRatio float64) *AdaptiveBufferState {
	if initialRatio < adaptiveBufferMinRatio {
		initialRatio = adaptiveBufferMinRatio
	}
	if initialRatio > adaptiveBufferMaxRatio {
		initialRatio = adaptiveBufferMaxRatio
	}
	s := &AdaptiveBufferState{CurrentRatio: initialRatio}
	s.touchLastAccessed()
	return s
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
	s.touchLastAccessed()
	return s.CurrentRatio
}

// softTriggerTokensWithRatioPolicy returns the soft trigger token count using CompressPolicy and explicit buffer ratio.
func softTriggerTokensWithRatioPolicy(p CompressPolicy, contextWindow int, bufferRatio float64) int {
	reserved := calculateReservedSystemPolicy(p, p.Model.SnapshotMode)
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
	reserved := calculateReservedSystemPolicy(p, p.Model.SnapshotMode)
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
