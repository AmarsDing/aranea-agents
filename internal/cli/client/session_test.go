package client_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/cli/client"
)

func TestArchiveSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions/sess-1/archive" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if err := c.ArchiveSession(context.Background(), "sess-1"); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}
}

func TestRestoreSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions/sess-1/restore" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sess-1","status":"active"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	sess, err := c.RestoreSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("RestoreSession: %v", err)
	}
	if sess.Status != "active" {
		t.Errorf("expected status active, got %s", sess.Status)
	}
}

func TestPinSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions/sess-1/pin" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sess-1","pinned_at":"2026-07-28T00:00:00Z"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	sess, err := c.PinSession(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("PinSession: %v", err)
	}
	if sess.PinnedAt == "" {
		t.Error("expected pinned_at set")
	}
}

func TestUnpinSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions/sess-1/unpin" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"id":"sess-1"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	if _, err := c.UnpinSession(context.Background(), "sess-1"); err != nil {
		t.Fatalf("UnpinSession: %v", err)
	}
}

func TestCompactSession(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/sessions:compact" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"compacted":true,"from_turn":1,"to_turn":5,"estimated_tokens_before":1000,"estimated_tokens_after":200}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.CompactSession(context.Background(), "sess-1", "keep this")
	if err != nil {
		t.Fatalf("CompactSession: %v", err)
	}
	if !resp.Compacted || resp.ToTurn != 5 {
		t.Errorf("unexpected response: %+v", resp)
	}
	if gotBody["sessionId"] != "sess-1" {
		t.Errorf("expected sessionId=sess-1 in body, got %v", gotBody["sessionId"])
	}
	if gotBody["preserveInstruction"] != "keep this" {
		t.Errorf("expected preserveInstruction in body, got %v", gotBody["preserveInstruction"])
	}
}

func TestExportSession(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/sessions/sess-1/export" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.Query().Get("format"); got != "markdown" {
			t.Errorf("expected format=markdown, got %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"content":"# Title","filename":"sess-1.md","content_type":"text/markdown"}`))
	}))
	defer srv.Close()
	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	resp, err := c.ExportSession(context.Background(), "sess-1", "markdown")
	if err != nil {
		t.Fatalf("ExportSession: %v", err)
	}
	if resp.Content != "# Title" || resp.Filename != "sess-1.md" {
		t.Errorf("unexpected response: %+v", resp)
	}
}
