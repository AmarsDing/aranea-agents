// 导出每个运维域的全部候选（含 skillName/path/installs/desc 前80字符）
//go:build ignore

package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

type item struct {
	SkillName   string `json:"skillName"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	GithubPath  string `json:"githubPath"`
	Installs    int    `json:"installCount"`
}

type line struct {
	Agent string `json:"agent"`
	Raw   struct {
		Data []item `json:"data"`
	} `json:"raw"`
}

func main() {
	f, _ := os.Open("search-results.jsonl")
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	order := []string{"ops_alarm_handler", "ops_auto_inspection", "ops_change_execution", "ops_command_expert", "ops_compliance_check", "ops_database", "ops_doc_generation", "ops_fault_diagnosis", "ops_log_analysis", "ops_network_inspection", "ops_server_command", "ops_system_inspection"}
	byAgent := map[string][]item{}
	for sc.Scan() {
		var l line
		if json.Unmarshal(sc.Bytes(), &l) == nil && l.Agent != "" {
			byAgent[l.Agent] = l.Raw.Data
		}
	}
	for _, a := range order {
		fmt.Printf("\n### %s\n", a)
		for _, it := range byAgent[a] {
			sub := strings.TrimPrefix(it.GithubPath, "https://github.com/aliyun/alibabacloud-aiops-skills/tree/master/")
			desc := strings.ReplaceAll(it.Description, "\n", " ")
			fmt.Printf("  %-44s i=%-4d %s\n    %s\n", it.SkillName, it.Installs, sub, trunc(desc, 100))
		}
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
