// 删 sls-query 新墓碑 + 重装 + publish/enable
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

func main() {
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	// 1. 删墓碑
	db.Exec(`DELETE FROM skill_version WHERE skill_id IN (SELECT id FROM skill WHERE skill_key='alibabacloud-sls-query' AND status='deleted')`)
	res, err := db.Exec(`DELETE FROM skill WHERE skill_key='alibabacloud-sls-query' AND status='deleted'`)
	if err != nil {
		fmt.Println("tombstone err:", err)
		return
	}
	n, _ := res.RowsAffected()
	fmt.Println("tombstone deleted rows:", n)

	// 2. 登录
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

	// 3. 重装 sls-query（新 pkginstall 逻辑：跨组去重）
	os.Setenv("ARANEA_GIT_PROXY", "socks5h://127.0.0.1:1080")
	ins := &pkginstall.Installer{APIURL: "http://127.0.0.1:8000", Token: token, Quiet: true, Timeout: 600 * time.Second}
	manifest := &pkginstall.Manifest{
		Version:  1,
		Metadata: pkginstall.ManifestMetadata{Name: "ops-reinstall-sls"},
		Spec: pkginstall.ManifestSpec{Skills: []pkginstall.SkillSpec{{
			URL:      "https://github.com/aliyun/alibabacloud-aiops-skills",
			Subpath:  "skills/storage/sls/alibabacloud-sls-query",
			Decision: "skip",
		}}},
	}
	result, err := ins.Install("", manifest)
	if err != nil {
		fmt.Println("reinstall error:", err)
		return
	}
	for _, st := range result.Steps {
		fmt.Printf("[reinstall] status=%s created=%d %s\n", st.Status, st.CreatedCount, st.Message)
	}
	if len(result.Errors) > 0 {
		fmt.Println("[reinstall-error]", result.Errors[0])
		return
	}

	// 4. publish + enable
	var id string
	err = db.QueryRow(`SELECT id FROM skill WHERE skill_key='alibabacloud-sls-query' AND status='draft'`).Scan(&id)
	if err != nil {
		fmt.Println("query id err:", err)
		return
	}
	code, body := doReq("POST", "http://127.0.0.1:8000/v1/skills/"+id+"/publish", "{}", token)
	fmt.Printf("publish HTTP %d %s\n", code, trunc(body, 100))
	code, body = doReq("PATCH", "http://127.0.0.1:8000/v1/skills/"+id+"/enabled", `{"enabled":true}`, token)
	fmt.Printf("enable HTTP %d %s\n", code, trunc(body, 100))
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
