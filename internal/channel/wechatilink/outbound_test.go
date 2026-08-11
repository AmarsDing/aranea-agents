package wechatilink

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/ilink/bot/sendmessage" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer tk" {
			t.Errorf("auth header wrong: %s", r.Header.Get("Authorization"))
		}
		var body sendMessageReq
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatal(err)
		}
		if body.Msg.ContextToken != "ctx-1" {
			t.Errorf("context_token want ctx-1, got %s", body.Msg.ContextToken)
		}
		if body.Msg.ToUserID != "user@im.wechat" {
			t.Errorf("to_user_id wrong: %s", body.Msg.ToUserID)
		}
		if body.Msg.MessageType != MessageTypeBot || body.Msg.MessageState != MessageStateFinish {
			t.Errorf("message type/state wrong: %d/%d", body.Msg.MessageType, body.Msg.MessageState)
		}
		if len(body.Msg.ItemList) != 1 || body.Msg.ItemList[0].TextItem.Text != "hi" {
			t.Errorf("item list wrong: %+v", body.Msg.ItemList)
		}
		if body.BaseInfo.ChannelVersion != channelVersion {
			t.Errorf("channel_version wrong: %s", body.BaseInfo.ChannelVersion)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0})
	}))
	defer server.Close()

	sender := &TextSender{BotToken: "tk", BaseURL: server.URL, ContextToken: "ctx-1"}
	if err := sender.SendText(context.Background(), "user@im.wechat", "hi"); err != nil {
		t.Fatal(err)
	}
}

func TestSendMessageSessionExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"ret": 0, "errcode": -14, "errmsg": "session expired"})
	}))
	defer server.Close()

	sender := &TextSender{BotToken: "tk", BaseURL: server.URL}
	err := sender.SendText(context.Background(), "u", "hi")
	if err != ErrSessionExpired {
		t.Errorf("want ErrSessionExpired, got %v", err)
	}
}

func TestSendTextNoToken(t *testing.T) {
	sender := &TextSender{}
	if err := sender.SendText(context.Background(), "u", "hi"); err == nil {
		t.Error("missing bot_token should error")
	}
}
