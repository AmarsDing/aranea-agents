package computeruse

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	bizcu "aranea-agents/internal/biz/computeruse"
	"aranea-agents/pkg/loggateway"
)

// newOmniTestServer 返回可编程的 OmniParser httptest 服务器与调用计数器。
func newOmniTestServer(t *testing.T, parseHandler func(w http.ResponseWriter, r *http.Request)) (*httptest.Server, *int32) {
	t.Helper()
	var probeCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/probe/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&probeCalls, 1)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Omniparser is ready"))
	})
	mux.HandleFunc("/parse/", parseHandler)
	return httptest.NewServer(mux), &probeCalls
}

func okParseHandler(w http.ResponseWriter, r *http.Request) {
	var req map[string]any
	_ = json.NewDecoder(r.Body).Decode(&req)
	if req["base64_image"] == "" {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"parsed_content_list": []map[string]any{
			{"type": "text", "content": "保存", "bbox": [4]float64{0.1, 0.2, 0.3, 0.4}, "interactivity": false, "source": "box_yolo_content_ocr"},
			{"type": "icon", "content": "search", "bbox": [4]float64{0.5, 0.5, 0.6, 0.6}, "interactivity": true, "source": "box_yolo_content_yolo"},
		},
	})
}

func TestOmniParserParse_MapsElements(t *testing.T) {
	srv, _ := newOmniTestServer(t, okParseHandler)
	defer srv.Close()
	c := NewOmniParserClient(srv.URL, loggateway.NewNoop())

	img := bizcu.Image{PNG: []byte("fake"), Width: 200, Height: 100}
	els, err := c.Parse(context.Background(), img)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if len(els) != 2 {
		t.Fatalf("len=%d, want 2", len(els))
	}
	// 归一化 bbox × 图像尺寸 → 物理像素。
	if got := els[0].BBox; got.X != 20 || got.Y != 20 || got.W != 40 || got.H != 20 {
		t.Fatalf("els[0].BBox=%+v, want {20 20 40 20}", got)
	}
	if els[0].Name != "保存" || els[0].Type != "text" || els[0].Interactivity {
		t.Fatalf("els[0] 映射错误: %+v", els[0])
	}
	if !els[1].Interactivity || els[1].Type != "icon" {
		t.Fatalf("els[1] 映射错误: %+v", els[1])
	}
	for i, el := range els {
		if el.Source != "vision" {
			t.Fatalf("els[%d].Source=%q, want vision", i, el.Source)
		}
	}
}

func TestOmniParserParse_ServerError(t *testing.T) {
	srv, _ := newOmniTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	defer srv.Close()
	c := NewOmniParserClient(srv.URL, loggateway.NewNoop())
	_, err := c.Parse(context.Background(), bizcu.Image{PNG: []byte("x"), Width: 10, Height: 10})
	if err == nil {
		t.Fatalf("500 应返回错误")
	}
}

func TestOmniParserAvailable_Healthy(t *testing.T) {
	srv, probeCalls := newOmniTestServer(t, okParseHandler)
	defer srv.Close()
	c := NewOmniParserClient(srv.URL, loggateway.NewNoop())
	if !c.Available(context.Background()) {
		t.Fatalf("健康服务应 Available")
	}
	if atomic.LoadInt32(probeCalls) != 1 {
		t.Fatalf("probe 应被调用 1 次，got %d", *probeCalls)
	}
	// TTL 内第二次调用应走缓存，不再真探测。
	if !c.Available(context.Background()) {
		t.Fatalf("TTL 内应仍 Available")
	}
	if atomic.LoadInt32(probeCalls) != 1 {
		t.Fatalf("TTL 内不应重复探测，probe=%d", *probeCalls)
	}
}

func TestOmniParserAvailable_DownAndRecover(t *testing.T) {
	var down atomic.Bool
	down.Store(true)
	mux := http.NewServeMux()
	mux.HandleFunc("/probe/", func(w http.ResponseWriter, r *http.Request) {
		if down.Load() {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewOmniParserClient(srv.URL, loggateway.NewNoop())
	c.probeTTL = 50 * time.Millisecond // 测试用短 TTL
	if c.Available(context.Background()) {
		t.Fatalf("服务宕机应不可用")
	}
	// 恢复后 TTL 内仍缓存 false；TTL 过后真探测恢复 true。
	down.Store(false)
	if c.Available(context.Background()) {
		t.Fatalf("TTL 内应仍缓存不可用")
	}
	time.Sleep(60 * time.Millisecond)
	if !c.Available(context.Background()) {
		t.Fatalf("TTL 过期后应重新探测并恢复")
	}
}

func TestOmniParserParse_FailureMarksUnavailable(t *testing.T) {
	var probeCalls int32
	mux := http.NewServeMux()
	mux.HandleFunc("/probe/", func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&probeCalls, 1)
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("/parse/", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := NewOmniParserClient(srv.URL, loggateway.NewNoop())
	c.probeTTL = time.Minute // 长 TTL：Parse 失败必须主动失效缓存
	if !c.Available(context.Background()) {
		t.Fatalf("初始应健康")
	}
	_, _ = c.Parse(context.Background(), bizcu.Image{PNG: []byte("x"), Width: 10, Height: 10})
	if c.Available(context.Background()) {
		t.Fatalf("Parse 失败后应立即降级为不可用（缓存失效）")
	}
	// 降级标记生效期间不得真探测。
	if got := atomic.LoadInt32(&probeCalls); got != 1 {
		t.Fatalf("降级期间不应重复探测，probe=%d", got)
	}
}

func TestOmniParserParse_SendsBase64(t *testing.T) {
	var gotB64 string
	srv, _ := newOmniTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		var req map[string]any
		_ = json.NewDecoder(r.Body).Decode(&req)
		gotB64, _ = req["base64_image"].(string)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"parsed_content_list": []any{}})
	})
	defer srv.Close()
	c := NewOmniParserClient(srv.URL, loggateway.NewNoop())
	png := []byte{0x89, 0x50, 0x4E, 0x47}
	_, err := c.Parse(context.Background(), bizcu.Image{PNG: png, Width: 1, Height: 1})
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if gotB64 != base64.StdEncoding.EncodeToString(png) {
		t.Fatalf("base64 不匹配: %q", gotB64)
	}
}
