// 批量安装 10 个运维域阿里云技能（直接 pkginstall 路径，等同 system_admin cli 工具）
//go:build ignore

package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"

	"aranea-agents/internal/pkginstall"
)

// 12 个运维域 → 10 个去重技能（按语义匹配人工选定）
var skills = []struct {
	For     string // 服务的运维域
	Subpath string
}{
	{"ops_alarm_handler", "skills/middleware/cms/alibabacloud-cms-alert-rule-create"},
	{"ops_auto_inspection", "skills/computing/alinux/alibabacloud-alinux-sysom-inspection"},
	{"ops_change_execution", "skills/migrationom/oos/alibabacloud-oos-chatops-agent"},
	{"ops_command_expert+ops_server_command", "skills/developertools/solutions/alibabacloud-workbench-cli"},
	{"ops_compliance_check", "skills/migrationom/entconsole/alibabacloud-resourcecenter-search"},
	{"ops_database", "skills/database/rds/alibabacloud-rds-copilot"},
	{"ops_doc_generation", "skills/playbooks/wadaps/alibabacloud-migration-mas-solution"},
	{"ops_fault_diagnosis", "skills/computing/ecs/alibabacloud-ecs-diagnose"},
	{"ops_log_analysis", "skills/storage/sls/alibabacloud-sls-query"},
	{"ops_network_inspection", "skills/playbooks/trouboper/alibabacloud-network-health-inspection"},
	{"ops_system_inspection", "skills/computing/alinux/alibabacloud-alinux-sysom-inspection"}, // 与 auto_inspection 共享，将被 skip
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

	ins := &pkginstall.Installer{APIURL: "http://127.0.0.1:8000", Token: token, Quiet: true}

	type outcome struct {
		For, Subpath, Status, Msg string
		Created                   int
	}
	var results []outcome
	for _, s := range skills {
		manifest := &pkginstall.Manifest{
			Version:  1,
			Metadata: pkginstall.ManifestMetadata{Name: "ops-skill-" + s.For},
			Spec: pkginstall.ManifestSpec{Skills: []pkginstall.SkillSpec{{
				URL:      "https://github.com/aliyun/alibabacloud-aiops-skills",
				Subpath:  s.Subpath,
				Decision: "skip", // 已存在则跳过（共享技能第二次安装会 skip）
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
		fmt.Printf("[%s] %-24s %s created=%d %s\n", oc.Status, oc.For, oc.Subpath, oc.Created, trunc(oc.Msg, 80))
	}
	out, _ := json.MarshalIndent(results, "", "  ")
	os.WriteFile("ops-install-results.json", out, 0o644)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

var _ = io.Discard
