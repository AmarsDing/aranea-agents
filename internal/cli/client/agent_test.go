package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/cli/client"
)

func TestListAgents(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"items":[{"id":"agent-1","name":"Test Agent"}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListAgents(context.Background(), "", 0, 0)
	if err != nil {
		t.Fatalf("ListAgents: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 agent, got %d", len(resp.Items))
	}
}

func TestGetAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents/agent-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"agent-1","name":"Test Agent"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	agent, err := c.GetAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("GetAgent: %v", err)
	}
	if agent.Id != "agent-1" {
		t.Errorf("expected id agent-1, got %s", agent.Id)
	}
}

func TestCreateAgent(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/agents" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"new-agent","name":"New Agent"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.CreateAgent(context.Background(), nil)
	if err != nil {
		t.Fatalf("CreateAgent: %v", err)
	}
}

func TestUpdateAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/agents/agent-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"agent-1","name":"Updated"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.UpdateAgent(context.Background(), "agent-1", nil)
	if err != nil {
		t.Fatalf("UpdateAgent: %v", err)
	}
}

func TestDeleteAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/agents/agent-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteAgent(context.Background(), "agent-1"); err != nil {
		t.Fatalf("DeleteAgent: %v", err)
	}
}

func TestEnableAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/agents/agent-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"agent-1","status":"active"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	agent, err := c.EnableAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("EnableAgent: %v", err)
	}
	if agent.Status != "active" {
		t.Errorf("expected status active, got %s", agent.Status)
	}
}

func TestDisableAgent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/agents/agent-1" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"id":"agent-1","status":"inactive"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	agent, err := c.DisableAgent(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("DisableAgent: %v", err)
	}
	if agent.Status != "inactive" {
		t.Errorf("expected status inactive, got %s", agent.Status)
	}
}

func TestGetAgentEffectiveTools(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/agents/agent-1/tools/effective" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"items":[],"total":0}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.GetAgentEffectiveTools(context.Background(), "agent-1")
	if err != nil {
		t.Fatalf("GetAgentEffectiveTools: %v", err)
	}
}

func TestUpdateAgentToolPolicy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/agents/agent-1/tools/policy" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(200)
		w.Write([]byte(`{"items":[],"total":0}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	_, err := c.UpdateAgentToolPolicy(context.Background(), "agent-1", nil)
	if err != nil {
		t.Fatalf("UpdateAgentToolPolicy: %v", err)
	}
}
