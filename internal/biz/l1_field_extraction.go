package biz

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
)

// StructuredEpisodeData holds the zero-cost extracted data from an L1 task snapshot.
type StructuredEpisodeData struct {
	Title          string
	Goal           string
	Outcome        string
	OutcomeSummary string
	KeyDecisions   []FieldDecision
	KeyArtifacts   []FieldArtifact
	Importance     float64
	Confidence     float64
	EpisodeKind    string
}

// FieldDecision represents a key decision extracted from an L1 field.
type FieldDecision struct {
	Path  string
	Value string
}

// FieldArtifact represents a key artifact or reference extracted from an L1 field.
type FieldArtifact struct {
	Path  string
	Value string
	Kind  string // "artifact" or "reference"
}

// l1Snapshot is the internal structure for parsing an L1 snapshot JSON.
type l1Snapshot struct {
	Task   map[string]any    `json:"task"`
	Fields []map[string]any  `json:"fields"`
}

// ExtractStructuredEpisode parses an L1 snapshot JSON and extracts structured episode data.
func ExtractStructuredEpisode(snapshotJSON []byte) StructuredEpisodeData {
	var snap l1Snapshot
	if err := json.Unmarshal(snapshotJSON, &snap); err != nil {
		return StructuredEpisodeData{
			KeyDecisions: []FieldDecision{},
			KeyArtifacts: []FieldArtifact{},
			Importance:   0.5,
			Confidence:   0.6,
			EpisodeKind:  "l1_archive_structured",
		}
	}

	title := strVal(snap.Task, "task_title")
	goal := strVal(snap.Task, "task_goal")
	status := strVal(snap.Task, "status")

	outcomeSummary := mapStatusToOutcome(status)
	outcome := outcomeSummary
	lastMsg := strVal(snap.Task, "last_assistant_message")
	if lastMsg != "" {
		lastMsg = truncateRunes(lastMsg, maxOutcomeAppendChars)
		outcome = outcomeSummary + lastMsg
	}

	keyDecisions := ExtractKeyDecisions(snap.Fields)
	keyArtifacts := ExtractKeyArtifacts(snap.Fields)

	return StructuredEpisodeData{
		Title:          title,
		Goal:           goal,
		Outcome:        outcome,
		OutcomeSummary: outcomeSummary,
		KeyDecisions:   keyDecisions,
		KeyArtifacts:   keyArtifacts,
		Importance:     0.5,
		Confidence:     0.6,
		EpisodeKind:    "l1_archive_structured",
	}
}

// mapStatusToOutcome converts a task status to a human-readable outcome summary.
func mapStatusToOutcome(status string) string {
	switch status {
	case "completed":
		return "任务已完成"
	case "cancelled":
		return "任务被取消（空闲超时）"
	case "failed":
		return "任务失败"
	case "timeout":
		return "任务超时"
	default:
		return status
	}
}

// ExtractKeyDecisions implements a 4-layer fallback strategy for extracting key decisions.
func ExtractKeyDecisions(fields []map[string]any) []FieldDecision {
	// Layer 0: field_kind = "decision"
	if result := extractByFieldKind(fields, "decision"); len(result) > 0 {
		return result
	}

	// Layer 1: field_path pattern matching (contains "decision", "choice", "approach", "option")
	patterns := []string{"decision", "choice", "approach", "option"}
	if result := extractByPathPatterns(fields, patterns, ""); len(result) > 0 {
		return result
	}

	// Layer 2: pin_to_prompt=true AND visibility="prompt" → top 3 by updated_at
	if result := extractPinnedAndVisible(fields, 3); len(result) > 0 {
		return result
	}

	// Layer 3: most recently updated visibility="prompt" fields → top 5
	return extractRecentVisible(fields, 5)
}

// ExtractKeyArtifacts implements a 3-layer fallback strategy for extracting key artifacts.
func ExtractKeyArtifacts(fields []map[string]any) []FieldArtifact {
	// Layer 0: field_kind = "artifact" OR field_kind = "reference"
	if result := extractArtifactsByKind(fields); len(result) > 0 {
		return result
	}

	// Layer 1: field_path pattern matching (contains "file", "path", "config", "output") AND field_kind="reference"
	patterns := []string{"file", "path", "config", "output"}
	if result := extractArtifactsByPathAndKind(fields, patterns); len(result) > 0 {
		return result
	}

	// Layer 2: field_kind = "reference"
	return extractArtifactsBySingleKind(fields, "reference")
}

// extractByFieldKind collects all fields matching the given field_kind.
func extractByFieldKind(fields []map[string]any, kind string) []FieldDecision {
	var result []FieldDecision
	for _, f := range fields {
		if strings.EqualFold(strVal(f, "field_kind"), kind) {
			result = append(result, FieldDecision{
				Path:  strVal(f, "field_path"),
				Value: strVal(f, "value_text"),
			})
		}
	}
	if result == nil {
		result = []FieldDecision{}
	}
	return result
}

