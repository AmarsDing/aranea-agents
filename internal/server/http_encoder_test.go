package server

import (
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/genproto/googleapis/api/httpbody"
)

// TestUtf8ResponseEncoder_HttpBodyPassthrough 钉死 K-3 ics 订阅的原始字节直通：
// *httpbody.HttpBody 不得被 JSON/base64 包裹（日历客户端只认 text/calendar
// 原始流）；普通 JSON 响应仍走 codec 且带 charset=utf-8（不回归）。
func TestUtf8ResponseEncoder_HttpBodyPassthrough(t *testing.T) {
	raw := "BEGIN:VCALENDAR\r\nVERSION:2.0\r\nBEGIN:VEVENT\r\nUID:ev_1@aranea\r\nEND:VEVENT\r\nEND:VCALENDAR\r\n"
	w := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/v1/office/calendar/feeds/abc", nil)
	if err := utf8ResponseEncoder(w, r, &httpbody.HttpBody{ContentType: "text/calendar; charset=utf-8", Data: []byte(raw)}); err != nil {
		t.Fatalf("encode httpbody: %v", err)
	}
	if ct := w.Header().Get("Content-Type"); ct != "text/calendar; charset=utf-8" {
		t.Fatalf("content-type = %q, want text/calendar; charset=utf-8", ct)
	}
	if w.Body.String() != raw {
		t.Fatalf("body must be raw ics bytes, got %q", w.Body.String()[:minInt(60, w.Body.Len())])
	}

	// 普通 JSON 响应不回归：codec 输出 + charset。
	w2 := httptest.NewRecorder()
	r2 := httptest.NewRequest("GET", "/v1/office/events", nil)
	r2.Header.Set("Accept", "application/json")
	if err := utf8ResponseEncoder(w2, r2, map[string]string{"title": "日程"}); err != nil {
		t.Fatalf("encode json: %v", err)
	}
	if ct := w2.Header().Get("Content-Type"); !strings.Contains(ct, "application/json") || !strings.Contains(ct, "charset=utf-8") {
		t.Fatalf("json content-type = %q, want application/json; charset=utf-8", ct)
	}
	if !strings.Contains(w2.Body.String(), "日程") {
		t.Fatalf("json body must carry utf-8 payload, got %q", w2.Body.String())
	}
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
