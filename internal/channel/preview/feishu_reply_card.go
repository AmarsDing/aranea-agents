package preview

import (
	"encoding/json"
	"strings"
)

const (
	imSectionReasoningLabel = "【思考过程】"
	imSectionBodyLabel      = "【正文】"
)

// FormatIMSectionedReply builds labeled plain text for IM when reasoning and body are separate.
func FormatIMSectionedReply(platform, reasoning, body string) string {
	var parts []string
	if r := strings.TrimSpace(FormatRenderedTranscriptForIM(platform, reasoning)); r != "" {
		parts = append(parts, imSectionReasoningLabel+"\n"+r)
	}
	if b := strings.TrimSpace(FormatAssistantReplyForIM(platform, body)); b != "" {
		parts = append(parts, imSectionBodyLabel+"\n"+b)
	}
	return strings.TrimSpace(strings.Join(parts, "\n\n"))
}

// ReasoningMarkdownFromOptions reads reasoning_markdown / reasoning_content from message options_json.
func ReasoningMarkdownFromOptions(optionsJSON string) string {
	optionsJSON = strings.TrimSpace(optionsJSON)
	if optionsJSON == "" || optionsJSON == "{}" {
		return ""
	}
	var opts struct {
		ReasoningMarkdown string `json:"reasoning_markdown"`
		ReasoningContent  string `json:"reasoning_content"`
	}
	if json.Unmarshal([]byte(optionsJSON), &opts) != nil {
		return ""
	}
	if v := strings.TrimSpace(opts.ReasoningMarkdown); v != "" {
		return v
	}
	return strings.TrimSpace(opts.ReasoningContent)
}

// BuildFeishuChannelReplyCardJSON renders reasoning/body in a Card 2.0 (like escalation notice).
func BuildFeishuChannelReplyCardJSON(reasoning, body string) (string, error) {
	elements := make([]any, 0, 2)
	if r := strings.TrimSpace(reasoning); r != "" {
		elements = append(elements, map[string]any{
			"tag":        "markdown",
			"element_id": "reply_reasoning",
			"content":    "**思考过程**\n" + truncateRunes(r, 1200),
		})
	}
	if b := strings.TrimSpace(body); b != "" {
		elements = append(elements, map[string]any{
			"tag":        "markdown",
			"element_id": "reply_body",
			"content":    "**正文**\n" + truncateRunes(b, 4000),
		})
	}
	if len(elements) == 0 {
		return "", nil
	}
	card := map[string]any{
		"schema": "2.0",
		"config": map[string]any{
			"update_multi":     true,
			"wide_screen_mode": true,
		},
		"header": map[string]any{
			"template": "blue",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "Agent 回复",
			},
		},
		"body": map[string]any{
			"direction": "vertical",
			"elements":  elements,
		},
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