// extractByPathPatterns collects fields whose field_path contains any of the patterns (case-insensitive).
func extractByPathPatterns(fields []map[string]any, patterns []string, kindFilter string) []FieldDecision {
	var result []FieldDecision
	for _, f := range fields {
		path := strVal(f, "field_path")
		pathLower := strings.ToLower(path)
		matched := false
		for _, p := range patterns {
			if strings.Contains(pathLower, p) {
				matched = true
				break
			}
		}
		if !matched {
			continue
		}
		if kindFilter != "" && !strings.EqualFold(strVal(f, "field_kind"), kindFilter) {
			continue
		}
		result = append(result, FieldDecision{
			Path:  path,
			Value: strVal(f, "value_text"),
		})
	}
	if result == nil {
		result = []FieldDecision{}
	}
	return result
}

// extractPinnedAndVisible collects fields where pin_to_prompt=true AND visibility="prompt", top N by updated_at.
func extractPinnedAndVisible(fields []map[string]any, topN int) []FieldDecision {
	var matched []map[string]any
	for _, f := range fields {
		if !boolVal(f, "pin_to_prompt") {
			continue
		}
		if !strings.EqualFold(strVal(f, "visibility"), "prompt") {
			continue
		}
		matched = append(matched, f)
	}
	sortByUpdatedDesc(matched)
	if len(matched) > topN {
		matched = matched[:topN]
	}
	result := make([]FieldDecision, 0, len(matched))
	for _, f := range matched {
		result = append(result, FieldDecision{
			Path:  strVal(f, "field_path"),
			Value: strVal(f, "value_text"),
		})
	}
	return result
}

// extractRecentVisible collects the most recently updated visibility="prompt" fields, top N.
func extractRecentVisible(fields []map[string]any, topN int) []FieldDecision {
	var matched []map[string]any
	for _, f := range fields {
		if strings.EqualFold(strVal(f, "visibility"), "prompt") {
			matched = append(matched, f)
		}
	}
	sortByUpdatedDesc(matched)
	if len(matched) > topN {
		matched = matched[:topN]
	}
	result := make([]FieldDecision, 0, len(matched))
	for _, f := range matched {
		result = append(result, FieldDecision{
			Path:  strVal(f, "field_path"),
			Value: strVal(f, "value_text"),
		})
	}
	return result
}

// extractArtifactsByKind collects fields with field_kind "artifact" or "reference".
func extractArtifactsByKind(fields []map[string]any) []FieldArtifact {
	var result []FieldArtifact
	for _, f := range fields {
		kind := strings.ToLower(strVal(f, "field_kind"))
		if kind == "artifact" || kind == "reference" {
			result = append(result, FieldArtifact{
				Path:  strVal(f, "field_path"),
				Value: strVal(f, "value_text"),
				Kind:  kind,
			})
		}
	}
	if result == nil {
		result = []FieldArtifact{}
	}
	return result
}

// extractArtifactsByPathAndKind collects fields matching path patterns AND field_kind="reference".
func extractArtifactsByPathAndKind(fields []map[string]any, patterns []string) []FieldArtifact {
	var result []FieldArtifact
	for _, f := range fields {
		if !strings.EqualFold(strVal(f, "field_kind"), "reference") {
			continue
		}
		path := strVal(f, "field_path")
		pathLower := strings.ToLower(path)
		matched := false
		for _, p := range patterns {
			if strings.Contains(pathLower, p) {
				matched = true
				break
			}
		}
		if matched {
			result = append(result, FieldArtifact{
				Path:  path,
				Value: strVal(f, "value_text"),
				Kind:  "reference",
			})
		}
	}
	if result == nil {
		result = []FieldArtifact{}
	}
	return result
}

// extractArtifactsBySingleKind collects all fields with the given field_kind.
func extractArtifactsBySingleKind(fields []map[string]any, kind string) []FieldArtifact {
	var result []FieldArtifact
	for _, f := range fields {
		if strings.EqualFold(strVal(f, "field_kind"), kind) {
			result = append(result, FieldArtifact{
				Path:  strVal(f, "field_path"),
				Value: strVal(f, "value_text"),
				Kind:  kind,
			})
		}
	}
	if result == nil {
		result = []FieldArtifact{}
	}
	return result
}

// sortByUpdatedDesc sorts fields by updated_at descending (most recent first).
func sortByUpdatedDesc(fields []map[string]any) {
	sort.Slice(fields, func(i, j int) bool {
		a := strVal(fields[i], "updated_at")
		b := strVal(fields[j], "updated_at")
		return a > b // descending
	})
}

// strVal extracts a string value from a map[string]any.
func strVal(m map[string]any, key string) string {
	v, ok := m[key]
	if !ok {
		return ""
	}
	s, ok := v.(string)
	if ok {
		return s
	}
	return ""
}

// boolVal extracts a boolean value from a map[string]any, handling bool, float64, and int types.
func boolVal(m map[string]any, key string) bool {
	v, ok := m[key]
	if !ok {
		return false
	}
	switch b := v.(type) {
	case bool:
		return b
	case float64:
		return b != 0
	case float32:
		return b != 0
	case int:
		return b != 0
	case int32:
		return b != 0
	case int64:
		return b != 0
	default:
		return false
	}
}

