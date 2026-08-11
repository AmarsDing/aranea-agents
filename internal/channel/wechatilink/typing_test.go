package wechatilink

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestSendTyping(t *testing.T) {
	var gotStatus int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/sendtyping" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["ilink_user_id"] != "user@im.wechat" {
			t.Errorf("ilink_user_id mismatch: %v", body["ilink_user_id"])
		}
		if body["typing_ticket"] != "ticket-1" {
			t.Errorf("typing_ticket mismatch: %v", body["typing_ticket"])
		}
		gotStatus = int(body["status"].(float64))
		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0})
	}))
	defer server.Close()

	c := newClient(server.URL, "tk", loggateway.NewNoop())
	if err := c.SendTyping(t.Context(), "user@im.wechat", "ticket-1", true); err != nil {
		t.Fatal(err)
	}
	if gotStatus != typingStatusOn {
		t.Errorf("typing on want status=%d, got %d", typingStatusOn, gotStatus)
	}

	if err := c.SendTyping(t.Context(), "user@im.wechat", "ticket-1", false); err != nil {
		t.Fatal(err)
	}
	if gotStatus != typingStatusOff {
		t.Errorf("typing off want status=%d, got %d", typingStatusOff, gotStatus)
	}
}

func TestMarkdownToWechat(t *testing.T) {
	in := "## 标题\n**加粗** 和 *斜体*\n- 项目一\n- 项目二\n---\n> 引用"
	out := markdownToWechat(in)
	for _, banned := range []string{"**", "## ", "- 项目", "---"} {
		if strings.Contains(out, banned) {
			t.Errorf("output still contains %q: %q", banned, out)
		}
	}
	if !strings.Contains(out, "加粗") || !strings.Contains(out, "项目一") {
		t.Errorf("content lost: %q", out)
	}
}
