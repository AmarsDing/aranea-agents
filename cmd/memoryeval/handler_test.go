package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
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
	searchOptions           []string
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

func (f *fakeEvalStore) SearchMemories(_ context.Context, userID, query string, topK int32, options ...string) ([]biz.EvalMemoryItem, error) {
	f.searchUser, f.searchQuery, f.searchTopK = userID, query, topK
	f.searchOptions = append([]string(nil), options...)
	return f.searchItems, f.searchErr
}

func newTestHandler(store biz.EvalMemoryStore, token string) http.Handler {
	return newEvalServer(store, token, loggateway.NewNoop()).routes()
}

func TestSearchOptionsReachStoreWithoutReplacingQuery(t *testing.T) {
	fake := &fakeEvalStore{}
	r := doJSON(t, newTestHandler(fake, ""), http.MethodPost, "/v1/memory/search", "", map[string]any{"user_id": "u", "query": "Original question", "top_k": 7, "options": []string{"A. alpha", "B. beta"}})
	if r.Code != http.StatusOK || fake.searchQuery != "Original question" || len(fake.searchOptions) != 2 || fake.searchOptions[1] != "B. beta" {
		t.Fatalf("search options lost or query rewritten: status=%d fake=%+v", r.Code, fake)
	}
}

func TestAddDerivesStableMessageIdentityFromRequest(t *testing.T) {
	fake := &fakeEvalStore{}
	h := newTestHandler(fake, "")
	body := map[string]any{"request_id": "request-stable", "user_id": "u", "session_id": "s", "messages": []map[string]string{{"role": "user", "content": "first"}, {"role": "user", "content": "second"}}}
	if r := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", body); r.Code != http.StatusOK {
		t.Fatal(r.Body.String())
	}
	first := fake.addMsgs[0].MessageID
	if first == "" || first == fake.addMsgs[1].MessageID {
		t.Fatal("missing per-message identity")
	}
	if r := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", body); r.Code != http.StatusOK || fake.addMsgs[0].MessageID != first {
		t.Fatal("request replay changed message identity")
	}
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

