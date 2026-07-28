package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	taxonomyv1 "aranea-agents/api/kratos/taxonomy/v1"
	"aranea-agents/internal/cli/client"
)

func TestListTaxonomy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/taxonomy" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"tax-1","key":"golang","name":"Golang"}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListTaxonomy(context.Background())
	if err != nil {
		t.Fatalf("ListTaxonomy: %v", err)
	}
	if len(resp.Items) != 1 || resp.Items[0].Key != "golang" {
		t.Errorf("unexpected items: %+v", resp.Items)
	}
}

func TestListTaxonomyTree(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/taxonomy/tree" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"node":{"id":"tax-1","name":"Root"},"children":[{"node":{"id":"tax-2","name":"Child"}}]}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListTaxonomyTree(context.Background())
	if err != nil {
		t.Fatalf("ListTaxonomyTree: %v", err)
	}
	if len(resp.Items) != 1 || len(resp.Items[0].Children) != 1 {
		t.Errorf("unexpected tree: %+v", resp.Items)
	}
}

func TestGetTaxonomy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/taxonomy/tax-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"tax-1","key":"golang","name":"Golang"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	node, err := c.GetTaxonomy(context.Background(), "tax-1")
	if err != nil {
		t.Fatalf("GetTaxonomy: %v", err)
	}
	if node.Id != "tax-1" {
		t.Errorf("expected id tax-1, got %q", node.Id)
	}
}

func TestCreateTaxonomy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/taxonomy" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"tax-new","key":"golang","name":"Golang"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	node, err := c.CreateTaxonomy(context.Background(), &taxonomyv1.CreateTaxonomyRequest{
		Key:  "golang",
		Name: "Golang",
	})
	if err != nil {
		t.Fatalf("CreateTaxonomy: %v", err)
	}
	if node.Id != "tax-new" {
		t.Errorf("expected id tax-new, got %q", node.Id)
	}
}

func TestUpdateTaxonomy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPatch || r.URL.Path != "/v1/taxonomy/tax-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		// body:"node" — 请求体必须是 TaxonomyNode 本体，不带包装字段。
		body, _ := io.ReadAll(r.Body)
		if strings.Contains(string(body), `"node"`) {
			t.Errorf("body should be the node itself, got %s", string(body))
		}
		if !strings.Contains(string(body), `"name"`) {
			t.Errorf("body should contain node fields, got %s", string(body))
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"tax-1","name":"Updated"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	node, err := c.UpdateTaxonomy(context.Background(), "tax-1", &taxonomyv1.TaxonomyNode{Name: "Updated"})
	if err != nil {
		t.Fatalf("UpdateTaxonomy: %v", err)
	}
	if node.Name != "Updated" {
		t.Errorf("expected name Updated, got %q", node.Name)
	}
}

func TestDeleteTaxonomy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/taxonomy/tax-1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteTaxonomy(context.Background(), "tax-1"); err != nil {
		t.Fatalf("DeleteTaxonomy: %v", err)
	}
}

func TestReorderTaxonomy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/v1/taxonomy/reorder" {
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
	if err := c.ReorderTaxonomy(context.Background(), []string{"tax-2", "tax-1"}); err != nil {
		t.Fatalf("ReorderTaxonomy: %v", err)
	}
}
