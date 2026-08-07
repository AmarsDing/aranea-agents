package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type fakeEvalStore struct {
	addUser, addSession string
	addMsgs             []biz.EvalMessage
	addN                int
	addErr              error

	searchUser, searchQuery string
	searchTopK              int32
	searchItems             []biz.EvalMemoryItem
	searchErr               error
}

func (f *fakeEvalStore) AddMessages(_ context.Context, userID, sessionID string, msgs []biz.EvalMessage) (int, error) {
	f.addUser, f.addSession, f.addMsgs = userID, sessionID, msgs
	if f.addErr != nil {
		return 0, f.addErr
	}
	if f.addN > 0 {
		return f.addN, nil
	}
	return len(msgs), nil
}

func (f *fakeEvalStore) SearchMemories(_ context.Context, userID, query string, topK int32) ([]biz.EvalMemoryItem, error) {
	f.searchUser, f.searchQuery, f.searchTopK = userID, query, topK
	return f.searchItems, f.searchErr
}

func newTestHandler(store biz.EvalMemoryStore, token string) http.Handler {
	return newEvalServer(store, token, loggateway.NewNoop()).routes()
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode body: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

// ---------- Add ----------

func TestHandleAdd_Success(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"request_id": "req-1",
		"user_id":    "u-1",
		"session_id": "s-1",
		"messages": []map[string]any{
			{"role": "user", "content": "我喜欢咖啡"},
			{"role": "assistant", "text": "已记住"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success   bool   `json:"success"`
		RequestID string `json:"request_id"`
		Timestamp string `json:"timestamp"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false, body = %s", rec.Body.String())
	}
	if resp.RequestID != "req-1" {
		t.Fatalf("request_id = %q, want req-1", resp.RequestID)
	}
	if resp.Timestamp == "" {
		t.Fatal("timestamp must not be empty")
	}
	if store.addUser != "u-1" || store.addSession != "s-1" {
		t.Fatalf("store got user=%q session=%q", store.addUser, store.addSession)
	}
	if len(store.addMsgs) != 2 {
		t.Fatalf("store got %d messages, want 2", len(store.addMsgs))
	}
	if store.addMsgs[0].Content != "我喜欢咖啡" || store.addMsgs[0].Role != "user" {
		t.Fatalf("msg[0] = %+v", store.addMsgs[0])
	}
	if store.addMsgs[1].Content != "已记住" {
		t.Fatalf("msg[1] text fallback failed: %+v", store.addMsgs[1])
	}
}

func TestHandleAdd_GeneratesRequestID(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id":  "u-1",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if resp["request_id"] == "" || resp["request_id"] == nil {
		t.Fatal("request_id must be generated when absent")
	}
}

func TestHandleAdd_MissingUserID(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if ok, _ := resp["success"].(bool); ok {
		t.Fatal("success must be false on 400")
	}
}

func TestHandleAdd_NoMessages(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{"user_id": "u-1"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAdd_AllBlankMessages(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id":  "u-1",
		"messages": []map[string]any{{"role": "user", "content": "   "}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAdd_InvalidJSON(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "")
	req := httptest.NewRequest(http.MethodPost, "/v1/memory/add", bytes.NewBufferString("{bad json"))
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleAdd_StoreError(t *testing.T) {
	store := &fakeEvalStore{addErr: errors.New("db down")}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id":  "u-1",
		"messages": []map[string]any{{"role": "user", "content": "hi"}},
	})
	// 5xx is platform-retryable per the integration contract.
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if ok, _ := resp["success"].(bool); ok {
		t.Fatal("success must be false on store error")
	}
}

// ---------- Search ----------

func TestHandleSearch_Success(t *testing.T) {
	store := &fakeEvalStore{searchItems: []biz.EvalMemoryItem{
		{ID: "f-1", Content: "用户喜欢咖啡", Score: 0.91, Timestamp: "2026-08-01T00:00:00Z"},
		{ID: "f-2", Content: "用户不喝牛奶", Score: 0.77, Timestamp: "2026-08-02T00:00:00Z"},
	}}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-1",
		"query":   "用户的饮食偏好",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Data []biz.EvalMemoryItem `json:"data"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].ID != "f-1" || resp.Data[0].Score != 0.91 {
		t.Fatalf("data = %+v", resp.Data)
	}
	// Default top_k per platform contract.
	if store.searchTopK != 100 {
		t.Fatalf("topK = %d, want default 100", store.searchTopK)
	}
	if store.searchUser != "u-1" || store.searchQuery != "用户的饮食偏好" {
		t.Fatalf("store got user=%q query=%q", store.searchUser, store.searchQuery)
	}
}

func TestHandleSearch_CustomTopK(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-1",
		"query":   "q",
		"top_k":   5,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if store.searchTopK != 5 {
		t.Fatalf("topK = %d, want 5", store.searchTopK)
	}
}

func TestHandleSearch_MissingUserID(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{"query": "q"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleSearch_MissingQuery(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{"user_id": "u-1"})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rec.Code)
	}
}

func TestHandleSearch_StoreError(t *testing.T) {
	store := &fakeEvalStore{searchErr: errors.New("db down")}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-1",
		"query":   "q",
	})
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", rec.Code)
	}
}

func TestHandleSearch_EmptyResultIsArray(t *testing.T) {
	store := &fakeEvalStore{searchItems: nil}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-1",
		"query":   "q",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	var raw map[string]json.RawMessage
	_ = json.NewDecoder(rec.Body).Decode(&raw)
	if string(raw["data"]) != "[]" {
		t.Fatalf("data must be [] not null, got %s", string(raw["data"]))
	}
}

// ---------- Auth & health ----------

func TestAuth_RejectsWrongToken(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "secret")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "wrong", map[string]any{
		"user_id": "u-1",
		"query":   "q",
	})
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", rec.Code)
	}
}

func TestAuth_AcceptsBearer(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "secret")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "secret", map[string]any{
		"user_id": "u-1",
		"query":   "q",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestAuth_AcceptsXApiKey(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "secret")
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(map[string]any{"user_id": "u-1", "query": "q"})
	req := httptest.NewRequest(http.MethodPost, "/v1/memory/search", &buf)
	req.Header.Set("X-Api-Key", "secret")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}

func TestHealthz(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "secret") // healthz bypasses auth
	rec := doJSON(t, h, http.MethodGet, "/healthz", "", nil)
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
}
