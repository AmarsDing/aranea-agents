//go:build ignore

package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
)

func main() {
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar}

	// login
	loginBody := []byte(`{"username":"admin","password":"changeme"}`)
	resp, err := client.Post("http://localhost:8000/v1/admins/login", "application/json", bytes.NewReader(loginBody))
	if err != nil {
		panic(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	// GET company node
	req, _ := http.NewRequest("GET", "http://localhost:8000/v1/organization/ae8b8503-4049-4cb2-811c-b795f5c30304", nil)
	resp2, err := client.Do(req)
	if err != nil {
		panic(err)
	}
	body, _ := io.ReadAll(resp2.Body)
	resp2.Body.Close()
	fmt.Printf("GET: %s\n", string(body))

	// PATCH with Chinese (Go string literal is UTF-8)
	patch := []byte(`{"node":{"name":"IT 运维行业","description":"测试中文描述：告警处理、故障诊断。"}}`)
	req2, _ := http.NewRequest("PATCH", "http://localhost:8000/v1/organization/ae8b8503-4049-4cb2-811c-b795f5c30304", bytes.NewReader(patch))
	req2.Header.Set("Content-Type", "application/json; charset=utf-8")
	resp3, err := client.Do(req2)
	if err != nil {
		panic(err)
	}
	body3, _ := io.ReadAll(resp3.Body)
	resp3.Body.Close()
	fmt.Printf("PATCH status=%d: %s\n", resp3.StatusCode, string(body3))

	// GET again to verify
	req3, _ := http.NewRequest("GET", "http://localhost:8000/v1/organization/ae8b8503-4049-4cb2-811c-b795f5c30304", nil)
	resp4, err := client.Do(req3)
	if err != nil {
		panic(err)
	}
	body4, _ := io.ReadAll(resp4.Body)
	resp4.Body.Close()
	fmt.Printf("GET2: %s\n", string(body4))
}
