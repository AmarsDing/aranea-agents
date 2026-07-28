package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	memoryv1 "aranea-agents/api/kratos/memory/v1"
	"aranea-agents/internal/cli/client"
)

func TestListMemoryFacts(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/memory/l3/facts" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		q := r.URL.Query()
		if q.Get("scope_type") != "agent" || q.Get("scope_id") != "a1" || q.Get("limit") != "10" {
			t.Errorf("unexpected query: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"f1","statement":"s"}],"total":1,"limit":10,"offset":0}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListMemoryFacts(context.Background(), "agent", "a1", "", "", "", 10, 0)
	if err != nil {
		t.Fatalf("ListMemoryFacts: %v", err)
	}
	if len(resp.Items) != 1 || resp.Total != 1 {
		t.Errorf("expected 1 fact, got %d (total %d)", len(resp.Items), resp.Total)
	}
}

func TestListCascadeProposals(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/memory/cascade/proposals" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("agent_id") != "a1" {
			t.Errorf("missing agent_id: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"p1","agentId":"a1","status":"pending"}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListCascadeProposals(context.Background(), "a1", "", 0)
	if err != nil {
		t.Fatalf("ListCascadeProposals: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 proposal, got %d", len(resp.Items))
	}
}

func TestApproveCascadeProposal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/memory/cascade/proposals/p1/approve" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"reviewer":"ops"`) {
			t.Errorf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"proposal":{"id":"p1","status":"approved"}}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	p, err := c.ApproveCascadeProposal(context.Background(), "p1", "ops")
	if err != nil {
		t.Fatalf("ApproveCascadeProposal: %v", err)
	}
	if p.Status != "approved" {
		t.Errorf("expected status approved, got %s", p.Status)
	}
}

func TestRejectCascadeProposal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/memory/cascade/proposals/p1/reject" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"proposal":{"id":"p1","status":"rejected"}}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	p, err := c.RejectCascadeProposal(context.Background(), "p1", "ops", "risky")
	if err != nil {
		t.Fatalf("RejectCascadeProposal: %v", err)
	}
	if p.Status != "rejected" {
		t.Errorf("expected status rejected, got %s", p.Status)
	}
}

func TestCompositeSearchMemories(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/memory/search/composite" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"query":"hello"`) {
			t.Errorf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"layer":"l3","id":"f1","text":"s","score":0.9}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	req := &memoryv1.CompositeSearchMemoriesRequest{AgentId: "a1", Query: "hello", Limit: 5}
	resp, err := c.CompositeSearchMemories(context.Background(), req)
	if err != nil {
		t.Fatalf("CompositeSearchMemories: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 hit, got %d", len(resp.Items))
	}
}

func TestDebugMemoryRecall(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/memory/recall/debug" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"l2Hits":[{"layer":"l2","id":"e1"}],"l3Hits":[{"layer":"l3","id":"f1"}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	req := &memoryv1.DebugMemoryRecallRequest{AgentId: "a1", SessionId: "s1", Query: "q"}
	resp, err := c.DebugMemoryRecall(context.Background(), req)
	if err != nil {
		t.Fatalf("DebugMemoryRecall: %v", err)
	}
	if len(resp.L2Hits) != 1 || len(resp.L3Hits) != 1 {
		t.Errorf("expected 1 l2 + 1 l3 hit, got %d + %d", len(resp.L2Hits), len(resp.L3Hits))
	}
}
