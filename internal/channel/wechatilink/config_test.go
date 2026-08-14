package wechatilink

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// iLink 网关要求 getconfig 请求体必须携带 ilink_user_id（未文档化，
// 缺省时返回 ret=-2 errmsg="ilink_user_id required"），回归测试防止再次漏传。
func TestGetConfigSendsILinkUserID(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/getconfig" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["ilink_user_id"] != "bot@im.wechat" {
			t.Errorf("ilink_user_id mismatch: %v", body["ilink_user_id"])
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0, "typing_ticket": "ticket-1"})
	}))
	defer server.Close()

	c := newClient(server.URL, "tk", loggateway.NewNoop())
	if _, err := c.GetConfig(t.Context(), "bot@im.wechat"); err != nil {
		t.Fatal(err)
	}
}

func TestTestConnection(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0, "typing_ticket": "ticket-1"})
	}))
	defer server.Close()

	if err := TestConnection(t.Context(), nil, server.URL, "tk", "bot@im.wechat", loggateway.NewNoop()); err != nil {
		t.Fatal(err)
	}
	if err := TestConnection(t.Context(), nil, server.URL, "tk", "", loggateway.NewNoop()); err == nil {
		t.Fatal("want error for empty ilinkUserID")
	}
}
