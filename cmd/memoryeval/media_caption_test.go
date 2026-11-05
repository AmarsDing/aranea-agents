package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// newVLMMockServer stubs an OpenAI-compatible vision chat endpoint. It
// records call count and the last request model, and replies with a fixed
// caption unless the handler overrides behavior.
func newVLMMockServer(t *testing.T, caption string, calls *atomic.Int32, lastModel *atomic.Value) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/chat/completions") {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		var req struct {
			Model    string `json:"model"`
			Messages []struct {
				Content []struct {
					Type     string `json:"type"`
					ImageURL struct {
						URL string `json:"url"`
					} `json:"image_url"`
				} `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		calls.Add(1)
		lastModel.Store(req.Model)
		if len(req.Messages) == 0 || len(req.Messages[0].Content) < 2 ||
			!strings.HasPrefix(req.Messages[0].Content[1].ImageURL.URL, "data:image/") {
			http.Error(w, "missing image part", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{
				{"message": map[string]any{"content": caption}},
			},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

func enableTestVLM(t *testing.T, baseURL string) {
	t.Helper()
	t.Setenv("EVAL_MEDIA_VLM_BASE_URL", baseURL)
	t.Setenv("EVAL_MEDIA_VLM_API_KEY", "test-key")
}

func TestVLMCaptionInjectsIntoAddBody(t *testing.T) {
	var calls atomic.Int32
	var lastModel atomic.Value
	srv := newVLMMockServer(t, "a hand-drawn architecture sketch with three labeled boxes", &calls, &lastModel)
	enableTestVLM(t, srv.URL)

	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-vlm",
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
	want := "看图：这是架构草图\nImage attachment (part 1): a hand-drawn architecture sketch with three labeled boxes"
	if len(store.addMsgs) != 1 || store.addMsgs[0].Content != want {
		t.Fatalf("content = %+v, want caption appended to text", store.addMsgs)
	}
	if got := lastModel.Load(); got != defaultEvalVLMModel {
		t.Fatalf("vlm model = %v, want compliance default %s", got, defaultEvalVLMModel)
	}
	if calls.Load() != 1 {
		t.Fatalf("vlm calls = %d, want 1", calls.Load())
	}
}

func TestVLMCaptionImageOnlyBatchStoresCaption(t *testing.T) {
	var calls atomic.Int32
	var lastModel atomic.Value
	srv := newVLMMockServer(t, "a whiteboard photo showing a flowchart", &calls, &lastModel)
	enableTestVLM(t, srv.URL)

	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-vlm-only",
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
	// With captions enabled the image-only message now stores evidence.
	if len(store.addMsgs) != 1 ||
		store.addMsgs[0].Content != "Image attachment (part 0): a whiteboard photo showing a flowchart" {
		t.Fatalf("image-only message should store its caption, got %+v", store.addMsgs)
	}
}

func TestVLMCaptionCacheDedupesIdenticalImages(t *testing.T) {
	var calls atomic.Int32
	var lastModel atomic.Value
	srv := newVLMMockServer(t, "a red circle on a white background", &calls, &lastModel)
	enableTestVLM(t, srv.URL)

	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	img := map[string]any{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}}
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-vlm-dup",
		"messages": []map[string]any{
			{"role": "user", "content": []map[string]any{img}},
			{"role": "user", "content": []map[string]any{img}},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if calls.Load() != 1 {
		t.Fatalf("identical images must share one vlm call, got %d", calls.Load())
	}
	if len(store.addMsgs) != 2 {
		t.Fatalf("both messages should store their caption, got %+v", store.addMsgs)
	}
}

func TestVLMCaptionFailureSoftDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "upstream broken", http.StatusInternalServerError)
	}))
	t.Cleanup(srv.Close)
	enableTestVLM(t, srv.URL)

	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-vlm-err",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "看图：架构草图"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("caption failure must never fail Add: status = %d", rec.Code)
	}
	if len(store.addMsgs) != 1 || store.addMsgs[0].Content != "看图：架构草图" {
		t.Fatalf("caption failure should fall back to text-only, got %+v", store.addMsgs)
	}
}

func TestVLMCaptionTimeoutSoftDegrades(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(3 * time.Second)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": "too late"}}},
		})
	}))
	t.Cleanup(srv.Close)
	enableTestVLM(t, srv.URL)
	t.Setenv("EVAL_MEDIA_VLM_TIMEOUT_SEC", "1")

	store := &fakeEvalStore{}
	h := newTestHandler(store, "")
	start := time.Now()
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/add", "", map[string]any{
		"user_id": "u-vlm-slow",
		"messages": []map[string]any{{
			"role": "user",
			"content": []map[string]any{
				{"type": "text", "text": "看图"},
				{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
			},
		}},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d", rec.Code)
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("add blocked %v on a slow vlm; timeout must bound it", elapsed)
	}
	if len(store.addMsgs) != 1 || store.addMsgs[0].Content != "看图" {
		t.Fatalf("slow caption should fall back to text-only, got %+v", store.addMsgs)
	}
}

func TestVLMSearchImageCaptionJoinsQuery(t *testing.T) {
	var calls atomic.Int32
	var lastModel atomic.Value
	srv := newVLMMockServer(t, "a system diagram with a blue database cylinder", &calls, &lastModel)
	enableTestVLM(t, srv.URL)

	fake := &fakeEvalStore{}
	h := newTestHandler(fake, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-vlm-search",
		"query": []map[string]any{
			{"type": "text", "text": "我之前发的图里数据库长什么样"},
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
		},
		"top_k": 5,
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rec.Code, rec.Body.String())
	}
	want := "我之前发的图里数据库长什么样 a system diagram with a blue database cylinder"
	if fake.searchQuery != want {
		t.Fatalf("search query = %q, want caption folded in", fake.searchQuery)
	}
}

func TestVLMSearchImageOnlyQueryUsesCaption(t *testing.T) {
	var calls atomic.Int32
	var lastModel atomic.Value
	srv := newVLMMockServer(t, "a photo of a ginger cat sleeping on a keyboard", &calls, &lastModel)
	enableTestVLM(t, srv.URL)

	fake := &fakeEvalStore{}
	h := newTestHandler(fake, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-vlm-imgq",
		"query": []map[string]any{
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
		},
	})
	if rec.Code != http.StatusOK {
		t.Fatalf("image-only query with caption must not 400: status = %d, body = %s", rec.Code, rec.Body.String())
	}
	if fake.searchQuery != "a photo of a ginger cat sleeping on a keyboard" {
		t.Fatalf("image-only query should become the caption, got %q", fake.searchQuery)
	}
}

// Disabled component: image parts in search are ignored exactly as before.
func TestSearchImageOnlyQueryWithoutVLMStays400(t *testing.T) {
	fake := &fakeEvalStore{}
	h := newTestHandler(fake, "")
	rec := doJSON(t, h, http.MethodPost, "/v1/memory/search", "", map[string]any{
		"user_id": "u-novlm",
		"query": []map[string]any{
			{"type": "image_url", "image_url": map[string]any{"url": "data:image/png;base64," + testTinyPNGBase64}},
		},
	})
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("caption-only route must keep 400 on image-only query, got %d", rec.Code)
	}
}

func TestVLMContentTextToleratesPartsArray(t *testing.T) {
	raw := json.RawMessage(`[{"type":"text","text":"first"},{"type":"text","text":"second"},{"type":"refusal","text":"no"}]`)
	if got := vlmContentText(raw); got != "first\nsecond" {
		t.Fatalf("parts content = %q", got)
	}
	if got := vlmContentText(json.RawMessage(`"plain"`)); got != "plain" {
		t.Fatalf("string content = %q", got)
	}
}
