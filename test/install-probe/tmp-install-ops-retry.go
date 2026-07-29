// 重试安装 8 个缺失运维技能（Timeout 加大到 600s）
//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"aranea-agents/internal/pkginstall"
)

// 缺失的 8 个（ecs-diagnose 墓碑已清理，可重装）
var skills = []struct {
	For     string
	Subpath string
}{
	{"ops_auto_inspection+ops_system_inspection", "skills/computing/alinux/alibabacloud-alinux-sysom-inspection"},
	{"ops_change_execution", "skills/migrationom/oos/alibabacloud-oos-chatops-agent"},
	{"ops_compliance_check", "skills/migrationom/entconsole/alibabacloud-resourcecenter-search"},
	{"ops_database", "skills/database/rds/alibabacloud-rds-copilot"},
	{"ops_doc_generation", "skills/playbooks/wadaps/alibabacloud-migration-mas-solution"},
	{"ops_fault_diagnosis", "skills/computing/ecs/alibabacloud-ecs-diagnose"},
	{"ops_log_analysis", "skills/storage/sls/alibabacloud-sls-query"},
	{"ops_network_inspection", "skills/playbooks/trouboper/alibabacloud-network-health-inspection"},
}

func main() {
	os.Setenv("ARANEA_GIT_PROXY", "socks5h://127.0.0.1:1080")

	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "changeme"})
	resp, err := http.Post("http://127.0.0.1:8000/v1/admins/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		panic(err)
	}
	var token string
	for _, c := range resp.Cookies() {
		if c.Name == "access_token" {
			token = c.Value
		}
	}
	resp.Body.Close()
	if token == "" {
		panic("no token")
	}
	fmt.Println("login ok")

	ins := &pkginstall.Installer{APIURL: "http://127.0.0.1:8000", Token: token, Quiet: true, Timeout: 600 * time.Second}

	type outcome struct {
		For, Subpath, Status, Msg string
		Created                   int
	}
	var results []outcome
	for _, s := range skills {
		manifest := &pkginstall.Manifest{
			Version:  1,
			Metadata: pkginstall.ManifestMetadata{Name: "ops-skill-retry-" + s.For},
			Spec: pkginstall.ManifestSpec{Skills: []pkginstall.SkillSpec{{
				URL:      "https://github.com/aliyun/alibabacloud-aiops-skills",
				Subpath:  s.Subpath,
				Decision: "skip",
			}}},
		}
		result, err := ins.Install("", manifest)
		oc := outcome{For: s.For, Subpath: s.Subpath}
		if err != nil {
			oc.Status = "error"
			oc.Msg = err.Error()
		} else {
			for _, st := range result.Steps {
				oc.Status = st.Status
				oc.Msg = st.Message
				oc.Created = st.CreatedCount
			}
			if len(result.Errors) > 0 {
				oc.Status = "error"
				oc.Msg = result.Errors[0]
			}
		}
		results = append(results, oc)
		fmt.Printf("[%s] %-24s created=%d %s\n", oc.Status, oc.For, oc.Created, trunc(oc.Msg, 100))
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("ops-install-retry-results.json", out, 0o644)
	fmt.Println("wrote ops-install-retry-results.json")
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