// --- Path B: LLM-Enhanced Episode ---

// EpisodeSignals holds the signals used to determine whether Path B should be triggered.
type EpisodeSignals struct {
	Importance     float64 // [0, 1]
	CriticScore    float64 // [0, 1], -1 if missing
	ToolCallCount  int
	DurationMs     int
	UserMark       string // "star", "consolidate", or ""
}

// maxOutcomeAppendChars limits the last_assistant_message appended to outcome.
const maxOutcomeAppendChars = 200

// Path B trigger thresholds.
const (
	PathBImportanceThreshold    = 0.7
	PathBCriticScoreThreshold   = 0.8
	PathBToolCallCountThreshold = 20
	PathBDurationMsThreshold    = 300000 // 5 minutes
)

// ExtractStructuredEpisodeFromMessages builds a StructuredEpisodeData from a list
// of consolidation messages (used by AutoMemoryWorker when no L1 snapshot is available).
// It derives title, outcome, and importance from the message content.
func ExtractStructuredEpisodeFromMessages(messages []ConsolidateMessage) StructuredEpisodeData {
	result := StructuredEpisodeData{
		KeyDecisions: []FieldDecision{},
		KeyArtifacts: []FieldArtifact{},
		Importance:   0.5,
		Confidence:   0.6,
		EpisodeKind:  "auto_memory_structured",
	}
	if len(messages) == 0 {
		return result
	}
	// Title: first user message (truncated)
	for _, m := range messages {
		if m.Role == "user" {
			result.Title = truncateRunes(m.Content, 120)
			break
		}
	}
	if result.Title == "" {
		result.Title = truncateRunes(messages[0].Content, 120)
	}
	// Outcome summary: last assistant message (truncated)
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "assistant" {
			result.OutcomeSummary = truncateRunes(messages[i].Content, 200)
			break
		}
	}
	result.Outcome = result.OutcomeSummary
	// Goal: same as title for auto-memory episodes
	result.Goal = result.Title
	return result
}

// truncateRunes truncates a string to at most max runes, appending "…" if truncated.
func truncateRunes(s string, max int) string {
	s = strings.TrimSpace(s)
	if max <= 0 || len(s) <= max {
		return s
	}
	// Count runes
	runes := 0
	for i := range s {
		if runes >= max {
			return s[:i] + "…"
		}
		runes++
	}
	return s
}

// ShouldTriggerPathB returns true if any Path B trigger condition is met.
// Simplified rule (P0): satisfy any one condition.
func ShouldTriggerPathB(s EpisodeSignals) bool {
	if s.Importance >= PathBImportanceThreshold {
		return true
	}
	if s.CriticScore >= PathBCriticScoreThreshold {
		return true
	}
	if s.ToolCallCount >= PathBToolCallCountThreshold {
		return true
	}
	if s.DurationMs >= PathBDurationMsThreshold {
		return true
	}
	if s.UserMark == "star" || s.UserMark == "consolidate" {
		return true
	}
	return false
}

// EpisodeScore computes the comprehensive score for Path B prioritization.
// When critic_score is present:
//
//	episode_score = 0.30*importance + 0.25*min(critic/0.8,1) + 0.15*min(tools/20,1) + 0.15*min(duration/300000,1) + 0.15*(user_mark?1:0)
//
// When critic_score is missing (weight redistribution):
//
//	episode_score = 0.40*importance + 0.20*min(tools/20,1) + 0.20*min(duration/300000,1) + 0.20*(user_mark?1:0)
func EpisodeScore(s EpisodeSignals) float64 {
	hasMark := 0.0
	if s.UserMark == "star" || s.UserMark == "consolidate" {
		hasMark = 1.0
	}
	toolsNorm := math.Min(float64(s.ToolCallCount)/float64(PathBToolCallCountThreshold), 1.0)
	durNorm := math.Min(float64(s.DurationMs)/float64(PathBDurationMsThreshold), 1.0)

	if s.CriticScore >= 0 {
		criticNorm := math.Min(s.CriticScore/PathBCriticScoreThreshold, 1.0)
		return 0.30*s.Importance + 0.25*criticNorm + 0.15*toolsNorm + 0.15*durNorm + 0.15*hasMark
	}
	// Critic score missing: redistribute weights
	return 0.40*s.Importance + 0.20*toolsNorm + 0.20*durNorm + 0.20*hasMark
}

// intVal extracts an int value from a map[string]any, handling float64 and int types.
func intVal(m map[string]any, key string) int {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return int(n)
	case float32:
		return int(n)
	case int:
		return n
	case int32:
		return int(n)
	case int64:
		return int(n)
	case json.Number:
		i, _ := n.Int64()
		return int(i)
	default:
		return 0
	}
}

// floatVal extracts a float64 value from a map[string]any.
func floatVal(m map[string]any, key string) float64 {
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch n := v.(type) {
	case float64:
		return n
	case float32:
		return float64(n)
	case int:
		return float64(n)
	case int64:
		return float64(n)
	default:
		return 0
	}
}
