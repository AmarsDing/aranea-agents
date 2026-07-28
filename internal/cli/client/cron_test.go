package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aranea-agents/internal/cli/client"
)

func TestResetCronTaskFailures(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/cron-tasks/ct-1/reset-failures" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"id":"ct-1"`) {
			t.Errorf("request body should carry typed ResetCronTaskFailuresRequest, got %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ct-1","name":"Nightly","status":"active"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	task, err := c.ResetCronTaskFailures(context.Background(), "ct-1")
	if err != nil {
		t.Fatalf("ResetCronTaskFailures: %v", err)
	}
	if task.Id != "ct-1" {
		t.Errorf("expected id ct-1, got %s", task.Id)
	}
}
