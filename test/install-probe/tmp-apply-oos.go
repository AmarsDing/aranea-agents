// 对已有 oos job 做 poll + apply keep_separate + publish/enable
//go:build ignore

package main

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	_ "github.com/lib/pq"
)

const apiBase = "http://127.0.0.1:8000"
const jobID = "4828745eb899ff6d76092702"

func main() {
	// 登录
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "changeme"})
	resp, err := http.Post(apiBase+"/v1/admins/login", "application/json", bytes.NewReader(loginBody))
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

	// poll job → candidateId
	client := &http.Client{Timeout: 60 * time.Second}
	var candID string
	for i := 0; i < 30; i++ {
		req, _ := http.NewRequest("GET", apiBase+"/v1/skills/import/"+jobID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		r, err := client.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		b, _ := io.ReadAll(r.Body)
		r.Body.Close()
		var job struct {
			Status     string `json:"status"`
			Candidates []struct {
				CandidateID string `json:"candidateId"`
				Slug        string `json:"slug"`
			} `json:"candidates"`
		}
		json.Unmarshal(b, &job)
		fmt.Printf("poll %d: status=%s candidates=%d\n", i, job.Status, len(job.Candidates))
		if job.Status == "completed" || job.Status == "applied" {
			for _, c := range job.Candidates {
				if c.Slug == "alibabacloud-oos-chatops-agent" {
					candID = c.CandidateID
				}
			}
			if candID == "" && len(job.Candidates) > 0 {
				candID = job.Candidates[0].CandidateID
			}
			break
		}
		if job.Status == "failed" {
			panic("job failed: " + string(b))
		}
		time.Sleep(2 * time.Second)
	}
	if candID == "" {
		panic("no candidate")
	}
	fmt.Println("candidate:", candID)

	// apply keep_separate
	body := map[string]any{"decisions": []map[string]string{{"candidateId": candID, "action": "keep_separate"}}}
	jb, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", apiBase+"/v1/skills/import/"+jobID+"/apply", bytes.NewReader(jb))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	r2, err := (&http.Client{Timeout: 300 * time.Second}).Do(req)
	if err != nil {
		panic(err)
	}
	b2, _ := io.ReadAll(r2.Body)
	r2.Body.Close()
	fmt.Printf("apply HTTP %d: %s\n", r2.StatusCode, trunc(string(b2), 200))
	if r2.StatusCode >= 300 {
		return
	}

	// publish + enable
	dsn := "postgres://postgres:Hangshan%40123@127.0.0.1:5432/aranea?sslmode=disable"
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		panic(err)
	}
	defer db.Close()
	var id string
	err = db.QueryRow(`SELECT id FROM skill WHERE skill_key='alibabacloud-oos-chatops-agent' AND status='draft'`).Scan(&id)
	if err != nil {
		fmt.Println("query id err:", err)
		return
	}
	for _, op := range []struct{ m, p, b string }{
		{"POST", "/publish", "{}"},
		{"PATCH", "/enabled", `{"enabled":true}`},
	} {
		r3, _ := http.NewRequest(op.m, apiBase+"/v1/skills/"+id+op.p, bytes.NewReader([]byte(op.b)))
		r3.Header.Set("Content-Type", "application/json")
		r3.Header.Set("Authorization", "Bearer "+token)
		r4, err := http.DefaultClient.Do(r3)
		if err != nil {
			fmt.Println(op.p, "err:", err)
			continue
		}
		bb, _ := io.ReadAll(r4.Body)
		r4.Body.Close()
		fmt.Printf("%s HTTP %d %s\n", op.p, r4.StatusCode, trunc(string(bb), 80))
	}
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
