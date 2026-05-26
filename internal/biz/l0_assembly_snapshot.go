package biz

import (
	"encoding/json"
	"strings"

	"aranea-agents/internal/llmcontext"
)

// L0AssemblySnapshotInsert is one persisted L0 assembly observation row.
type L0AssemblySnapshotInsert struct {
	ID                    string
	SessionID             string
	RunID                 string
	TurnID                string
	SpanID                string
	AgentID               string
	TeamID                string
	Provider              string
	Model                 string
	ContextWindowTokens   int
	BudgetTokens          int
	RecentWindowTurns     int
	RecentWindowTokens    int
	SummaryTokenEstimate  int
	L1FieldCount          int
	L1TokenEstimate       int
	L3ChunkCount          int
	L3TokenEstimate       int
	L4PathCount           int
	L4TokenEstimate       int
	PromptTokenEstimate   int
	PromptTokenActual     int
	UsedRatio             float64
	TruncateStrategy      string
	TruncatedMessageCount int
	SummarizedTurnFrom    int
	SummarizedTurnTo      int
	SegmentsJSON          string
	WarningCodesJSON      string
	MetadataJSON          string
	CreatedAt             string
}

// ShouldWriteL0AssemblySnapshot gates snapshot persistence per agent settings.
func ShouldWriteL0AssemblySnapshot(settings *AgentRuntimeSettings, usedRatio float64, forceDebug bool) bool {
	if forceDebug {
		return true
	}
	if settings == nil {
		return false
	}
	if !settings.EvolutionMetricsEnabled {
		return false
	}
	mode := strings.ToLower(strings.TrimSpace(settings.L0SnapshotMode))
	if mode == "off" {
		return false
	}
	if mode == "always" {
		return true
	}
	return usedRatio >= llmcontext.ContextStatusWarningThreshold
}

// L0WarningCodesFromRatio derives warning codes from estimated usage ratio.
func L0WarningCodesFromRatio(usedRatio float64) []string {
	switch llmcontext.ContextStatusForRatio(usedRatio) {
	case "exceeded":
		return []string{"exceeded"}
	case "critical":
		return []string{"critical"}
	case "warning":
		return []string{"near_limit"}
	default:
		return nil
	}
}

// L0WarningCodesJSON encodes warning codes for storage.
func L0WarningCodesJSON(codes []string) string {
	if len(codes) == 0 {
		return "[]"
	}
	b, err := json.Marshal(codes)
	if err != nil {
		return "[]"
	}
	return string(b)
}

// L0SnapshotObservePhase is stored in metadata_json.phase for observe-only rows.
const L0SnapshotObservePhase = "observe_v1"
