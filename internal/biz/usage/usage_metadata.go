package usage

import (
	"encoding/json"
	"strings"
)

// MetadataKeyUsageSource is the metadata_json key recording how a usage row's
// token counts were obtained: "streaming" / "runner_completion" (provider-reported)
// or "estimated" (filled from text by EstimateTokensIfMissing). Lets DB queries
// distinguish estimated rows from authoritative ones (P0-2, 2026-08-19).
const MetadataKeyUsageSource = "usage_source"

// MergeUsageSourceMetadata sets metadata_json[usage_source], preserving existing
// keys. Passthrough when usageSource is empty (usage never observed — nothing to
// claim) or metaJSON is unparseable (caller keeps its original payload).
func MergeUsageSourceMetadata(metaJSON, usageSource string) string {
	usageSource = strings.TrimSpace(usageSource)
	if usageSource == "" {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyUsageSource] = usageSource
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// MetadataKeyEffort is the metadata_json key recording the resolved thinking
// effort applied to the LLM call (off/low/medium/high/max; M83 LBG-6). Only
// rows whose call site routes effort (spirit planner decompose / allocator
// match) carry the key; other rows omit it and the frontend renders empty.
const MetadataKeyEffort = "effort"

// MergeEffortMetadata sets metadata_json[effort], preserving existing keys.
// Passthrough when effort is empty (caller did not route effort — nothing to
// claim) or metaJSON is unparseable. Callers must pass the value actually
// applied to the request (post-ResolveThinkingEffort), not a desired value.
func MergeEffortMetadata(metaJSON, effort string) string {
	effort = strings.TrimSpace(effort)
	if effort == "" {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyEffort] = effort
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// MetadataKeyLLMRounds is the metadata_json key recording how many LLM rounds
// (billable API calls) a chat_turn usage row sums over. Tool-call loops bill
// each round's full prompt separately, so without the round count a summed
// row is unexplainable (P1-C, 2026-08-20).
const MetadataKeyLLMRounds = "llm_rounds"

// MergeLLMRoundsMetadata sets metadata_json[llm_rounds], preserving existing
// keys. Passthrough when rounds <= 0 (no stream observation — nothing to
// claim) or metaJSON is unparseable.
func MergeLLMRoundsMetadata(metaJSON string, rounds int) string {
	if rounds <= 0 {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyLLMRounds] = rounds
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// MetadataKeyWaitMS / MetadataKeyModelLatencyMS split HITL wait out of
// wall-clock latency on chat_turn usage rows (包 D D6).
const (
	MetadataKeyWaitMS         = "wait_ms"
	MetadataKeyModelLatencyMS = "model_latency_ms"
)

// MergeWaitMSMetadata sets wait_ms and model_latency_ms (= max(0, latency-wait)).
func MergeWaitMSMetadata(metaJSON string, waitMS, latencyMS int) string {
	if waitMS <= 0 {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyWaitMS] = waitMS
	payload[MetadataKeyModelLatencyMS] = ModelLatencyMS(latencyMS, waitMS)
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// ModelLatencyMS is max(0, wall latency − HITL wait).
func ModelLatencyMS(latencyMS, waitMS int) int {
	if waitMS < 0 {
		waitMS = 0
	}
	out := latencyMS - waitMS
	if out < 0 {
		return 0
	}
	return out
}

// MetadataKeyUsageAttribution is the metadata_json key recording how a usage
// row's tokens are attributed. UsageAttributionRunLevelAnchorFallback marks
// team_member rows that carry RUN-LEVEL totals attributed to the anchor member
// (anchor-fallback path when graph watch produced no per-member steps) — such
// rows duplicate the team_turn row's totals and must NOT additionally
// accumulate session metrics (P2-1 双计根治, 2026-08-19).
const MetadataKeyUsageAttribution = "usage_attribution"

// UsageAttributionRunLevelAnchorFallback see MetadataKeyUsageAttribution.
const UsageAttributionRunLevelAnchorFallback = "run_level_anchor_fallback"

// UsageAttributionMemberLevelStream marks team_member rows persisted from the
// stream's per-member MemberUsage on the completion path (P2-1b, 2026-08-19):
// graph watch writes member steps with zero tokens, so without these rows a
// watch-healthy team run is invisible to billable aggregates. Like the
// anchor-fallback row, these duplicate the team_turn row's totals and must NOT
// additionally accumulate session metrics.
const UsageAttributionMemberLevelStream = "member_level_stream"

// UsageAttributionStreamAnchorRemainder marks the team_member row carrying the
// run-total REMAINDER (run totals − Σ MemberUsage) attributed to the anchor
// member: consumption authored by the team root (non-member author) is inside
// the run totals but outside every MemberUsage entry. Same session-metrics
// rule as UsageAttributionMemberLevelStream.
const UsageAttributionStreamAnchorRemainder = "stream_anchor_remainder"

// MergeUsageAttributionMetadata sets metadata_json[usage_attribution],
// preserving existing keys. Same passthrough rules as MergeUsageSourceMetadata.
func MergeUsageAttributionMetadata(metaJSON, attribution string) string {
	attribution = strings.TrimSpace(attribution)
	if attribution == "" {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyUsageAttribution] = attribution
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}

// MetadataKeyCostStatus is the metadata_json key flagging rows whose cost could
// not be priced (P2-2, 2026-08-19). CostStatusNoPricing marks events that carry
// billable tokens but zero computed cost AND no price fields at all — without
// the flag, missing pricing config is indistinguishable from legitimately-free
// consumption in aggregates.
const MetadataKeyCostStatus = "cost_status"

// CostStatusNoPricing see MetadataKeyCostStatus.
const CostStatusNoPricing = "no_pricing"

// MergeCostStatusMetadata sets metadata_json[cost_status], preserving existing
// keys. Same passthrough rules as MergeUsageSourceMetadata.
func MergeCostStatusMetadata(metaJSON, status string) string {
	status = strings.TrimSpace(status)
	if status == "" {
		return metaJSON
	}
	var payload map[string]any
	if err := json.Unmarshal([]byte(metaJSON), &payload); err != nil || payload == nil {
		payload = map[string]any{}
	}
	payload[MetadataKeyCostStatus] = status
	raw, err := json.Marshal(payload)
	if err != nil {
		return metaJSON
	}
	return string(raw)
}
