package preview

import (
	"encoding/json"
	"fmt"
)

func escalateCallbackValue(action, sessionRunID, sessionID string) map[string]any {
	return map[string]any{
		"action":         action,
		"session_run_id": sessionRunID,
		"session_id":     sessionID,
	}
}

func escalateButton(action, sessionRunID, sessionID, label, btnType, elementID string) map[string]any {
	return map[string]any{
		"tag":        "button",
		"element_id": elementID,
		"type":       btnType,
		"text": map[string]any{
			"tag":     "plain_text",
			"content": label,
		},
		"behaviors": []any{
			map[string]any{
				"type":  "callback",
				"value": escalateCallbackValue(action, sessionRunID, sessionID),
			},
		},
	}
}

// BuildFeishuEscalateCardJSON builds IM card for soft-budget escalation notice (CC-R-02).
// Uses Card JSON 2.0 so card.action.trigger callbacks work over WS / webhook.
func BuildFeishuEscalateCardJSON(sessionRunID, sessionID, webURL string) (string, error) {
	body := "任务处理时间较长，是否转入后台继续？"
	if webURL != "" {
		body += fmt.Sprintf("\n[查看 Web 会话](%s)", webURL)
	}
	card := map[string]any{
		"schema": "2.0",
		"config": map[string]any{
			"update_multi":     true,
			"wide_screen_mode": true,
		},
		"header": map[string]any{
			"template": "orange",
			"title": map[string]any{
				"tag":     "plain_text",
				"content": "后台继续执行？",
			},
		},
		"body": map[string]any{
			"direction": "vertical",
			"elements": []any{
				map[string]any{
					"tag":        "markdown",
					"element_id": "escalate_body",
					"content":    body,
				},
				map[string]any{
					"tag":        "column_set",
					"element_id": "escalate_actions",
					"flex_mode":  "flow",
					"columns": []any{
						map[string]any{
							"tag":            "column",
							"width":          "auto",
							"vertical_align": "top",
							"elements": []any{
								escalateButton("background", sessionRunID, sessionID, "后台继续", "primary_filled", "btn_background"),
							},
						},
						map[string]any{
							"tag":            "column",
							"width":          "auto",
							"vertical_align": "top",
							"elements": []any{
								escalateButton("cancel", sessionRunID, sessionID, "取消执行", "default", "btn_cancel"),
							},
						},
					},
				},
				map[string]any{
					"tag":        "markdown",
					"element_id": "escalate_hint",
					"content":    "也可回复 /background 或 /cancel",
				},
				map[string]any{
					"tag":        "markdown",
					"element_id": "escalate_run_id",
					"content":    fmt.Sprintf("run_id: `%s`", sessionRunID),
				},
			},
		},
	}
	raw, err := json.Marshal(card)
	if err != nil {
		return "", err
	}
	return string(raw), nil
}
