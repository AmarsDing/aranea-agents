package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"aranea-agents/internal/biz"
)

// L1CueResult holds the L1 cue string and the pinned field values for cross-layer dedup.
type L1CueResult struct {
	Cue         string
	FieldValues []string // values of pinned L1 fields (for L3 cross-layer dedup)
}

// L1MemoryCue injects pinned working-memory fields for the active L1 task in this session.
func L1MemoryCue(ctx context.Context, l1Reader biz.L1AdminReader, ag biz.Agent, policy biz.MemoryRuntimePolicy, sessionID string) *L1CueResult {
	if l1Reader == nil || !policy.InjectL1 {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	taskRows, err := l1Reader.ListL1TaskRows(ctx, sessionID, strings.TrimSpace(ag.ID), "", "")
	if err != nil || len(taskRows) == 0 {
		return nil
	}
	var task map[string]any
	if json.Unmarshal(taskRows[0], &task) != nil {
		return nil
	}
	taskID := strings.TrimSpace(fmt.Sprint(task["id"]))
	if taskID == "" || taskID == "<nil>" {
		return nil
	}
	fieldRows, err := l1Reader.ListL1FieldRows(ctx, taskID, false)
	if err != nil || len(fieldRows) == 0 {
		return formatL1TaskOnlyResult(task)
	}

	// Collect pinned fields into a slice for budget filtering.
	fieldMax := policy.L1FieldMaxChars
	type pinnedField struct {
		path string
		val  string
		est  int
	}
	var pinnedFields []pinnedField
	for _, raw := range fieldRows {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		if !fieldPinnedToPrompt(row) {
			continue
		}
		if fieldExpired(row) {
			continue
		}
		path := strings.TrimSpace(fmt.Sprint(row["field_path"]))
		val := l1FieldValue(row)
		if path == "" || path == "<nil>" || val == "" {
			continue
		}
		if len(val) > fieldMax {
			val = safeTruncate(val, fieldMax)
		}
		pinnedFields = append(pinnedFields, pinnedField{
			path: path,
			val:  val,
			est:  fieldTokenEstimate(row),
		})
	}
	if len(pinnedFields) == 0 {
		return formatL1TaskOnlyResult(task)
	}

	// Apply budget filter: include fields until budget exhausted.
	budgetTokens := policy.L1BudgetTokens
	if budgetTokens > 0 {
		var totalEstimate int
		for i := 0; i < len(pinnedFields); i++ {
			if totalEstimate+pinnedFields[i].est > budgetTokens {
				pinnedFields = pinnedFields[:i]
				break
			}
			totalEstimate += pinnedFields[i].est
		}
	}
	if len(pinnedFields) == 0 {
		return formatL1TaskOnlyResult(task)
	}

	var b strings.Builder
	title := strings.TrimSpace(fmt.Sprint(task["task_title"]))
	goal := strings.TrimSpace(fmt.Sprint(task["task_goal"]))
	b.WriteString("## L1 working memory (current task)\n")
	if title != "" && title != "<nil>" {
		fmt.Fprintf(&b, "Task: %s\n", title)
	}
	if goal != "" && goal != "<nil>" && goal != title {
		fmt.Fprintf(&b, "Goal: %s\n", goal)
	}
	b.WriteString("Pinned fields:\n")
	fieldValues := make([]string, 0, len(pinnedFields))
	for _, f := range pinnedFields {
		fmt.Fprintf(&b, "- %s: %s\n", f.path, f.val)
		fieldValues = append(fieldValues, f.val)
	}
	return &L1CueResult{Cue: strings.TrimSpace(b.String()), FieldValues: fieldValues}
}

func fieldPinnedToPrompt(row map[string]any) bool {
	switch v := row["pin_to_prompt"].(type) {
	case bool:
		return v
	case float64:
		return v != 0
	case int:
		return v != 0
	case json.Number:
		n, _ := v.Int64()
		return n != 0
	default:
		return fmt.Sprint(v) == "true" || fmt.Sprint(v) == "1"
	}
}

func formatL1TaskOnlyResult(task map[string]any) *L1CueResult {
	title := strings.TrimSpace(fmt.Sprint(task["task_title"]))
	goal := strings.TrimSpace(fmt.Sprint(task["task_goal"]))
	if title == "" || title == "<nil>" {
		return nil
	}
	var b strings.Builder
	b.WriteString("## L1 working memory (current task)\n")
	fmt.Fprintf(&b, "Task: %s\n", title)
	if goal != "" && goal != "<nil>" && goal != title {
		fmt.Fprintf(&b, "Goal: %s\n", goal)
	}
	return &L1CueResult{Cue: strings.TrimSpace(b.String())}
}

func l1FieldValue(row map[string]any) string {
	if v := strings.TrimSpace(fmt.Sprint(row["value_text"])); v != "" && v != "<nil>" {
		return v
	}
	if v := strings.TrimSpace(fmt.Sprint(row["preview"])); v != "" && v != "<nil>" {
		return v
	}
	if v := strings.TrimSpace(fmt.Sprint(row["value_json"])); v != "" && v != "<nil>" && v != "{}" {
		return v
	}
	return ""
}

func fieldTokenEstimate(row map[string]any) int {
	switch v := row["token_estimate"].(type) {
	case float64:
		return int(v)
	case int:
		return v
	case json.Number:
		n, _ := v.Int64()
		return int(n)
	default:
		return 0
	}
}

// fieldExpired returns true if the field has a non-empty expires_at that is in the past.
func fieldExpired(row map[string]any) bool {
	exp := strings.TrimSpace(fmt.Sprint(row["expires_at"]))
	if exp == "" || exp == "<nil>" {
		return false
	}
	t, err := time.Parse(time.RFC3339, exp)
	if err != nil {
		// Try RFC3339Nano as fallback (SQLite stores with nanosecond precision).
		t, err = time.Parse(time.RFC3339Nano, exp)
		if err != nil {
			return false
		}
	}
	return time.Now().UTC().After(t)
}
