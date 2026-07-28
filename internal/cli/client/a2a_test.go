package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	a2av1 "aranea-agents/api/kratos/a2a/v1"
	"aranea-agents/internal/cli/client"
)

func TestDiscoverA2ARemoteAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/a2a/remote-discover" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"remoteUrl"`) {
			t.Errorf("body should contain remoteUrl, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"agentId":"remote-1","displayName":"Remote Bot","source":"remote","remoteUrl":"https://a2a.example.com"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	card, err := c.DiscoverA2ARemoteAgent(context.Background(), "https://a2a.example.com", "", "")
	if err != nil {
		t.Fatalf("DiscoverA2ARemoteAgent: %v", err)
	}
	if card.AgentId != "remote-1" {
		t.Errorf("expected agentId remote-1, got %q", card.AgentId)
	}
}

func TestListA2ARemoteAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/a2a/remote-agents" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("workspace"); got != "ws-1" {
			t.Errorf("workspace: got %q, want %q", got, "ws-1")
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"ra-1","displayName":"Bot","remoteUrl":"https://a2a.example.com","enabled":true,"healthy":true}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListA2ARemoteAgents(context.Background(), "ws-1")
	if err != nil {
		t.Fatalf("ListA2ARemoteAgents: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Id != "ra-1" {
		t.Errorf("unexpected items: %+v", resp.Items)
	}
}

func TestRegisterA2ARemoteAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/a2a/remote-agents" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"remoteUrl"`) {
			t.Errorf("body should contain remoteUrl, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"ra-new","displayName":"Bot","remoteUrl":"https://a2a.example.com","enabled":true}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	agent, err := c.RegisterA2ARemoteAgent(context.Background(), &a2av1.RegisterRemoteAgentRequest{
		RemoteUrl:   "https://a2a.example.com",
		DisplayName: "Bot",
	})
	if err != nil {
		t.Fatalf("RegisterA2ARemoteAgent: %v", err)
	}
	if agent.Id != "ra-new" {
		t.Errorf("expected id ra-new, got %q", agent.Id)
	}
}

func TestDeleteA2ARemoteAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/a2a/remote-agents/ra-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteA2ARemoteAgent(context.Background(), "ra-1"); err != nil {
		t.Fatalf("DeleteA2ARemoteAgent: %v", err)
	}
}

func TestListA2AAudit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/a2a/audit" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("caller_agent_id") != "agent-a" || q.Get("limit") != "10" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"aud-1","invokeId":"inv-1","callerAgentId":"agent-a","calleeAgentId":"agent-b","capability":"chat","status":"success","durationMs":42}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListA2AAudit(context.Background(), "agent-a", "", 10, 0)
	if err != nil {
		t.Fatalf("ListA2AAudit: %v", err)
	}
	if resp.Total != 1 || len(resp.Items) != 1 || resp.Items[0].InvokeId != "inv-1" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestGetA2AConfig(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/a2a/config" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"publicBaseUrl":"https://a2a.example.com","publicBaseUrlSource":"config"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	cfg, err := c.GetA2AConfig(context.Background())
	if err != nil {
		t.Fatalf("GetA2AConfig: %v", err)
	}
	if cfg.PublicBaseUrl != "https://a2a.example.com" || cfg.PublicBaseUrlSource != "config" {
		t.Errorf("unexpected config: %+v", cfg)
	}
}
