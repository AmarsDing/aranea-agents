package preview

import (
	"encoding/json"
	"strings"
	"testing"
)

// parseGateCard 解析卡片 JSON 为通用结构（测试断言用）。
func parseGateCard(t *testing.T, cardJSON string) map[string]any {
	t.Helper()
	var card map[string]any
	if err := json.Unmarshal([]byte(cardJSON), &card); err != nil {
		t.Fatalf("card JSON unparseable: %v\n%s", err, cardJSON)
	}
	return card
}

// gateCardButtons 收集卡片中全部按钮元素。
func gateCardButtons(t *testing.T, card map[string]any) []map[string]any {
	t.Helper()
	var out []map[string]any
	elements, _ := card["elements"].([]any)
	for _, el := range elements {
		row, _ := el.(map[string]any)
		if row["tag"] != "action" {
			continue
		}
		actions, _ := row["actions"].([]any)
		for _, a := range actions {
			btn, _ := a.(map[string]any)
			if btn["tag"] == "button" {
				out = append(out, btn)
			}
		}
	}
	return out
}

func buttonValue(t *testing.T, btn map[string]any) map[string]any {
	t.Helper()
	v, _ := btn["value"].(map[string]any)
	if v == nil {
		t.Fatalf("button missing value: %+v", btn)
	}
	return v
}

func TestBuildFeishuConfirmGateCardJSON(t *testing.T) {
	card, err := BuildFeishuConfirmGateCardJSON(ConfirmGateCardParams{
		StepID:      "st1",
		SessionID:   "s1",
		ToolName:    "shell_exec",
		ArgsSummary: `{"cmd":"ls"}`,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	parsed := parseGateCard(t, card)

	header := parsed["header"].(map[string]any)
	if header["template"] != "orange" {
		t.Fatalf("template = %v, want orange", header["template"])
	}

	buttons := gateCardButtons(t, parsed)
	if len(buttons) != 4 {
		t.Fatalf("buttons = %d, want 4", len(buttons))
	}
	wantReplies := []string{"approve", "deny", "approve_session", "approve_always"}
	for i, btn := range buttons {
		v := buttonValue(t, btn)
		if v["action"] != GateCardActionConfirm {
			t.Fatalf("button %d action = %v", i, v["action"])
		}
		if v["step_id"] != "st1" || v["session_id"] != "s1" {
			t.Fatalf("button %d ids = %v/%v", i, v["step_id"], v["session_id"])
		}
		if v["reply"] != wantReplies[i] {
			t.Fatalf("button %d reply = %v, want %v", i, v["reply"], wantReplies[i])
		}
	}
}

func TestBuildFeishuClarifyGateCardJSON_Interactive(t *testing.T) {
	card, err := BuildFeishuClarifyGateCardJSON(ClarifyGateCardParams{
		StepID:    "st1",
		SessionID: "s1",
		Questions: []ClarifyGateQuestion{
			{Question: "平台？", Options: []string{"Web", "iOS"}, Recommended: []string{"Web"}},
			{Question: "风格？", Options: []string{"简约", "华丽"}},
		},
		Selections:  [][]string{{"iOS"}, nil},
		Interactive: true,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	parsed := parseGateCard(t, card)

	header := parsed["header"].(map[string]any)
	if header["template"] != "blue" {
		t.Fatalf("template = %v, want blue", header["template"])
	}

	buttons := gateCardButtons(t, parsed)
	if len(buttons) != 4 {
		t.Fatalf("buttons = %d, want 4 (2 questions x 2 options)", len(buttons))
	}
	// Q1 的 iOS 被选中：primary + ✓ 前缀。
	selected := buttonValue(t, buttons[1])
	if selected["q"] != float64(0) || selected["opt"] != "iOS" {
		t.Fatalf("button 1 value = %+v", selected)
	}
	text := buttons[1]["text"].(map[string]any)["content"].(string)
	if !strings.HasPrefix(text, "✓ ") {
		t.Fatalf("selected button text = %q, want ✓ prefix", text)
	}
	if buttons[1]["type"] != "primary" {
		t.Fatalf("selected button type = %v, want primary", buttons[1]["type"])
	}
	// Q1 的 Web 是推荐项：★ 前缀。
	recText := buttons[0]["text"].(map[string]any)["content"].(string)
	if !strings.HasPrefix(recText, "★ ") {
		t.Fatalf("recommended button text = %q, want ★ prefix", recText)
	}
}

func TestBuildFeishuClarifyGateCardJSON_DegradedPlainText(t *testing.T) {
	card, err := BuildFeishuClarifyGateCardJSON(ClarifyGateCardParams{
		StepID:    "st1",
		SessionID: "s1",
		Questions: []ClarifyGateQuestion{
			{Question: "功能？", Options: []string{"A", "B", "C"}, Recommended: []string{"A"}},
		},
		Interactive: false,
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	parsed := parseGateCard(t, card)
	if got := gateCardButtons(t, parsed); len(got) != 0 {
		t.Fatalf("degraded card must have no buttons, got %d", len(got))
	}
	if !strings.Contains(card, "★推荐") {
		t.Fatalf("degraded card must render options with ★推荐 mark")
	}
	if !strings.Contains(card, "请直接回复消息作答") {
		t.Fatalf("degraded card must render free-text hint")
	}
}

func TestBuildFeishuGateResultCardJSON(t *testing.T) {
	card, err := BuildFeishuGateResultCardJSON(GateResultCardParams{
		Template: "green",
		Title:    "✓ 已批准 · shell_exec",
		Lines:    []string{"第一行", "", "  ", "第二行"},
	})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	parsed := parseGateCard(t, card)
	header := parsed["header"].(map[string]any)
	if header["template"] != "green" {
		t.Fatalf("template = %v, want green", header["template"])
	}
	// 空行被过滤：4 行输入 → 2 个 element。
	elements, _ := parsed["elements"].([]any)
	if len(elements) != 2 {
		t.Fatalf("elements = %d, want 2 (blank lines filtered)", len(elements))
	}
	// 终态卡无按钮。
	if got := gateCardButtons(t, parsed); len(got) != 0 {
		t.Fatalf("result card must have no buttons, got %d", len(got))
	}
	// 模板缺省回退 grey。
	card, err = BuildFeishuGateResultCardJSON(GateResultCardParams{Title: "t", Lines: []string{"x"}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	parsed = parseGateCard(t, card)
	if parsed["header"].(map[string]any)["template"] != "grey" {
		t.Fatalf("default template must be grey")
	}
}
