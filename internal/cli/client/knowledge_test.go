package client_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	knowledgev1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/cli/client"
)

func TestListCollections(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/knowledge/collections" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"c1","name":"docs","status":"active"}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListCollections(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("ListCollections: %v", err)
	}
	if len(resp.Items) != 1 || resp.Total != 1 {
		t.Errorf("expected 1 collection, got %d (total %d)", len(resp.Items), resp.Total)
	}
}

func TestGetCollection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/knowledge/collections/c1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"c1","name":"docs"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	col, err := c.GetCollection(context.Background(), "c1")
	if err != nil {
		t.Fatalf("GetCollection: %v", err)
	}
	if col.Name != "docs" {
		t.Errorf("expected name docs, got %s", col.Name)
	}
}

func TestCreateCollection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/knowledge/collections" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"embeddingModel":"bge-m3"`) {
			t.Errorf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"c-new","name":"docs"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	req := &knowledgev1.CreateCollectionRequest{Name: "docs", EmbeddingModel: "bge-m3"}
	col, err := c.CreateCollection(context.Background(), req)
	if err != nil {
		t.Fatalf("CreateCollection: %v", err)
	}
	if col.Id != "c-new" {
		t.Errorf("expected id c-new, got %s", col.Id)
	}
}

func TestDeleteCollection(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/knowledge/collections/c1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteCollection(context.Background(), "c1"); err != nil {
		t.Fatalf("DeleteCollection: %v", err)
	}
}

func TestListDocuments(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/knowledge/documents" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if r.URL.Query().Get("collection_id") != "c1" {
			t.Errorf("missing collection_id: %s", r.URL.RawQuery)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"items":[{"id":"d1","source":"a.pdf","status":"indexed"}],"total":1}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ListDocuments(context.Background(), "c1", 0, 0)
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(resp.Items) != 1 {
		t.Errorf("expected 1 document, got %d", len(resp.Items))
	}
}

func TestGetDocumentContent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/knowledge/documents/d1/content" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"d1","contentText":"hello","organized":true}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	doc, err := c.GetDocumentContent(context.Background(), "d1")
	if err != nil {
		t.Fatalf("GetDocumentContent: %v", err)
	}
	if doc.ContentText != "hello" || !doc.Organized {
		t.Errorf("unexpected content: %+v", doc)
	}
}

func TestDeleteDocument(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete || r.URL.Path != "/v1/knowledge/documents/d1" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(204)
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.DeleteDocument(context.Background(), "d1"); err != nil {
		t.Fatalf("DeleteDocument: %v", err)
	}
}

func TestSearchKnowledge(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/knowledge/search" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		body, _ := io.ReadAll(r.Body)
		if !strings.Contains(string(body), `"query":"refunds"`) {
			t.Errorf("unexpected body: %s", body)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"chunks":[{"id":"ch1","docId":"d1","content":"text","score":0.8}]}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	req := &knowledgev1.SearchRequest{CollectionId: "c1", Query: "refunds", TopK: 5}
	resp, err := c.SearchKnowledge(context.Background(), req)
	if err != nil {
		t.Fatalf("SearchKnowledge: %v", err)
	}
	if len(resp.Chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(resp.Chunks))
	}
}
