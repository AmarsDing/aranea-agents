// GOAI 零人工运维闭环模拟数据验证。
// 流程：模拟告警 → 告警处理 Agent 分析 → 故障诊断 Agent 诊断 → 验证输出结构完整性。
// 验证点：
//  1. 告警处理 Agent 输出《告警分析单》（摘要/严重度评估/原因/建议/后续）
//  2. 故障诊断 Agent 输出《故障诊断报告》（症状/根因+置信度/排查步骤/解决方案/验证/预防）
//  3. 命令生成专家仅返回 JSON {command, explanation}
//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"strings"
	"time"
)

const baseURL = "http://localhost:8000"

type client struct {
	hc *http.Client
}

func newClient() *client {
	jar, _ := cookiejar.New(nil)
	return &client{hc: &http.Client{Jar: jar, Timeout: 180 * time.Second}}
}

func (c *client) do(method, path string, body any) (int, []byte) {
	var rdr io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequest(method, baseURL+path, rdr)
	if err != nil {
		fatal("build request", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		fatal(method+" "+path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, data
}

func fatal(step string, err error) {
	fmt.Fprintf(os.Stderr, "FATAL %s: %v\n", step, err)
	os.Exit(1)
}

var passCount, failCount int

func check(name string, ok bool, detail string) {
	if ok {
		passCount++
		fmt.Printf("  PASS %s\n", name)
	} else {
		failCount++
		fmt.Printf("  FAIL %s — %s\n", name, detail)
	}
}

func agentIDByKey(c *client, key string) string {
	st, body := c.do("GET", "/v1/agents?limit=500", nil)
	if st != 200 {
		fatal("list agents", fmt.Errorf("status=%d", st))
	}
	var resp struct {
		Items []struct {
			ID       string `json:"id"`
			AgentKey string `json:"agentKey"`
		} `json:"items"`
	}
	_ = json.Unmarshal(body, &resp)
	for _, it := range resp.Items {
		if it.AgentKey == key {
			return it.ID
		}
	}
	return ""
}

func createSession(c *client, agentID, title string) string {
	st, body := c.do("POST", "/v1/sessions", map[string]any{
		"ownerType": "agent", "agentId": agentID, "title": title,
		"defaultProvider": "deepseek", "defaultModel": "deepseek-v4-flash",
	})
	if st != 200 {
		fatal("create session "+title, fmt.Errorf("status=%d body=%s", st, body))
	}
	var s struct {
		ID string `json:"id"`
	}
	_ = json.Unmarshal(body, &s)
	if s.ID == "" {
		fatal("parse session", fmt.Errorf("body=%s", body))
	}
	return s.ID
}

func sendAndCollect(c *client, sessionID, agentKey, content string) string {
	st, body := c.do("POST", "/v1/chat/messages", map[string]any{
		"sessionId": sessionID, "agentKey": agentKey, "content": content,
		"options": map[string]any{"provider": "deepseek", "model": "deepseek-v4-flash"},
	})
	if st != 200 {
		fatal("send message", fmt.Errorf("status=%d body=%s", st, body))
	}
	// SendChatMessage 同步返回 agent_message
	var resp struct {
		AgentMessage map[string]any `json:"agentMessage"`
	}
	_ = json.Unmarshal(body, &resp)
	raw, _ := json.Marshal(resp.AgentMessage)
	return extractText(resp.AgentMessage, string(raw))
}

func extractText(m map[string]any, raw string) string {
	// 常见字段：content_markdown / content / text / parts[]
	for _, k := range []string{"content_markdown", "content", "text", "message"} {
		if v, ok := m[k].(string); ok && v != "" {
			return v
		}
	}
	if parts, ok := m["parts"].([]any); ok {
		var sb strings.Builder
		for _, p := range parts {
			if pm, ok := p.(map[string]any); ok {
				if t, ok := pm["text"].(string); ok {
					sb.WriteString(t)
				}
			}
		}
		if sb.Len() > 0 {
			return sb.String()
		}
	}
	return raw
}

func containsAll(text string, keys ...string) (bool, string) {
	for _, k := range keys {
		if !strings.Contains(text, k) {
			return false, "缺少关键词: " + k
		}
	}
	return true, ""
}

func main() {
	c := newClient()
	st, body := c.do("POST", "/v1/admins/login", map[string]any{"email": "admin@aranea.local", "password": "changeme"})
	if st != 200 {
		fatal("login", fmt.Errorf("status=%d body=%s", st, body))
	}
	fmt.Println("登录成功，开始闭环验证\n")

	// ========== 场景 1：告警处理 Agent ==========
	fmt.Println("=== 场景 1：模拟数据库 CPU 告警 → 告警处理 Agent ===")
	alarmID := agentIDByKey(c, "ops_alarm_handler")
	if alarmID == "" {
		fatal("lookup ops_alarm_handler", fmt.Errorf("not found"))
	}
	sess1 := createSession(c, alarmID, "GOAI验证-告警处理")

	simAlert := `【模拟告警】
告警ID: ALM-20260728-0042
级别: critical
资产: prod-db-01 (192.168.10.21, PostgreSQL 主库)
指标: CPU 使用率 96%，持续 15 分钟；慢查询数量突增至 120 条/分钟
关联告警: ALM-20260728-0039 (同机磁盘 IO 利用率 91%)
时间: 2026-07-28 22:15:00
请分析该告警并输出《告警分析单》。`

	reply1 := sendAndCollect(c, sess1, "ops_alarm_handler", simAlert)
	fmt.Printf("--- 告警处理 Agent 回复（前 600 字）---\n%.600s\n---\n", reply1)

	ok, missing := containsAll(reply1, "告警分析单", "严重度", "处理建议")
	check("告警分析单结构完整", ok, missing)
	ok = strings.Contains(reply1, "P1") || strings.Contains(reply1, "P2") || strings.Contains(reply1, "P3") || strings.Contains(reply1, "P4")
	check("严重度分级输出（P1-P4）", ok, "未包含 P1-P4 分级")

	// ========== 场景 2：故障诊断 Agent（携带场景 1 结论）==========
	fmt.Println("\n=== 场景 2：故障诊断 Agent 根因定位 ===")
	faultID := agentIDByKey(c, "ops_fault_diagnosis")
	sess2 := createSession(c, faultID, "GOAI验证-故障诊断")

	diagInput := fmt.Sprintf(`【故障诊断请求】
告警处理 Agent 初步结论：prod-db-01 CPU 96%% 持续 15 分钟，慢查询 120 条/分钟，磁盘 IO 91%%。
补充证据：
- metric.query(pg_cpu) 22:00 起从 35%% 陡增至 96%%
- 慢查询 TOP1: SELECT * FROM orders WHERE customer_id=$1 AND status='pending' （平均 8.2s，全表扫描 420 万行）
- 当日 18:00 有一次 orders 表结构变更（新增字段，未重建索引）
请执行根因分析并输出《故障诊断报告》。`)

	reply2 := sendAndCollect(c, sess2, "ops_fault_diagnosis", diagInput)
	fmt.Printf("--- 故障诊断 Agent 回复（前 600 字）---\n%.600s\n---\n", reply2)

	ok, missing = containsAll(reply2, "故障诊断报告", "根因", "解决方案")
	check("故障诊断报告结构完整", ok, missing)
	ok = strings.Contains(reply2, "置信度") || strings.Contains(reply2, "%")
	check("根因含置信度", ok, "未包含置信度")

	// ========== 场景 3：命令生成专家 JSON 输出 ==========
	fmt.Println("\n=== 场景 3：命令生成专家 JSON 格式校验 ===")
	cmdID := agentIDByKey(c, "ops_command_expert")
	sess3 := createSession(c, cmdID, "GOAI验证-命令生成")

	reply3 := sendAndCollect(c, sess3, "ops_command_expert",
		"目标 OS：Linux（CentOS 7）。意图：查询 orders 表上所有索引。数据库是 PostgreSQL 14。")
	fmt.Printf("--- 命令生成专家回复 ---\n%.400s\n---\n", reply3)

	// 提取 JSON（可能在代码围栏中）
	jsonText := strings.TrimSpace(reply3)
	jsonText = strings.TrimPrefix(jsonText, "```json")
	jsonText = strings.TrimPrefix(jsonText, "```")
	jsonText = strings.TrimSuffix(jsonText, "```")
	jsonText = strings.TrimSpace(jsonText)
	var cmdOut struct {
		Command     string `json:"command"`
		Explanation string `json:"explanation"`
	}
	ok = json.Unmarshal([]byte(jsonText), &cmdOut) == nil && cmdOut.Command != ""
	check("命令生成专家输出合法 JSON {command, explanation}", ok, "输出非预期 JSON 格式")

	// ========== 汇总 ==========
	fmt.Printf("\n=== 验证汇总：PASS %d / FAIL %d ===\n", passCount, failCount)
	if failCount > 0 {
		os.Exit(1)
	}
}