// Cycle 2 contract: messages[].timestamp is sent as Unix milliseconds (JSON
// number) when the source has one. The handler must tolerate it instead of
// failing the whole Add request at JSON decode time.
func TestHandleAdd_NumericTimestamp(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"request_id": "req-ts-1",
		"user_id":    "u-1",
		"session_id": "s-1",
		"messages": []map[string]any{
			{"role": "user", "timestamp": 1704067200000, "content": "raw memory text"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Success {
		t.Fatalf("success = false, body = %s", rec.Body.String())
	}
	if len(store.addMsgs) != 1 {
		t.Fatalf("store got %d messages, want 1", len(store.addMsgs))
	}
	if got := store.addMsgs[0].Timestamp; got != "1704067200000" {
		t.Fatalf("timestamp = %q, want %q", got, "1704067200000")
	}
}

// Cycle 2 contract: the Add response must echo user_id and session_id.
func TestHandleAdd_EchoesUserAndSession(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"request_id": "req-echo",
		"user_id":    "u-echo",
		"session_id": "s-echo",
		"messages":   []map[string]any{{"role": "user", "content": "hi"}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success   bool   `json:"success"`
		RequestID string `json:"request_id"`
		UserID    string `json:"user_id"`
		SessionID string `json:"session_id"`
	}
	if err := json.NewDecoder(rec.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if !resp.Success {
		t.Fatalf("success = false, body = %s", rec.Body.String())
	}
	if resp.RequestID != "req-echo" {
		t.Fatalf("request_id = %q, want req-echo", resp.RequestID)
	}
	if resp.UserID != "u-echo" {
		t.Fatalf("user_id = %q, want u-echo", resp.UserID)
	}
	if resp.SessionID != "s-echo" {
		t.Fatalf("session_id = %q, want s-echo", resp.SessionID)
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
		{ID: "f-1", Content: "用户喜欢咖啡", Score: 0.91, Timestamp: "2026-08-01T00:00:00Z", CreatedAt: "2026-08-01T00:00:00Z"},
		{ID: "f-2", Content: "用户不喝牛奶", Score: 0.77, Timestamp: "2026-08-02T00:00:00Z", CreatedAt: "2026-08-02T00:00:00Z"},
	}}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-1",
		"query":   "用户的饮食偏好",
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	body := rec.Body.String()
	var resp struct {
		Data []biz.EvalMemoryItem `json:"data"`
	}
	if err := json.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 2 || resp.Data[0].ID != "f-1" || resp.Data[0].Score != 0.91 {
		t.Fatalf("data = %+v", resp.Data)
	}
	// Cycle-2 API Guide: Search evidence carries created_at; cycle-1 used
	// timestamp. Both must be on the wire so either contract can parse it.
	for _, key := range []string{`"timestamp":"2026-08-01T00:00:00Z"`, `"created_at":"2026-08-01T00:00:00Z"`} {
		if !strings.Contains(body, key) {
			t.Fatalf("search body missing %s: %s", key, body)
		}
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

// ---------- Multimodal content parts (2026-10-23 官网核实契约) ----------
// Contract: content/query may be a string or an ordered ContentPart array;
// parts are only text / image_url (inline base64 Data URI, JPEG/PNG/WebP,
// <=10 MiB decoded per image, <=30 MiB per Add). Unknown part types are
// skipped, never an error (Q23: must not 400 on image fields).

const testTinyPNGBase64 = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg=="

func TestHandleAdd_ContentPartsArrayJoinsTextInOrder(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-parts",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "第一段"},
				{"type": "text", "text": "第二段"},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 {
		t.Fatalf("store got %d messages, want 1", len(store.addMsgs))
	}
	if store.addMsgs[0].Content != "第一段\n第二段" {
		t.Fatalf("joined content = %q, want text parts joined with newline", store.addMsgs[0].Content)
	}
}

func TestHandleAdd_ImagePartNeverFailsTheRequest(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-img",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "看图：这是架构草图"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 || store.addMsgs[0].Content != "看图：这是架构草图" {
		t.Fatalf("store msgs = %+v", store.addMsgs)
	}
	// Media sink disabled by default: no parts evidence is attached.
	if len(store.addMsgs[0].Parts) != 0 {
		t.Fatalf("media sink disabled, expected no Parts, got %+v", store.addMsgs[0].Parts)
	}
}

// Q23: a batch whose only payload is an image must still succeed; the
// caption-only intake simply has no text evidence to store.
func TestHandleAdd_ImageOnlyBatchAcknowledged(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-imgonly",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("image-only add must not fail (Q23): status = %d, body = %s", rec.Code, rec.Body.String())
	}
	var resp struct {
		Success bool `json:"success"`
	}
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if !resp.Success {
		t.Fatalf("success=false on image-only batch: %s", rec.Body.String())
	}
	if len(store.addMsgs) != 0 {
		t.Fatalf("no text evidence should reach the store, got %+v", store.addMsgs)
	}
}

func TestHandleAdd_RemoteImageURLSkipped(t *testing.T) {
	dir := t.TempDir()
	store := &fakeEvalStore{}
	srv := newEvalServer(store, "", loggateway.NewNoop())
	srv.mediaDir = dir
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-remote",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "远程图不入存"},
				{"type": "image_url", "image_url": map[string]any{"url": "https://example.com/a.png"}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 || store.addMsgs[0].Content != "远程图不入存" {
		t.Fatalf("store msgs = %+v", store.addMsgs)
	}
	if len(store.addMsgs[0].Parts) != 0 {
		t.Fatalf("remote image must not be persisted, got %+v", store.addMsgs[0].Parts)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("media dir must stay empty for remote URLs, got %d entries", len(entries))
	}
}

func TestHandleAdd_UnknownPartTypeSkipped(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-unknown",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "video", "source": "clip.mp4"},
				{"type": "text", "text": "正文保留"},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 || store.addMsgs[0].Content != "正文保留" {
		t.Fatalf("store msgs = %+v", store.addMsgs)
	}
}

func TestHandleAdd_MediaSinkPersistsImagePart(t *testing.T) {
	dir := t.TempDir()
	store := &fakeEvalStore{}
	srv := newEvalServer(store, "", loggateway.NewNoop())
	srv.mediaDir = dir
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", map[string]any{
		"request_id": "req-media",
		"user_id":    "u-media",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "草图见附图"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 {
		t.Fatalf("store msgs = %+v", store.addMsgs)
	}
	parts := store.addMsgs[0].Parts
	if len(parts) != 1 {
		t.Fatalf("Parts = %+v, want exactly one persisted image", parts)
	}
	p := parts[0]
	if p.Type != "image_url" || p.Mime != "image/png" || p.Index != 1 || p.Bytes <= 0 || p.Ref == "" {
		t.Fatalf("part metadata incomplete: %+v", p)
	}
	raw, err := os.ReadFile(p.Ref)
	if err != nil {
		t.Fatalf("evidence file unreadable: %v (ref=%s)", err, p.Ref)
	}
	if len(raw) != p.Bytes {
		t.Fatalf("evidence size = %d, part says %d", len(raw), p.Bytes)
	}
	want, _ := base64.StdEncoding.DecodeString(testTinyPNGBase64)
	if !bytes.Equal(raw, want) {
		t.Fatal("evidence bytes do not round-trip the original image")
	}
}

func TestHandleAdd_OversizedImageSkipped(t *testing.T) {
	dir := t.TempDir()
	store := &fakeEvalStore{}
	srv := newEvalServer(store, "", loggateway.NewNoop())
	srv.mediaDir = dir
	srv.maxImageBytes = 8 // the tiny png decodes to 68 bytes
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-big",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
				{"type": "text", "text": "超限图被丢弃"},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 || store.addMsgs[0].Content != "超限图被丢弃" {
		t.Fatalf("store msgs = %+v", store.addMsgs)
	}
	if len(store.addMsgs[0].Parts) != 0 {
		t.Fatalf("oversized image must not be persisted, got %+v", store.addMsgs[0].Parts)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 0 {
		t.Fatalf("media dir must stay empty for oversized images, got %d entries", len(entries))
	}
}

// EVAL_MAX_BODY_BYTES (default 0 = disabled) guards the payload cap; over
// the cap the platform expects a retryable-size error, not a truncated parse.
func TestHandleAdd_BodyLimitReturns413(t *testing.T) {
	srv := newEvalServer(&fakeEvalStore{}, "", loggateway.NewNoop())
	srv.maxBodyBytes = 64
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id":  "u-limit",
		"messages": []map[string]any{{"role": "user", "content": "this body is definitely longer than sixty-four bytes of json"}},
	})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body = %s", rec.Code, rec.Body.String())
	}
	var resp map[string]any
	_ = json.NewDecoder(rec.Body).Decode(&resp)
	if ok, _ := resp["success"].(bool); ok {
		t.Fatal("success must be false on 413")
	}
}

func TestHandleSearch_BodyLimitReturns413(t *testing.T) {
	srv := newEvalServer(&fakeEvalStore{}, "", loggateway.NewNoop())
	srv.maxBodyBytes = 32
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-limit", "query": "a query that pushes the body beyond thirty-two bytes",
	})
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("status = %d, want 413, body = %s", rec.Code, rec.Body.String())
	}
}

