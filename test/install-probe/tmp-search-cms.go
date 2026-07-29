// 单次搜索：云监控告警
//go:build ignore

package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func main() {
	proxyURL, _ := url.Parse("socks5h://127.0.0.1:1080")
	client := &http.Client{Timeout: 30 * time.Second, Transport: &http.Transport{Proxy: http.ProxyURL(proxyURL)}}
	u := "https://agentexplorer.aliyuncs.com/openapi/for-agent/skills?keyword=" + url.QueryEscape("云监控 CMS 告警规则配置与处理") + "&searchMode=semantic&maxResults=6"
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", "AlibabaCloud-Agent-Skills/alibabacloud-find-skills")
	req.Header.Set("x-acs-version", "2026-03-17")
	var body []byte
	for i := 0; i < 4; i++ {
		resp, err := client.Do(req)
		if err != nil {
			fmt.Println("retry:", err)
			time.Sleep(time.Duration(i+1) * 2 * time.Second)
			continue
		}
		body, _ = io.ReadAll(resp.Body)
		resp.Body.Close()
		break
	}
	var parsed struct {
		Data []struct {
			SkillName   string `json:"skillName"`
			Description string `json:"description"`
			GithubPath  string `json:"githubPath"`
			Installs    int    `json:"installCount"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &parsed); err != nil {
		fmt.Println("parse err:", err, string(body)[:200])
		return
	}
	for _, it := range parsed.Data {
		sub := strings.TrimPrefix(it.GithubPath, "https://github.com/aliyun/alibabacloud-aiops-skills/tree/master/")
		fmt.Printf("%-44s i=%-4d %s\n  %s\n", it.SkillName, it.Installs, sub, strings.ReplaceAll(it.Description, "\n", " ")[:100])
	}
}
