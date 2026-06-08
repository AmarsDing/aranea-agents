package session

import (
	"context"
	"encoding/json"
	"strings"
	"unicode/utf8"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type memoryCompactResult struct {
	summaryMarkdown string
	fromTurn        int
	toTurn          int
	didCompact      bool
}

// compactCoverage measures how well the current L1 fields cover the 6 ICS dimensions.
type compactCoverage struct {
	HasIntent     bool
	HasState      bool
	DecisionCount int
	FileCount     int
	FactCount     int
	HasPending    bool
}

// ICS computes the Information Coverage Score (0–1).
func (c compactCoverage) ICS() float64 {
	return gradedScore(c.HasIntent, 0.25) +
		gradedScore(c.HasState, 0.20) +
		gradedScoreCount(c.DecisionCount, 2, 0.20) +
		gradedScoreCount(c.FileCount, 2, 0.15) +
		gradedScoreCount(c.FactCount, 3, 0.10) +
		gradedScore(c.HasPending, 0.10)
}

func gradedScore(has bool, weight float64) float64 {
	if has {
		return weight
	}
	return 0
}

func gradedScoreCount(count, threshold int, weight float64) float64 {
	if count >= threshold {
		return weight
	}
	if count == 1 {
		return weight * 0.5
	}
	return 0
}

// shouldUseStructuredCompact decides whether Level 2 (structured) compression is appropriate.
func shouldUseStructuredCompact(coverage compactCoverage, structuredTokens, originalTokens int) bool {
	ics := coverage.ICS()
	if ics < 0.70 {
		return false
	}
	if originalTokens <= 0 {
		return true
	}
	ratio := float64(structuredTokens) / float64(originalTokens)
	return ratio <= 0.60
}

func tryMemoryCompact(ctx context.Context, body []biz.ChatMessage, reader biz.MemoryFactReader, l1Reader biz.L1AdminReader, sessionID string, lg loggateway.Logger) memoryCompactResult {
	if len(body) == 0 {
		return memoryCompactResult{}
	}

	// Collect facts from L3 semantic memory (legacy path).
	var facts []biz.MemoryFactEntry
	if reader != nil {
		loaded, err := reader.ReadSessionMemoryFacts(ctx, sessionID)
		if err != nil {
			lg.Warn("MemoryCompact: read memory facts failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
		} else {
			facts = loaded
		}
	}

	// Enrich with L1 working memory fields if available.
	if l1Reader != nil {
		l1Facts := readL1Facts(ctx, l1Reader, sessionID, lg)
		facts = append(facts, l1Facts...)
	}

	if len(facts) == 0 {
		return memoryCompactResult{}
	}

	// Build coverage from available memory facts.
	coverage := buildCompactCoverage(facts, body)
	md := buildStructuredSummary(facts, coverage)

	from := body[0].TurnNumber
	to := body[len(body)-1].TurnNumber
	return memoryCompactResult{
		summaryMarkdown: md,
		fromTurn:        from,
		toTurn:          to,
		didCompact:      true,
	}
}

// buildStructuredSummary generates a Markdown summary organized by ICS dimensions.
func buildStructuredSummary(facts []biz.MemoryFactEntry, coverage compactCoverage) string {
	var sb strings.Builder
	sb.WriteString("## Session Memory Summary\n")

	if coverage.HasIntent {
		sb.WriteString("\n### Intent\n")
		for _, f := range facts {
			if strings.EqualFold(f.Scope, "intent") {
				sb.WriteString("- " + f.Statement + "\n")
			}
		}
	}

	if coverage.HasState {
		sb.WriteString("\n### Current State\n")
		for _, f := range facts {
			if strings.EqualFold(f.Scope, "state") {
				sb.WriteString("- " + f.Statement + "\n")
			}
		}
	}

	if coverage.DecisionCount > 0 {
		sb.WriteString("\n### Decisions\n")
		for _, f := range facts {
			if strings.EqualFold(f.Scope, "decision") {
				sb.WriteString("- " + f.Statement + "\n")
			}
		}
	}

	if coverage.FileCount > 0 {
		sb.WriteString("\n### Files\n")
		for _, f := range facts {
			if strings.EqualFold(f.Scope, "file") {
				sb.WriteString("- " + f.Statement + "\n")
			}
		}
	}

	// Always include key facts section.
	sb.WriteString("\n### Key Facts\n")
	for _, f := range facts {
		scope := strings.ToLower(strings.TrimSpace(f.Scope))
		if scope != "intent" && scope != "state" && scope != "decision" && scope != "file" && scope != "pending" {
			sb.WriteString("- " + f.Statement)
			if f.Scope != "" {
				sb.WriteString(" _[" + f.Scope + "]_")
			}
			sb.WriteString("\n")
		}
	}

	if coverage.HasPending {
		sb.WriteString("\n### Pending\n")
		for _, f := range facts {
			if strings.EqualFold(f.Scope, "pending") {
				sb.WriteString("- " + f.Statement + "\n")
			}
		}
	}

	return sb.String()
}

// buildCompactCoverage builds an ICS coverage assessment from memory facts and messages.
func buildCompactCoverage(facts []biz.MemoryFactEntry, body []biz.ChatMessage) compactCoverage {
	var cov compactCoverage
	for _, f := range facts {
		scope := strings.ToLower(strings.TrimSpace(f.Scope))
		switch scope {
		case "intent":
			cov.HasIntent = true
		case "state":
			cov.HasState = true
		case "decision":
			cov.DecisionCount++
		case "file":
			cov.FileCount++
		case "pending":
			cov.HasPending = true
		default:
			cov.FactCount++
		}
	}
	return cov
}

// readL1Facts reads L1 working memory tasks and fields, converting them into
// MemoryFactEntry items for use in MemoryCompact summaries.
func readL1Facts(ctx context.Context, l1Reader biz.L1AdminReader, sessionID string, lg loggateway.Logger) []biz.MemoryFactEntry {
	taskRows, err := l1Reader.ListL1TaskRows(ctx, sessionID, "", "", "")
	if err != nil {
		lg.Warn("MemoryCompact: list L1 tasks failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(err))
		return nil
	}
	if len(taskRows) == 0 {
		return nil
	}

	var facts []biz.MemoryFactEntry
	for _, raw := range taskRows {
		taskObj := decodeMap(raw)
		if taskObj == nil {
			continue
		}

		taskID, _ := taskObj["id"].(string)
		taskTitle, _ := taskObj["task_title"].(string)
		status, _ := taskObj["status"].(string)
		if taskID == "" {
			continue
		}

		// Add task as a pending/intent fact.
		scope := "pending"
		if status == "active" {
			scope = "intent"
		}
		if taskTitle != "" {
			facts = append(facts, biz.MemoryFactEntry{
				Statement: taskTitle,
				Scope:     scope,
			})
		}

		// Read fields for this task.
		fieldRows, fieldErr := l1Reader.ListL1FieldRows(ctx, taskID, false)
		if fieldErr != nil {
			lg.Warn("MemoryCompact: list L1 fields failed", loggateway.StepID("session.compress"), loggateway.SessionID(sessionID), loggateway.Err(fieldErr))
			continue
		}
		for _, fieldRaw := range fieldRows {
			fieldObj := decodeMap(fieldRaw)
			if fieldObj == nil {
				continue
			}
			fieldPath, _ := fieldObj["field_path"].(string)
			valueText, _ := fieldObj["value_text"].(string)
			fieldKind, _ := fieldObj["field_kind"].(string)
			if fieldPath == "" {
				continue
			}
			statement := fieldPath
			if valueText != "" {
				statement = fieldPath + ": " + truncateFieldText(valueText, maxFieldTextChars)
			}
			// Use field_kind value for scope mapping when available; fall back to path-based heuristics.
			scope := mapFieldKindValueToScope(fieldKind)
			if scope == "" {
				scope = mapFieldKindToScope(fieldPath)
			}
			facts = append(facts, biz.MemoryFactEntry{
				Statement: statement,
				Scope:     scope,
			})
		}
	}
	return facts
}

// decodeMap is a lightweight JSON object decoder for raw byte slices.
func decodeMap(raw []byte) map[string]any {
	if len(raw) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil
	}
	return m
}

// mapFieldKindToScope maps L1 field paths to ICS scope categories.
func mapFieldKindToScope(fieldPath string) string {
	p := strings.ToLower(fieldPath)
	switch {
	case strings.Contains(p, "intent") || strings.Contains(p, "goal"):
		return "intent"
	case strings.Contains(p, "state") || strings.Contains(p, "status"):
		return "state"
	case strings.Contains(p, "decision") || strings.Contains(p, "choice"):
		return "decision"
	case strings.Contains(p, "file") || strings.Contains(p, "path"):
		return "file"
	case strings.Contains(p, "pending") || strings.Contains(p, "todo") || strings.Contains(p, "task"):
		return "pending"
	default:
		return "fact"
	}
}

// mapFieldKindValueToScope maps field_kind enum values to ICS scope categories.
// When a field has an explicit field_kind, it provides more accurate scope mapping
// than path-based heuristics.
func mapFieldKindValueToScope(fieldKind string) string {
	switch strings.ToLower(strings.TrimSpace(fieldKind)) {
	case "decision":
		return "decision"
	case "artifact":
		return "file"
	case "progress":
		return "state"
	case "constraint":
		return "intent"
	default:
		return ""
	}
}

// truncateFieldText truncates text to maxLen runes with ellipsis.
func truncateFieldText(text string, maxLen int) string {
	if utf8.RuneCountInString(text) <= maxLen {
		return text
	}
	runes := []rune(text)
	return string(runes[:maxLen]) + "…"
}
