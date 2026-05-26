package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
)

// L1MemoryCue injects pinned working-memory fields for the active L1 task in this session.
func L1MemoryCue(ctx context.Context, admin biz.SessionAdminStore, ag biz.Agent, policy biz.MemoryRuntimePolicy, sessionID string) string {
	if admin == nil || !policy.InjectL1 {
		return ""
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return ""
	}
	taskRows, err := admin.ListL1TaskRows(ctx, sessionID, strings.TrimSpace(ag.ID), "", "")
	if err != nil || len(taskRows) == 0 {
		return ""
	}
	var task map[string]any
	if json.Unmarshal(taskRows[0], &task) != nil {
		return ""
	}
	taskID := strings.TrimSpace(fmt.Sprint(task["id"]))
	if taskID == "" || taskID == "<nil>" {
		return ""
	}
	fieldRows, err := admin.ListL1FieldRows(ctx, taskID, false)
	if err != nil || len(fieldRows) == 0 {
		return formatL1TaskOnly(task)
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

	fieldMax := policy.L1FieldMaxChars
	added := 0
	for _, raw := range fieldRows {
		var row map[string]any
		if json.Unmarshal(raw, &row) != nil {
			continue
		}
		if !fieldPinnedToPrompt(row) {
			continue
		}
		path := strings.TrimSpace(fmt.Sprint(row["field_path"]))
		val := l1FieldValue(row)
		if path == "" || path == "<nil>" || val == "" {
			continue
		}
		if len(val) > fieldMax {
			val = val[:fieldMax] + "…"
		}
		fmt.Fprintf(&b, "- %s: %s\n", path, val)
		added++
	}
	if added == 0 {
		return formatL1TaskOnly(task)
	}
	return strings.TrimSpace(b.String())
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

func formatL1TaskOnly(task map[string]any) string {
	title := strings.TrimSpace(fmt.Sprint(task["task_title"]))
	goal := strings.TrimSpace(fmt.Sprint(task["task_goal"]))
	if title == "" || title == "<nil>" {
		return ""
	}
	var b strings.Builder
	b.WriteString("## L1 working memory (current task)\n")
	fmt.Fprintf(&b, "Task: %s\n", title)
	if goal != "" && goal != "<nil>" && goal != title {
		fmt.Fprintf(&b, "Goal: %s\n", goal)
	}
	return strings.TrimSpace(b.String())
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
