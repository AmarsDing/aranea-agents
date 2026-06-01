package main

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	f, err := os.Create("login_test_result.txt")
	if err != nil {
		fmt.Println("create file error:", err)
		return
	}
	defer f.Close()

	tests := []struct {
		name string
		body string
	}{
		{"dev/dev", `{"password":"dev","username":"dev"}`},
		{"admin/changeme", `{"password":"changeme","username":"admin"}`},
	}
	for _, t := range tests {
		resp, err := http.Post("http://localhost:8000/v1/admins/login", "application/json", bytes.NewReader([]byte(t.body)))
		if err != nil {
			fmt.Fprintf(f, "[%s] ERROR: %v\n", t.name, err)
			continue
		}
		b, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		fmt.Fprintf(f, "[%s] Status=%s Body=%s\n", t.name, resp.Status, string(b))
		fmt.Fprintf(f, "[%s] Cookies: %v\n", t.name, resp.Cookies())
	}
}
