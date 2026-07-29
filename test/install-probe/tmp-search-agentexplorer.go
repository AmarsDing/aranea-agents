// 为 12 个运维域搜索 AgentExplorer 技能
//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"time"
)

var domains = []struct {
	Agent   string
	Keyword string
}{
	{"ops_alarm_handler", "云监控告警处理与通知"},
	{"ops_auto_inspection", "ECS 自动巡检"},
	{"ops_change_execution", "运维变更编排执行 OOS"},
	{"ops_command_expert", "ECS 云助手命令执行"},
	{"ops_compliance_check", "资源合规检查与审计"},
	{"ops_database", "RDS 数据库运维诊断"},
	{"ops_doc_generation", "运维文档生成"},
	{"ops_fault_diagnosis", "ECS 故障诊断排查"},
	{"ops_log_analysis", "SLS 日志分析"},
	{"ops_network_inspection", "VPC 网络巡检诊断"},
	{"ops_server_command", "服务器远程命令执行"},
	{"ops_system_inspection", "操作系统巡检"},
}

type skillItem struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	Category    string `json:"categoryCode"`
	Installs    int    `json:"installCount"`
}

func main() {
	proxyURL, _ := url.Parse("socks5h://127.0.0.1:1080")
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)},
	}
	out, _ := os.Create("search-results.jsonl")
	defer out.Close()

	for _, d := range domains {
		u := "https://agentexplorer.aliyuncs.com/openapi/for-agent/skills?keyword=" +
			url.QueryEscape(d.Keyword) + "&searchMode=semantic&maxResults=6"
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", "AlibabaCloud-Agent-Skills/alibabacloud-find-skills")
		req.Header.Set("x-acs-version", "2026-03-17")
		resp, err := client.Do(req)
		if err != nil {
			fmt.Printf("%-24s ERROR: %v\n", d.Agent, err)
			continue
		}
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		out.Write(append([]byte(fmt.Sprintf(`{"agent":%q,"raw":`, d.Agent)), body...))
		out.WriteString("}\n")

		var parsed struct {
			Data []skillItem `json:"data"`
		}
		if err := json.Unmarshal(body, &parsed); err != nil {
			fmt.Printf("%-24s parse error: %v raw=%s\n", d.Agent, err, trunc(string(body), 120))
			continue
		}
		fmt.Printf("\n== %s (%s) ==\n", d.Agent, d.Keyword)
		for _, s := range parsed.Data {
			fmt.Printf("  %-42s [%s] %s\n", s.Name, s.Category, trunc(s.Description, 70))
		}
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