// Search query may also arrive as a ContentPart array; text parts join for
// retrieval and image parts are ignored (caption-only route).
func TestHandleSearch_QueryContentPartsArray(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-1",
		"query": []map[string]any{
			{"type": "text", "text": "近一周"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			{"type": "text", "text": "咖啡偏好"},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if store.searchQuery != "近一周 咖啡偏好" {
		t.Fatalf("query = %q, want text parts joined with space", store.searchQuery)
	}
}

func TestHandleSearch_QueryImageOnlyRejected(t *testing.T) {
	h := newTestHandler(&fakeEvalStore{}, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-1",
		"query":   []map[string]any{{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}}},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400 when no text remains after part filtering", rec.Code)
	}
}

// Regression: the media evidence path must not break the existing
// flexTimestamp / text fallback / blank-message behaviors.
func TestHandleAdd_BlankContentWithValidImageStillAcked(t *testing.T) {
	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-mixed",
		"messages": []map[string]any{
			{"role": "user", "content": "   "},
			{"role": "user", "content": []map[string]any{
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/webp;base64," + testTinyPNGBase64}},
			}},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 0 {
		t.Fatalf("no text evidence should reach the store, got %+v", store.addMsgs)
	}
}

func TestMediaEvidencePathSanitizesUserScope(t *testing.T) {
	dir := t.TempDir()
	store := &fakeEvalStore{}
	srv := newEvalServer(store, "", loggateway.NewNoop())
	srv.mediaDir = dir
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", map[string]any{
		"request_id": "req-path",
		"user_id":    "../evil/../../u",
		"messages": []map[string]any{{
			"role":    "user",
			"content": []map[string]any{{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}}},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 0 {
		t.Fatalf("image-only message must not reach the store, got %+v", store.addMsgs)
	}
	entries, _ := os.ReadDir(dir)
	if len(entries) != 1 {
		t.Fatalf("expected one sanitized user dir under media root, got %v", entries)
	}
	sub := entries[0].Name()
	if strings.Contains(sub, "..") || sub == "" {
		t.Fatalf("unsanitized path token: %q", sub)
	}
	files, err := os.ReadDir(filepath.Join(dir, sub))
	if err != nil || len(files) != 1 {
		t.Fatalf("expected one evidence file under %s: %d files, err=%v", sub, len(files), err)
	}
}

// An over-budget image must be dropped without consuming budget: later
// in-budget images in the same Add still persist (order-independent).
func TestHandleAdd_MediaBudgetOverflowDoesNotBurnLaterImages(t *testing.T) {
	dir := t.TempDir()
	store := &fakeEvalStore{}
	srv := newEvalServer(store, "", loggateway.NewNoop())
	srv.mediaDir = dir
	srv.maxMediaBytes = 140 // tiny png is 68 bytes: first fits, second overflows, third fits again
	bigger := base64.StdEncoding.EncodeToString(make([]byte, 100))
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", map[string]any{
		"request_id": "req-budget",
		"user_id":    "u-budget",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "budget ordering"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + bigger}},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 {
		t.Fatalf("store msgs = %+v", store.addMsgs)
	}
	parts := store.addMsgs[0].Parts
	if len(parts) != 2 {
		t.Fatalf("expected image 1 and image 3 persisted, image 2 dropped; Parts = %+v", parts)
	}
	if parts[0].Index != 1 || parts[1].Index != 3 {
		t.Fatalf("persisted the wrong images: %+v", parts)
	}
}

// A replayed Add with the same request_id is idempotent for evidence: the
// first copy is kept, the request still succeeds, no corrupt rewrite.
func TestHandleAdd_MediaEvidenceReplayIsIdempotent(t *testing.T) {
	dir := t.TempDir()
	store := &fakeEvalStore{}
	srv := newEvalServer(store, "", loggateway.NewNoop())
	srv.mediaDir = dir
	body := map[string]any{
		"request_id": "req-replay",
		"user_id":    "u-replay",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "replay"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			},
		}},
	}
	if rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", body); rec.Code != http.StatusOK {
		t.Fatalf("first add: %d %s", rec.Code, rec.Body.String())
	}
	if rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", body); rec.Code != http.StatusOK {
		t.Fatalf("replayed add must stay ok: %d %s", rec.Code, rec.Body.String())
	}
	entries, err := os.ReadDir(filepath.Join(dir, "u-replay"))
	if err != nil || len(entries) != 1 {
		t.Fatalf("replay must not duplicate evidence: %d files, err=%v", len(entries), err)
	}
}

// Legal data-URI variants (mediatype parameters, case-insensitive scheme and
// base64 marker) must parse instead of being dropped as unsupported.
func TestHandleAdd_DataURIVariantsAccepted(t *testing.T) {
	dir := t.TempDir()
	store := &fakeEvalStore{}
	srv := newEvalServer(store, "", loggateway.NewNoop())
	srv.mediaDir = dir
	rec := doJSON(t, srv.routes(), http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-variants",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "variants"},
				{"type": "image_url", "image_url": map[string]any{"url": "DATA:image/png;charset=utf-8;BASE64," + testTinyPNGBase64}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if len(store.addMsgs) != 1 || len(store.addMsgs[0].Parts) != 1 {
		t.Fatalf("parameterized data URI must persist: %+v", store.addMsgs)
	}
	if store.addMsgs[0].Parts[0].Mime != "image/png" {
		t.Fatalf("mime = %q, want image/png after param strip", store.addMsgs[0].Parts[0].Mime)
	}
}
