package preview

import (
	"encoding/json"
	"fmt"
	"strings"
)

// 渠道交互门卡片（确认/澄清）。按钮 value 只携带短键（reply/q/opt），
// 完整结构化 token 由 service 层映射，避免卡片协议泄漏内部格式。
const (
	GateCardActionConfirm = "gate_confirm"
	GateCardActionClarify = "gate_clarify"

	gateCardArgsMaxRunes    = 600
	gateCardOptionMaxRunes  = 30
	gateCardQuestionMaxRune = 200
)

// ConfirmGateCardParams 确认卡片参数（ToolName/ArgsSummary 由调用方预处理和截断）。
type ConfirmGateCardParams struct {
	StepID      string
	SessionID   string
	ToolName    string
	ArgsSummary string
}

// gateButtonValue 序列化按钮回调 value。
func gateButtonValue(action, stepID, sessionID string, extra map[string]any) map[string]any {
	v := map[string]any{
		"action":     action,
		"step_id":    stepID,
		"session_id": sessionID,
	}
	for k, val := range extra {
		v[k] = val
	}
	return v
}

func gateButton(text, btnType string, value map[string]any) map[string]any {
	return map[string]any{
		"tag":  "button",
		"type": btnType,
		"text": map[string]any{
			"tag":     "plain_text",
			"content": text,
		},
		"value": value,
	}
}

func gateActionRow(buttons ...any) map[string]any {
	return map[string]any{
		"tag":     "action",
		"actions": buttons,
	}
}

func gateMarkdown(content string) map[string]any {
	return map[string]any{
		"tag": "div",
		"text": map[string]any{
			"tag":     "lark_md",
			"content": content,
		},
	}
}

func gateCardShell(template, title string, elements []any) (string, error) {
	card := map[string]any{
		"config": map[string]any{"wide_screen_mode": true, "update_multi": true},
		"header": map[string]any{
			"template": template,
			"title": map[string]any{
				"tag":     "plain_text",
				"content": title,
			},
		},
		"elements": elements,
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}

// BuildFeishuConfirmGateCardJSON 构建工具确认卡片（4 键，与 Web ConfirmBlock 对齐）。
func BuildFeishuConfirmGateCardJSON(p ConfirmGateCardParams) (string, error) {
	toolName := strings.TrimSpace(p.ToolName)
	if toolName == "" {
		toolName = "未知工具"
	}
	var body strings.Builder
	body.WriteString(fmt.Sprintf("工具 **%s** 需要确认后执行", escapeLarkMD(toolName)))
	if args := strings.TrimSpace(p.ArgsSummary); args != "" {
		body.WriteString("\n```\n")
		body.WriteString(truncateRunes(args, gateCardArgsMaxRunes))
		body.WriteString("\n```")
	}
	buttons := []any{
		gateButton("允许本次", "primary", gateButtonValue(GateCardActionConfirm, p.StepID, p.SessionID, map[string]any{"reply": "approve"})),
		gateButton("拒绝", "danger", gateButtonValue(GateCardActionConfirm, p.StepID, p.SessionID, map[string]any{"reply": "deny"})),
		gateButton("会话内始终允许", "default", gateButtonValue(GateCardActionConfirm, p.StepID, p.SessionID, map[string]any{"reply": "approve_session"})),
		gateButton("始终允许", "default", gateButtonValue(GateCardActionConfirm, p.StepID, p.SessionID, map[string]any{"reply": "approve_always"})),
	}
	elements := []any{
		gateMarkdown(body.String()),
		gateActionRow(buttons...),
	}
	return gateCardShell("orange", "⚠️ 需要确认 · "+truncateRunes(toolName, 40), elements)
}

// GateResultCardParams 交互门终态卡片参数（确认/澄清共用）。
type GateResultCardParams struct {
	Template string   // green / red / grey / orange
	Title    string   // 如 "✓ 已批准 · client_open_url"
	Lines    []string // markdown 行
}

// BuildFeishuGateResultCardJSON 构建终态卡片（无按钮）。
func BuildFeishuGateResultCardJSON(p GateResultCardParams) (string, error) {
	template := strings.TrimSpace(p.Template)
	if template == "" {
		template = "grey"
	}
	elements := make([]any, 0, len(p.Lines))
	for _, line := range p.Lines {
		if strings.TrimSpace(line) == "" {
			continue
		}
		elements = append(elements, gateMarkdown(line))
	}
	return gateCardShell(template, p.Title, elements)
}

// ClarifyGateQuestion 澄清卡片单题参数（service 层从 biz 信封映射；只含 single 模式）。
type ClarifyGateQuestion struct {
	Question    string
	Options     []string
	Recommended []string
}

// ClarifyGateCardParams 澄清卡片参数。Selections 与 Questions 对齐（空 = 未作答）。
// Interactive=false 时渲染为纯文本说明卡（多选/无选项等降级场景），
// 用户经自由文本消息作答（resolveClarificationFreeText 已有路径）。
type ClarifyGateCardParams struct {
	StepID      string
	SessionID   string
	Questions   []ClarifyGateQuestion
	Selections  [][]string
	Interactive bool
}

// BuildFeishuClarifyGateCardJSON 构建澄清卡片。
func BuildFeishuClarifyGateCardJSON(p ClarifyGateCardParams) (string, error) {
	elements := make([]any, 0, len(p.Questions)*2+2)
	for i, q := range p.Questions {
		var qmd strings.Builder
		qmd.WriteString(fmt.Sprintf("**Q%d. %s**", i+1, escapeLarkMD(truncateRunes(strings.TrimSpace(q.Question), gateCardQuestionMaxRune))))
		if !p.Interactive {
			for _, opt := range q.Options {
				mark := ""
				if containsString(q.Recommended, opt) {
					mark = " ★推荐"
				}
				qmd.WriteString(fmt.Sprintf("\n- %s%s", escapeLarkMD(opt), mark))
			}
		}
		elements = append(elements, gateMarkdown(qmd.String()))
		if !p.Interactive {
			continue
		}
		var selected []string
		if i < len(p.Selections) {
			selected = p.Selections[i]
		}
		buttons := make([]any, 0, len(q.Options))
		for _, opt := range q.Options {
			label := truncateRunes(opt, gateCardOptionMaxRunes)
			btnType := "default"
			switch {
			case containsString(selected, opt):
				label = "✓ " + label
				btnType = "primary"
			case containsString(q.Recommended, opt):
				label = "★ " + label
			}
			buttons = append(buttons, gateButton(label, btnType, gateButtonValue(
				GateCardActionClarify, p.StepID, p.SessionID,
				map[string]any{"q": i, "opt": opt},
			)))
		}
		elements = append(elements, gateActionRow(buttons...))
	}
	hint := "点击选项作答；也可以直接回复文字作答。"
	if !p.Interactive {
		hint = "请直接回复消息作答（可引用选项或自由描述）。"
	}
	elements = append(elements, gateMarkdown("<font color='grey'>"+hint+"</font>"))
	return gateCardShell("blue", "❓ 需要你的确认", elements)
}

func containsString(list []string, target string) bool {
	for _, v := range list {
		if v == target {
			return true
		}
	}
	return false
}
