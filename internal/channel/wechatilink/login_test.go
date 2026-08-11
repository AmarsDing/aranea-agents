package wechatilink

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestLoginFlow(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/ilink/bot/get_bot_qrcode":
			if r.URL.Query().Get("bot_type") != "3" {
				t.Errorf("bot_type want 3, got %s", r.URL.Query().Get("bot_type"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ret": 0, "qrcode": "sess-1", "qrcode_img_content": "data:image/png;base64,abc",
			})
		case "/ilink/bot/get_qrcode_status":
			if r.URL.Query().Get("qrcode") != "sess-1" {
				t.Errorf("qrcode want sess-1, got %s", r.URL.Query().Get("qrcode"))
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ret": 0, "status": "confirmed", "bot_token": "tk123",
				"baseurl": "https://t.example.com", "ilink_user_id": "uid1",
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
	}))
	defer server.Close()

	lc := NewLoginClient(server.URL, nil)
	qr, err := lc.GetBotQRCode(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if qr.QRCode != "sess-1" || qr.QRCodeImgContent == "" {
		t.Errorf("unexpected qr resp: %+v", qr)
	}

	status, err := lc.GetQRCodeStatus(context.Background(), "sess-1")
	if err != nil {
		t.Fatal(err)
	}
	if status.Status != QRStatusConfirmed || status.BotToken != "tk123" {
		t.Errorf("unexpected status: %+v", status)
	}
	if status.BaseURL != "https://t.example.com" || status.ILinkUserID != "uid1" {
		t.Errorf("unexpected login payload: %+v", status)
	}
}

func TestLoginAPIError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ret": -1, "errcode": -2, "errmsg": "bad param"})
	}))
	defer server.Close()

	lc := NewLoginClient(server.URL, nil)
	if _, err := lc.GetBotQRCode(context.Background()); err == nil {
		t.Fatal("expected error on ret=-1")
	}
}
