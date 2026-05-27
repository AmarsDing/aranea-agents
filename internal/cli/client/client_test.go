package client_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/cli/client"
)

func TestClient_BearerAndUA(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if auth := r.Header.Get("Authorization"); auth != "Bearer test-token" {
			t.Errorf("expected Bearer test-token, got %q", auth)
		}
		ua := r.Header.Get("User-Agent")
		if len(ua) == 0 {
			t.Error("missing User-Agent header")
		}
		if accept := r.Header.Get("Accept"); accept != "application/json" {
			t.Errorf("expected Accept: application/json, got %q", accept)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL, "test-token", "dev", false, nil)
	err := c.Do(context.Background(), http.MethodGet, "/v1/test", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestClient_ErrorDecode_401(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"reason":"UNAUTHENTICATED","message":"Token is invalid"}`))
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL, "", "dev", false, nil)
	err := c.Do(context.Background(), http.MethodGet, "/v1/test", nil, nil)
	if err == nil {
		t.Fatal("expected error for 401, got nil")
	}
	t.Logf("error: %v", err)
}

func TestClient_ErrorDecode_404(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"reason":"NOT_FOUND","message":"resource not found"}`))
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	err := c.Do(context.Background(), http.MethodGet, "/v1/agents/x", nil, nil)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}

func TestClient_ErrorDecode_500(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte(`internal error`))
	}))
	defer srv.Close()

	c := client.NewClient(srv.URL, "tok", "dev", false, nil)
	err := c.Do(context.Background(), http.MethodGet, "/v1/test", nil, nil)
	if err == nil {
		t.Fatal("expected error for 500, got nil")
	}
}
