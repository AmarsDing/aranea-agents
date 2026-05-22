package lark

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendTextMessageUserID(t *testing.T) {
	var gotType string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotType = r.URL.Query().Get("receive_id_type")
		_, _ = w.Write([]byte(`{"code":0}`))
	}))
	defer srv.Close()

	transport := &rewriteTransport{base: http.DefaultTransport, host: srv.URL}
	client := &http.Client{Transport: transport}
	err := SendTextMessage(context.Background(), client, RegionFeishu, "tok", "u_abc", ReceiveIDTypeUserID, "hi")
	if err != nil {
		t.Fatal(err)
	}
	if gotType != ReceiveIDTypeUserID {
		t.Fatalf("receive_id_type=%q", gotType)
	}
}

func TestFeishuTextSenderEffectiveReceiveIDType(t *testing.T) {
	s := &FeishuTextSender{ReceiveIDType: ReceiveIDTypeChatID}
	if s.effectiveReceiveIDType() != ReceiveIDTypeChatID {
		t.Fatalf("got %q", s.effectiveReceiveIDType())
	}
}
