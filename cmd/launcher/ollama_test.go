package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestOllamaModelPresent(t *testing.T) {
	models := []string{"bge-m3:latest", "nomic-embed-text"}
	if !ollamaModelPresent(models, "bge-m3") {
		t.Fatal("bge-m3:latest should satisfy bge-m3")
	}
	if ollamaModelPresent(models, "qwen3") {
		t.Fatal("qwen3 must be reported missing")
	}
	if ollamaModelPresent(models, "bge") {
		t.Fatal("prefix-only shorter name must not match")
	}
	if !ollamaModelPresent([]string{"bge-m3"}, "bge-m3") {
		t.Fatal("exact untagged name should match")
	}
}

func TestOllamaModelsParsesTags(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/tags" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"models":[{"name":"bge-m3:latest"},{"name":"qwen3:8b"}]}`))
	}))
	defer srv.Close()

	models, err := ollamaModels(&http.Client{Timeout: 2 * time.Second}, srv.URL)
	if err != nil {
		t.Fatalf("ollamaModels: %v", err)
	}
	if len(models) != 2 || models[0] != "bge-m3:latest" {
		t.Fatalf("unexpected models: %#v", models)
	}
}

func TestOllamaModelsHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()
	if _, err := ollamaModels(&http.Client{Timeout: time.Second}, srv.URL); err == nil {
		t.Fatal("500 response must surface an error")
	}
}
