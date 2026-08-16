package agent

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// L1CueResult holds the L1 cue string and the pinned field values for cross-layer dedup.
type L1CueResult struct {
	Cue         string
	FieldValues []string // values of pinned L1 fields (for L3 cross-layer dedup)
}

// L1MemoryCue injects pinned working-memory fields for the active L1 task in this session.
func L1MemoryCue(ctx context.Context, l1Reader biz.L1AdminReader, ag biz.Agent, policy biz.MemoryRuntimePolicy, sessionID string, lg loggateway.Logger) *L1CueResult {
	if l1Reader == nil || !policy.InjectL1 {
		return nil
	}
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil
	}
	taskRows, err := l1Reader.ListL1TaskRows(ctx, sessionID, strings.TrimSpace(ag.ID), "", "")
	if err != nil {
		lg.Warn("L1 memory query failed", loggateway.StepID("agent.memory_query_fail"), loggateway.Err(err))
		return nil
	}
	if len(taskRows) == 0 {
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
	if err != nil {
		lg.Warn("L1 field query failed", loggateway.StepID("agent.memory_query_fail"), loggateway.Err(err))
		return formatL1TaskOnlyResult(task)
	}
	if len(fieldRows) == 0 {
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
	rowStatus := strings.TrimSpace(fmt.Sprint(task["status"]))
	b.WriteString("## L1 working memory (current task)\n")
	if title != "" && title != "<nil>" {
		fmt.Fprintf(&b, "Task: %s\n", title)
	}
	if goal != "" && goal != "<nil>" && goal != title {
		fmt.Fprintf(&b, "Goal: %s\n", goal)
	}
	if board := parseL1TaskBoard(task); board != nil {
		board.appendTo(&b, rowStatus)
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
	rowStatus := strings.TrimSpace(fmt.Sprint(task["status"]))
	if title == "" || title == "<nil>" {
		return nil
	}
	var b strings.Builder
	b.WriteString("## L1 working memory (current task)\n")
	fmt.Fprintf(&b, "Task: %s\n", title)
	if goal != "" && goal != "<nil>" && goal != title {
		fmt.Fprintf(&b, "Goal: %s\n", goal)
	}
	if board := parseL1TaskBoard(task); board != nil {
		board.appendTo(&b, rowStatus)
	}
	return &L1CueResult{Cue: strings.TrimSpace(b.String())}
}

// l1TaskBoard 是跨会话长任务的结构化进度快照（任务状态表层），
// 承载于 memory_l1_tasks.metadata_json["task_board"]：
//
//	{"status":"...","done":["..."],"next":"...","blockers":["..."]}
//
// 与 L1 pinned fields 的关系：pinned fields 记"关键事实"，board 记"做到哪了"。
// 全部字段可选；全空视为不存在（不渲染）。
type l1TaskBoard struct {
	Status   string
	Done     []string
	Next     string
	Blockers []string
}

const (
	// 防御上限：metadata_json 被写爆时 board 注入不得撑爆 prompt。
	l1TaskBoardMaxItems     = 8   // done/blockers 各自条目上限
	l1TaskBoardMaxItemRunes = 160 // 单条目 rune 上限
	l1TaskBoardMaxLineRunes = 200 // status/next 单行上限
)

// parseL1TaskBoard 从任务行 map（scanL1TaskRow 产物）解析 task_board。
// metadata_json 缺失/非法/无 task_board 键/全空均返回 nil。
func parseL1TaskBoard(task map[string]any) *l1TaskBoard {
	raw := strings.TrimSpace(fmt.Sprint(task["metadata_json"]))
	if raw == "" || raw == "<nil>" {
		return nil
	}
	var meta struct {
		Board json.RawMessage `json:"task_board"`
	}
	if json.Unmarshal([]byte(raw), &meta) != nil || len(meta.Board) == 0 {
		return nil
	}
	var parsed struct {
		Status   string   `json:"status"`
		Done     []string `json:"done"`
		Next     string   `json:"next"`
		Blockers []string `json:"blockers"`
	}
	if json.Unmarshal(meta.Board, &parsed) != nil {
		return nil
	}
	board := &l1TaskBoard{
		Status:   truncateBoardRunes(parsed.Status, l1TaskBoardMaxLineRunes),
		Next:     truncateBoardRunes(parsed.Next, l1TaskBoardMaxLineRunes),
		Done:     cleanBoardList(parsed.Done),
		Blockers: cleanBoardList(parsed.Blockers),
	}
	if board.empty() {
		return nil
	}
	return board
}

func (b *l1TaskBoard) empty() bool {
	return b.Status == "" && b.Next == "" && len(b.Done) == 0 && len(b.Blockers) == 0
}

// appendTo 将 board 渲染到 L1 cue（Task/Goal 之后、Pinned fields 之前）。
// rowStatus 为任务行级状态：board 无 status 且行状态非常态（非 active）时回退展示。
func (b *l1TaskBoard) appendTo(sb *strings.Builder, rowStatus string) {
	status := b.Status
	if status == "" && rowStatus != "" && rowStatus != "active" && rowStatus != "<nil>" {
		status = rowStatus
	}
	if status != "" {
		fmt.Fprintf(sb, "Status: %s\n", status)
	}
	if len(b.Done) > 0 {
		sb.WriteString("Progress:\n")
		for _, d := range b.Done {
			fmt.Fprintf(sb, "- %s\n", d)
		}
	}
	if b.Next != "" {
		fmt.Fprintf(sb, "Next: %s\n", b.Next)
	}
	if len(b.Blockers) > 0 {
		sb.WriteString("Blockers:\n")
		for _, bl := range b.Blockers {
			fmt.Fprintf(sb, "- %s\n", bl)
		}
	}
}

func cleanBoardList(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		s = truncateBoardRunes(s, l1TaskBoardMaxItemRunes)
		if s == "" {
			continue
		}
		out = append(out, s)
		if len(out) >= l1TaskBoardMaxItems {
			break
		}
	}
	return out
}

func truncateBoardRunes(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	r := []rune(s)
	if len(r) > maxRunes {
		return string(r[:maxRunes]) + "…"
	}
	return s
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
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil && n > 0 {
			return n
		}
		return 0
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
