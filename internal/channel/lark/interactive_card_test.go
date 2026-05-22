package lark

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSendInteractiveMessage_postsCardJSON(t *testing.T) {
	var gotBody map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/open-apis/im/v1/messages") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		if r.Method != http.MethodPost {
			t.Fatalf("method=%s", r.Method)
		}
		raw, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(raw, &gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok","data":{"message_id":"om_card_1"}}`))
	}))
	defer srv.Close()

	old := testAPIBase
	testAPIBase = func(string) string { return srv.URL }
	defer func() { testAPIBase = old }()

	card := `{"config":{"wide_screen_mode":true},"elements":[]}`
	id, err := SendInteractiveMessage(context.Background(), srv.Client(), RegionFeishu, "tok", "ou_x", ReceiveIDTypeOpenID, card)
	if err != nil {
		t.Fatal(err)
	}
	if id != "om_card_1" {
		t.Fatalf("message_id=%q", id)
	}
	if gotBody["msg_type"] != "interactive" {
		t.Fatalf("msg_type=%q", gotBody["msg_type"])
	}
	if !strings.Contains(gotBody["content"], "wide_screen_mode") {
		t.Fatalf("content=%q", gotBody["content"])
	}
}

func TestUpdateInteractiveMessage_patchesCardJSON(t *testing.T) {
	var method string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		if !strings.Contains(r.URL.Path, "/open-apis/im/v1/messages/om_card_1") {
			t.Fatalf("path=%s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":0,"msg":"ok"}`))
	}))
	defer srv.Close()

	old := testAPIBase
	testAPIBase = func(string) string { return srv.URL }
	defer func() { testAPIBase = old }()

	card := `{"config":{"wide_screen_mode":true},"elements":[{"tag":"div"}]}`
	if err := UpdateInteractiveMessage(context.Background(), srv.Client(), RegionFeishu, "tok", "om_card_1", card); err != nil {
		t.Fatal(err)
	}
	if method != http.MethodPatch {
		t.Fatalf("method=%q", method)
	}
}
