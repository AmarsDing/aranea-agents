// 一次性探针：用生产同款 prompt+schema 直连 DeepSeek，验证 LLM 记忆提取为何 0 产出（2026-07-29）
package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"
)

const systemPrompt = `You extract durable user-specific facts from chat messages for a long-term memory store.

## Output Format
Call the provided function "extract_memory_facts" with your results.
If the model does not support function calling, output JSON with this schema:
{"facts":[{"statement":"...","subject_type":"person|preference|event|concept","scope":"user|agent","confidence":0.9,"topics":["tag"],"is_pii_sensitive":false}],"no_facts_reason":""}

## Rules
- Include only stable preferences, identity, constraints, and confirmed facts about the user.
- Do NOT store secrets, passwords, API keys, or ephemeral one-off task details.
- Each statement must be self-contained and written in third person about the user when possible.
- Return at most 8 facts.
- Set "is_pii_sensitive" to true if the statement contains or implies personal identifiable information.
- Set "no_facts_reason" when returning zero facts to explain why (e.g. "only_greetings", "only_task_context", "already_known").
- "subject_type" categorizes the fact: person, preference, event, concept, or other.
- "scope" is "user" for cross-session facts, "agent" for agent-specific behavior.
- "confidence" ranges 0.0–1.0 reflecting how certain the fact is.
`

var toolSchema = map[string]any{
	"type": "function",
	"function": map[string]any{
		"name":        "extract_memory_facts",
		"description": "Extract durable user-specific facts from the conversation for long-term memory storage.",
		"parameters": map[string]any{
			"type": "object",
			"properties": map[string]any{
				"facts": map[string]any{
					"type": "array",
					"items": map[string]any{
						"type": "object",
						"properties": map[string]any{
							"statement":        map[string]any{"type": "string"},
							"subject_type":     map[string]any{"type": "string", "enum": []string{"person", "preference", "event", "concept", "other"}},
							"scope":            map[string]any{"type": "string", "enum": []string{"user", "agent"}},
							"confidence":       map[string]any{"type": "number"},
							"topics":           map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
							"is_pii_sensitive": map[string]any{"type": "boolean"},
						},
						"required": []string{"statement", "subject_type", "confidence"},
					},
				},
				"no_facts_reason": map[string]any{"type": "string"},
			},
			"required": []string{"facts"},
		},
	},
}

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		fmt.Println("open:", err)
		os.Exit(1)
	}
	defer db.Close()

	// 1. 读取 deepseek 模型配置
	var cfgJSON string
	err = db.QueryRow(`SELECT config_json FROM llm_provider_models WHERE provider='deepseek' AND model='deepseek-v4-flash'`).Scan(&cfgJSON)
	if err != nil {
		fmt.Println("model cfg:", err)
		os.Exit(1)
	}
	var cfg map[string]any
	json.Unmarshal([]byte(cfgJSON), &cfg)
	baseURL, _ := cfg["base_url"].(string)
	apiKey, _ := cfg["api_key"].(string)
	if baseURL == "" {
		if v, ok := cfg["baseURL"].(string); ok {
			baseURL = v
		}
	}
	fmt.Printf("== model config ==\nbase_url=%s api_key_len=%d enabled_keys=%v\n", baseURL, len(apiKey), keysOf(cfg))

	// 2. 拉取最近自动记忆处理过的 session 的 transcript（activities → task/reply）
	sessIDs := []string{
		"ec86e351-88fc-4ffd-88d8-0ffce1e8af53", // spirit, 今天 18:13/18:23 facts_added:0
		"4fb82036-8ce4-48c1-bfe8-08094794d80f", // 40 条消息 facts_added:0
	}
	for _, sid := range sessIDs {
		fmt.Printf("\n== session %s transcript ==\n", sid)
		rows, err := db.Query(`SELECT kind, left(COALESCE(content,''),200) FROM activities WHERE session_id=$1 AND kind IN ('task','reply') ORDER BY timestamp ASC LIMIT 30`, sid)
		if err != nil {
			fmt.Println("  activities ERR:", err)
			continue
		}
		var transcript strings.Builder
		n := 0
		for rows.Next() {
			var kind, content string
			rows.Scan(&kind, &content)
			role := "ASSISTANT"
			if kind == "task" {
				role = "USER"
			}
			fmt.Printf("  [%s] %s\n", role, strings.ReplaceAll(content, "\n", " "))
			transcript.WriteString(role + ": " + content + "\n")
			n++
		}
		rows.Close()
		fmt.Printf("  (%d messages)\n", n)
		if n == 0 {
			fmt.Println("  !! 空 transcript — ExtractFacts 会静默返回 0")
			continue
		}

		// 3. 直连 DeepSeek 调用提取
		if apiKey == "" || baseURL == "" {
			fmt.Println("  !! 无 api_key/base_url，跳过 LLM 调用")
			continue
		}
		callExtract(baseURL, apiKey, transcript.String())
	}
}

func callExtract(baseURL, apiKey, transcript string) {
	reqBody := map[string]any{
		"model": "deepseek-v4-flash",
		"messages": []map[string]string{
			{"role": "system", "content": systemPrompt},
			{"role": "user", "content": "Conversation excerpt:\n\n" + transcript},
		},
		"tools":  []map[string]any{toolSchema},
		"stream": false,
	}
	b, _ := json.Marshal(reqBody)
	url := strings.TrimSuffix(baseURL, "/") + "/chat/completions"
	req, _ := http.NewRequest("POST", url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+apiKey)
	cli := &http.Client{Timeout: 90 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		fmt.Println("  LLM call ERR:", err)
		return
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("  LLM status=%d\n", resp.StatusCode)
	out := string(body)
	if len(out) > 2000 {
		out = out[:2000] + "…(truncated)"
	}
	fmt.Println("  LLM raw response:", out)
}

func keysOf(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	return ks
}
