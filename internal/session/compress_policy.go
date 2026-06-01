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
