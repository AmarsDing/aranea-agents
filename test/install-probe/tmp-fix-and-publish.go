// 清理 3 个 deleted 墓碑 + 重装 + 全部 publish/enable 一站式
//go:build ignore

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"aranea-agents/internal/pkginstall"
	_ "github.com/lib/pq"
)

var tombstones = []string{
	"alibabacloud-alinux-sysom-inspection",
	"alibabacloud-sls-query",
	"alibabacloud-network-health-inspection",
}

var reinstall = []struct {
	For     string
	Subpath string
}{
	{"ops_auto_inspection+ops_system_inspection", "skills/computing/alinux/alibabacloud-alinux-sysom-inspection"},
	{"ops_log_analysis", "skills/storage/sls/alibabacloud-sls-query"},
	{"ops_network_inspection", "skills/playbooks/trouboper/alibabacloud-network-health-inspection"},
}

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// ── Step 1: 清理墓碑 ──
	for _, slug := range tombstones {
		db.Exec(`DELETE FROM skill_version WHERE skill_id IN (SELECT id FROM skill WHERE skill_key=$1 AND status='deleted')`, slug)
		res, err := db.Exec(`DELETE FROM skill WHERE skill_key=$1 AND status='deleted'`, slug)
		if err != nil {
			fmt.Printf("[tombstone] %s err: %v\n", slug, err)
			continue
		}
		n, _ := res.RowsAffected()
		fmt.Printf("[tombstone] %s deleted rows=%d\n", slug, n)
	}

	// ── Step 2: 登录 ──
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

	// ── Step 3: 重装 3 个 ──
	os.Setenv("ARANEA_GIT_PROXY", "socks5h://127.0.0.1:1080")
	ins := &pkginstall.Installer{APIURL: "http://127.0.0.1:8000", Token: token, Quiet: true, Timeout: 600 * time.Second}
	for _, s := range reinstall {
		manifest := &pkginstall.Manifest{
			Version:  1,
			Metadata: pkginstall.ManifestMetadata{Name: "ops-reinstall-" + s.For},
			Spec: pkginstall.ManifestSpec{Skills: []pkginstall.SkillSpec{{
				URL:      "https://github.com/aliyun/alibabacloud-aiops-skills",
				Subpath:  s.Subpath,
				Decision: "skip",
			}}},
		}
		result, err := ins.Install("", manifest)
		if err != nil {
			fmt.Printf("[reinstall-error] %-24s %v\n", s.For, err)
			continue
		}
		for _, st := range result.Steps {
			fmt.Printf("[reinstall] %-24s status=%s created=%d %s\n", s.For, st.Status, st.CreatedCount, trunc(st.Message, 80))
		}
		if len(result.Errors) > 0 {
			fmt.Printf("[reinstall-error] %-24s %s\n", s.For, result.Errors[0])
		}
	}

	// ── Step 4: publish + enable 全部 draft alibabacloud-* ──
	rows, err := db.Query(`SELECT id, skill_key FROM skill WHERE status='draft' AND lifecycle_status='active' AND skill_key LIKE 'alibabacloud%' ORDER BY skill_key`)
	if err != nil {
		panic(err)
	}
	type sk struct{ id, key string }
	var drafts []sk
	for rows.Next() {
		var s sk
		rows.Scan(&s.id, &s.key)
		drafts = append(drafts, s)
	}
	rows.Close()
	fmt.Printf("\ndraft alibabacloud skills to publish+enable: %d\n", len(drafts))

	okPub, okEn, fail := 0, 0, 0
	for _, s := range drafts {
		code, body := doReq("POST", "http://127.0.0.1:8000/v1/skills/"+s.id+"/publish", "{}", token)
		if code >= 300 {
			fmt.Printf("[FAIL-publish] %-40s HTTP %d: %s\n", s.key, code, trunc(body, 100))
			fail++
			continue
		}
		okPub++
		code, body = doReq("PATCH", "http://127.0.0.1:8000/v1/skills/"+s.id+"/enabled", `{"enabled":true}`, token)
		if code >= 300 {
			fmt.Printf("[FAIL-enable] %-40s HTTP %d: %s\n", s.key, code, trunc(body, 100))
			fail++
			continue
		}
		okEn++
		fmt.Printf("[ok] %s\n", s.key)
	}
	fmt.Printf("\npublished=%d enabled=%d failures=%d\n", okPub, okEn, fail)
}

func doReq(method, url, body, token string) (int, string) {
	req, _ := http.NewRequest(method, url, bytes.NewReader([]byte(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(b)
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
