// 解析搜索结果，重试失败域，为每个运维域选 top skill，输出安装清单
//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type item struct {
	SkillName   string `json:"skillName"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	GithubPath  string `json:"githubPath"`
	Category    string `json:"categoryCode"`
	Installs    int    `json:"installCount"`
	Likes       int    `json:"likeCount"`
}

type line struct {
	Agent string `json:"agent"`
	Raw   struct {
		Data []item `json:"data"`
	} `json:"raw"`
}

var retryDomains = []struct{ Agent, Keyword string }{
	{"ops_alarm_handler", "云监控告警处理与通知"},
	{"ops_auto_inspection", "ECS 自动巡检"},
}

func main() {
	f, _ := os.Open("search-results.jsonl")
	defer f.Close()
	byAgent := map[string][]item{}
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	for sc.Scan() {
		var l line
		if err := json.Unmarshal(sc.Bytes(), &l); err == nil && l.Agent != "" {
			byAgent[l.Agent] = l.Raw.Data
		}
	}

	// 重试失败域
	proxyURL, _ := url.Parse("socks5h://127.0.0.1:1080")
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	for _, d := range retryDomains {
		if len(byAgent[d.Agent]) > 0 {
			continue
		}
		u := "https://agentexplorer.aliyuncs.com/openapi/for-agent/skills?keyword=" + url.QueryEscape(d.Keyword) + "&searchMode=semantic&maxResults=6"
		req, _ := http.NewRequest("GET", u, nil)
		req.Header.Set("User-Agent", "AlibabaCloud-Agent-Skills/alibabacloud-find-skills")
		req.Header.Set("x-acs-version", "2026-03-17")
		var body []byte
		for attempt := 0; attempt < 3; attempt++ {
			resp, err := client.Do(req)
			if err != nil {
				time.Sleep(time.Duration(attempt+1) * 2 * time.Second)
				continue
			}
			body, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
			break
		}
		var parsed struct {
			Data []item `json:"data"`
		}
		if json.Unmarshal(body, &parsed) == nil {
			byAgent[d.Agent] = parsed.Data
		}
	}

	// 每个域选 installCount 最高者
	order := []string{"ops_alarm_handler", "ops_auto_inspection", "ops_change_execution", "ops_command_expert", "ops_compliance_check", "ops_database", "ops_doc_generation", "ops_fault_diagnosis", "ops_log_analysis", "ops_network_inspection", "ops_server_command", "ops_system_inspection"}
	type pick struct {
		Agent, Skill, Path, Disp string
		Installs                 int
	}
	var picks []pick
	for _, agent := range order {
		items := byAgent[agent]
		if len(items) == 0 {
			fmt.Printf("%-26s NO RESULTS\n", agent)
			continue
		}
		best := items[0]
		for _, it := range items[1:] {
			if it.Installs > best.Installs {
				best = it
			}
		}
		sub := strings.TrimPrefix(best.GithubPath, "https://github.com/aliyun/alibabacloud-aiops-skills/tree/master/")
		picks = append(picks, pick{agent, best.SkillName, sub, best.DisplayName, best.Installs})
		fmt.Printf("%-26s → %-44s installs=%-5d %s\n", agent, best.SkillName, best.Installs, sub)
	}

	// 输出安装清单 JSON（供批量安装脚本用）
	out, _ := json.MarshalIndent(picks, "", "  ")
	os.WriteFile("ops-picks.json", out, 0o644)
	fmt.Printf("\nwrote ops-picks.json (%d picks)\n", len(picks))
}
