// 提取 missing4 批次中工具返回的完整错误内容
//go:build ignore

package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "github.com/lib/pq"
)

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// 找工具响应事件：包含 tool_name=cli_admin_skill_install_from_url 且有 content
	rows, err := db.Query(`SELECT session_id, event::text FROM trpc_session_events WHERE created_at::timestamptz > NOW() - INTERVAL '40 minutes' AND event::text LIKE '%"tool_name": "cli_admin_skill_install_from_url"%' ORDER BY created_at ASC`)
	if err != nil {
		panic(err)
	}
	defer rows.Close()
	for rows.Next() {
		var sid, ev string
		rows.Scan(&sid, &ev)
		// 尝试解析 JSON 提取 choices[0].message.content
		var m map[string]any
		if err := json.Unmarshal([]byte(ev), &m); err != nil {
			fmt.Printf("--- session %s (unparsed, raw excerpt) ---\n%s\n\n", sid[:8], truncAround(ev, "status", 800))
			continue
		}
		content := extractContent(m)
		fmt.Printf("--- session %s tool result ---\n%s\n\n", sid[:8], trunc(content, 1200))
	}
}

func extractContent(m map[string]any) string {
	// event 结构：choices[0].message.content 或 llm_response.choices...
	var walk func(v any, depth int) string
	walk = func(v any, depth int) string {
		if depth > 6 {
			return ""
		}
		switch t := v.(type) {
		case map[string]any:
			if c, ok := t["content"].(string); ok && strings.Contains(c, "status") {
				return c
			}
			for _, k := range []string{"message", "delta"} {
				if sub, ok := t[k]; ok {
					if r := walk(sub, depth+1); r != "" {
						return r
					}
				}
			}
			for _, k := range []string{"choices", "messages"} {
				if sub, ok := t[k]; ok {
					if r := walk(sub, depth+1); r != "" {
						return r
					}
				}
			}
		case []any:
			for _, item := range t {
				if r := walk(item, depth+1); r != "" {
					return r
				}
			}
		}
		return ""
	}
	return walk(m, 0)
}

func truncAround(s, kw string, n int) string {
	idx := strings.Index(s, kw)
	if idx < 0 {
		return trunc(s, n)
	}
	start := idx - 300
	if start < 0 {
		start = 0
	}
	end := idx + n
	if end > len(s) {
		end = len(s)
	}
	return s[start:end]
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
