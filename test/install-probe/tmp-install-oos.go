// 专用安装 oos-chatops-agent：上传 → poll → 手动 apply keep_separate
//go:build ignore

package main

import (
	"archive/zip"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"aranea-agents/internal/pkginstall"
)

const apiBase = "http://127.0.0.1:8000"

func main() {
	os.Setenv("ARANEA_GIT_PROXY", "socks5h://127.0.0.1:1080")

	token := login()

	// 1. clone + zip
	tmpDir, cleanup, err := pkginstall.FetchFromURL("https://github.com/aliyun/alibabacloud-aiops-skills", "", true)
	if err != nil {
		panic(err)
	}
	defer cleanup()
	srcDir := filepath.Join(tmpDir, "skills/migrationom/oos/alibabacloud-oos-chatops-agent")
	zipPath := filepath.Join(os.TempDir(), "oos-chatops.zip")
	if err := zipDirLocal(srcDir, zipPath); err != nil {
		panic(err)
	}
	defer os.Remove(zipPath)
	fmt.Println("zipped:", zipPath)

	// 2. upload
	jobID := upload(zipPath, token)
	fmt.Println("job:", jobID)

	// 3. poll job → 取 candidateId
	candID := pollJob(jobID, token)
	fmt.Println("candidate:", candID)

	// 4. apply keep_separate
	apply(jobID, candID, token)

	// 5. publish + enable
	publishEnable(token)
}

func login() string {
	loginBody, _ := json.Marshal(map[string]string{"username": "admin", "password": "changeme"})
	resp, err := http.Post(apiBase+"/v1/admins/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	for _, c := range resp.Cookies() {
		if c.Name == "access_token" {
			return c.Value
		}
	}
	panic("no token")
}

func upload(zipPath, token string) string {
	f, _ := os.Open(zipPath)
	defer f.Close()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	fw, _ := w.CreateFormFile("file", filepath.Base(zipPath))
	io.Copy(fw, f)
	w.WriteField("source", "cli_url")
	w.WriteField("source_url", "https://github.com/aliyun/alibabacloud-aiops-skills")
	w.WriteField("source_subpath", "skills/migrationom/oos/alibabacloud-oos-chatops-agent")
	w.Close()
	req, _ := http.NewRequest("POST", apiBase+"/v1/skills/import", &buf)
	req.Header.Set("Content-Type", w.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 600 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	var out struct {
		JobID  string `json:"jobId"`
		JobID2 string `json:"job_id"`
	}
	json.Unmarshal(b, &out)
	if out.JobID == "" {
		out.JobID = out.JobID2
	}
	if out.JobID == "" {
		panic("upload failed: " + string(b))
	}
	return out.JobID
}

func pollJob(jobID, token string) string {
	client := &http.Client{Timeout: 60 * time.Second}
	for i := 0; i < 30; i++ {
		req, _ := http.NewRequest("GET", apiBase+"/v1/skills/import/"+jobID, nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := client.Do(req)
		if err != nil {
			time.Sleep(2 * time.Second)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		var job struct {
			Status     string `json:"status"`
			Candidates []struct {
				CandidateID string `json:"candidateId"`
				Slug        string `json:"slug"`
			} `json:"candidates"`
		}
		json.Unmarshal(b, &job)
		if job.Status == "completed" {
			for _, c := range job.Candidates {
				if c.Slug == "alibabacloud-oos-chatops-agent" {
					return c.CandidateID
				}
			}
			if len(job.Candidates) > 0 {
				return job.Candidates[0].CandidateID
			}
			panic("no candidate in job: " + string(b))
		}
		if job.Status == "failed" {
			panic("job failed: " + string(b))
		}
		time.Sleep(2 * time.Second)
	}
	panic("job not terminal")
}

func apply(jobID, candID, token string) {
	body := map[string]any{"decisions": []map[string]string{{"candidateId": candID, "action": "keep_separate"}}}
	jb, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", apiBase+"/v1/skills/import/"+jobID+"/apply", bytes.NewReader(jb))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	client := &http.Client{Timeout: 300 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	fmt.Printf("apply HTTP %d: %s\n", resp.StatusCode, trunc(string(b), 200))
}

func publishEnable(token string) {
	// 查 id
	type skillRow struct{ ID string }
	req, _ := http.NewRequest("GET", apiBase+"/v1/skills?search=alibabacloud-oos-chatops-agent", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		panic(err)
	}
	b, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	var list struct {
		Items []struct {
			ID     string `json:"id"`
			Slug   string `json:"slug"`
			Status string `json:"status"`
		} `json:"items"`
	}
	json.Unmarshal(b, &list)
	for _, it := range list.Items {
		if it.Slug != "alibabacloud-oos-chatops-agent" || it.Status != "draft" {
			continue
		}
		for _, op := range []struct{ m, p, b string }{
			{"POST", "/publish", "{}"},
			{"PATCH", "/enabled", `{"enabled":true}`},
		} {
			r2, _ := http.NewRequest(op.m, apiBase+"/v1/skills/"+it.ID+op.p, bytes.NewReader([]byte(op.b)))
			r2.Header.Set("Content-Type", "application/json")
			r2.Header.Set("Authorization", "Bearer "+token)
			r3, err := http.DefaultClient.Do(r2)
			if err != nil {
				fmt.Println(op.p, "err:", err)
				continue
			}
			bb, _ := io.ReadAll(r3.Body)
			r3.Body.Close()
			fmt.Printf("%s HTTP %d %s\n", op.p, r3.StatusCode, trunc(string(bb), 80))
		}
	}
}

func zipDirLocal(src, dst string) error {
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	w := zip.NewWriter(out)
	defer w.Close()
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(src, path)
		rel = filepath.ToSlash(rel)
		f, err := w.Create(rel)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		_, err = f.Write(data)
		return err
	})
}

func trunc(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}
