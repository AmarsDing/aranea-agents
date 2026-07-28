package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	organizationv1 "aranea-agents/api/kratos/organization/v1"
	"aranea-agents/internal/cli/client"
)

func TestListOrganization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/organization" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"org-1","orgKey":"acme","name":"Acme","level":"company"}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListOrganization(context.Background())
	if err != nil {
		t.Fatalf("ListOrganization: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].OrgKey != "acme" {
		t.Errorf("unexpected items: %+v", resp.Items)
	}
}

func TestListOrganizationTree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/organization/tree" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"node":{"id":"org-1","name":"Acme"},"children":[{"node":{"id":"org-2","name":"R&D"}}]}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListOrganizationTree(context.Background())
	if err != nil {
		t.Fatalf("ListOrganizationTree: %v", err)
	}
	if len(resp.Items) != 1 || len(resp.Items[0].Children) != 1 {
		t.Errorf("unexpected tree: %+v", resp.Items)
	}
}

func TestGetOrganization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/organization/org-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"org-1","orgKey":"acme","name":"Acme"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	node, err := c.GetOrganization(context.Background(), "org-1")
	if err != nil {
		t.Fatalf("GetOrganization: %v", err)
	}
	if node.Id != "org-1" {
		t.Errorf("expected id org-1, got %q", node.Id)
	}
}

func TestCreateOrganization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/organization" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"org-new","orgKey":"acme","name":"Acme","level":"company"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	node, err := c.CreateOrganization(context.Background(), &organizationv1.CreateOrganizationRequest{
		OrgKey: "acme",
		Name:   "Acme",
		Level:  "company",
	})
	if err != nil {
		t.Fatalf("CreateOrganization: %v", err)
	}
	if node.Id != "org-new" {
		t.Errorf("expected id org-new, got %q", node.Id)
	}
}

func TestUpdateOrganization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/organization/org-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		// body:"node" — 请求体必须是 OrganizationNode 本体，不带包装字段。
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"node"`) {
			t.Errorf("body should be the node itself, got %s", string(body))
		}
		if !strings.Contains(string(body), `"name"`) {
			t.Errorf("body should contain node fields, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"org-1","name":"Updated"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	node, err := c.UpdateOrganization(context.Background(), "org-1", &organizationv1.OrganizationNode{Name: "Updated"})
	if err != nil {
		t.Fatalf("UpdateOrganization: %v", err)
	}
	if node.Name != "Updated" {
		t.Errorf("expected name Updated, got %q", node.Name)
	}
}

func TestDeleteOrganization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/organization/org-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteOrganization(context.Background(), "org-1"); err != nil {
		t.Fatalf("DeleteOrganization: %v", err)
	}
}

func TestReorderOrganization(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/organization/reorder" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"ids"`) {
			t.Errorf("body should contain ids, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.ReorderOrganization(context.Background(), []string{"org-2", "org-1"}); err != nil {
		t.Fatalf("ReorderOrganization: %v", err)
	}
}
