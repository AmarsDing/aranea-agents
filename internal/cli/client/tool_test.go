package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/cli/client"
)

func TestListTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tools" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"tool-1","key":"browser","display_name":"Web Browser","enabled":true}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListTools(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatalf("ListTools: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 tool, got %d", len(resp.Items))
	}
	if resp.Items[0].Key != "browser" {
		t.Errorf("expected key=browser, got %s", resp.Items[0].Key)
	}
}

func TestGetTool(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/tools/tool-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"tool-1","key":"browser","display_name":"Web Browser"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	tool, err := c.GetTool(context.Background(), "tool-1")
	if err != nil {
		t.Fatalf("GetTool: %v", err)
	}
	if tool.Id != "tool-1" {
		t.Errorf("expected id tool-1, got %s", tool.Id)
	}
}

func TestToggleToolEnabled_Enable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/tools/tool-1/enabled" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"tool-1","enabled":true}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	tool, err := c.ToggleToolEnabled(context.Background(), "tool-1", true, "browser")
	if err != nil {
		t.Fatalf("ToggleToolEnabled: %v", err)
	}
	if !tool.Enabled {
		t.Error("expected tool.Enabled=true")
	}
}

func TestToggleToolEnabled_Disable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/tools/tool-1/enabled" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"tool-1","enabled":false}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.ToggleToolEnabled(context.Background(), "tool-1", false, "browser")
	if err != nil {
		t.Fatalf("ToggleToolEnabled(disable): %v", err)
	}
}
