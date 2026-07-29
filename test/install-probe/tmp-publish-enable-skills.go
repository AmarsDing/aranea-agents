// 发布+启用所有 draft skill（anthropics 16 个 + alibabacloud-find-skills）
//go:build ignore

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	_ "github.com/lib/pq"
)

func main() {
	// 登录
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

	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()

	rows, err := db.Query(`SELECT id, skill_key FROM skill WHERE status='draft' AND lifecycle_status='active' ORDER BY skill_key`)
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
	fmt.Printf("draft skills to publish+enable: %d\n", len(drafts))

	okPub, okEn, failures := 0, 0, 0
	for _, s := range drafts {
		// publish
		code, body := doReq("POST", "http://127.0.0.1:8000/v1/skills/"+s.id+"/publish", "{}", token)
		if code >= 300 {
			fmt.Printf("[FAIL-publish] %-36s HTTP %d: %s\n", s.key, code, trunc(body, 120))
			failures++
			continue
		}
		okPub++
		// enable
		code, body = doReq("PATCH", "http://127.0.0.1:8000/v1/skills/"+s.id+"/enabled", `{"enabled":true}`, token)
		if code >= 300 {
			fmt.Printf("[FAIL-enable] %-36s HTTP %d: %s\n", s.key, code, trunc(body, 120))
			failures++
			continue
		}
		okEn++
		fmt.Printf("[ok] %s\n", s.key)
	}
	fmt.Printf("\npublished=%d enabled=%d failures=%d\n", okPub, okEn, failures)
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
