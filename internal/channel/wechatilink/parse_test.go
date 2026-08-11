package wechatilink

import (
	"testing"

	"aranea-agents/internal/channel/port"
)

func TestParseTextMessage(t *testing.T) {
	msg := WeixinMessage{
		FromUserID:   "user@im.wechat",
		MessageID:    100,
		MessageType:  MessageTypeUser,
		MessageState: MessageStateFinish,
		ItemList:     []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "hello"}}},
		ContextToken: "ctx-1",
		SessionID:    "sess-1",
	}
	ev, err := parseMessage("ch-1", &msg)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Text != "hello" {
		t.Errorf("text want hello, got %s", ev.Text)
	}
	if ev.PeerID != "user@im.wechat" {
		t.Errorf("peer want user@im.wechat, got %s", ev.PeerID)
	}
	if ev.PlatformType != "wechat_ilink" {
		t.Errorf("platform want wechat_ilink, got %s", ev.PlatformType)
	}
	if ev.OutboundMeta[port.MetaContextToken] != "ctx-1" {
		t.Error("context_token not propagated to OutboundMeta")
	}
	if ev.OutboundMeta[port.MetaRecipient] != "user@im.wechat" {
		t.Errorf("recipient wrong: %s", ev.OutboundMeta[port.MetaRecipient])
	}
	if ev.IdempotencyKey != "wechat_ilink:ch-1:100" {
		t.Errorf("idempotency key wrong: %s", ev.IdempotencyKey)
	}
}

func TestParseGroupMessageRecipient(t *testing.T) {
	msg := WeixinMessage{
		FromUserID: "user@im.wechat",
		GroupID:    "group@im.chatroom",
		MessageID:  200,
		ItemList:   []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "hi group"}}},
	}
	ev, err := parseMessage("ch-1", &msg)
	if err != nil {
		t.Fatal(err)
	}
	if ev.OutboundMeta[port.MetaRecipient] != "group@im.chatroom" {
		t.Errorf("group recipient want group id, got %s", ev.OutboundMeta[port.MetaRecipient])
	}
	if ev.PeerID != "user@im.wechat" {
		t.Errorf("peer should remain sender, got %s", ev.PeerID)
	}
}

func TestParseVoiceWithTranscription(t *testing.T) {
	msg := WeixinMessage{
		FromUserID: "u1",
		MessageID:  300,
		ItemList:   []MessageItem{{Type: ItemTypeVoice, VoiceItem: &VoiceItem{Text: "语音转文字结果"}}},
	}
	ev, err := parseMessage("ch-1", &msg)
	if err != nil {
		t.Fatal(err)
	}
	if ev.Text != "语音转文字结果" {
		t.Errorf("voice should use server transcription, got %s", ev.Text)
	}
}

func TestParseMediaPlaceholders(t *testing.T) {
	cases := []struct {
		name string
		item MessageItem
		want string
	}{
		{"image", MessageItem{Type: ItemTypeImage, ImageItem: &ImageItem{}}, "[图片]"},
		{"voice-no-text", MessageItem{Type: ItemTypeVoice, VoiceItem: &VoiceItem{}}, "[语音消息，未识别]"},
		{"file", MessageItem{Type: ItemTypeFile, FileItem: &FileItem{FileName: "a.pdf"}}, "[文件: a.pdf]"},
		{"video", MessageItem{Type: ItemTypeVideo, VideoItem: &VideoItem{}}, "[视频]"},
		{"unknown", MessageItem{Type: 99}, "[未知消息]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			msg := WeixinMessage{FromUserID: "u1", MessageID: 1, ItemList: []MessageItem{tc.item}}
			ev, err := parseMessage("ch-1", &msg)
			if err != nil {
				t.Fatal(err)
			}
			if ev.Text != tc.want {
				t.Errorf("want %q, got %q", tc.want, ev.Text)
			}
		})
	}
}

func TestParseEmptyItemList(t *testing.T) {
	msg := WeixinMessage{FromUserID: "u1", MessageID: 1}
	if _, err := parseMessage("ch-1", &msg); err == nil {
		t.Error("empty item list should error")
	}
}

func TestGroupGating(t *testing.T) {
	groupMsg := func(text string) *WeixinMessage {
		return &WeixinMessage{
			FromUserID: "u1", GroupID: "g1", MessageID: 1,
			ItemList: []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: text}}},
		}
	}
	boolPtr := func(b bool) *bool { return &b }

	t.Run("private passthrough", func(t *testing.T) {
		msg := &WeixinMessage{FromUserID: "u1", MessageID: 1,
			ItemList: []MessageItem{{Type: ItemTypeText, TextItem: &TextItem{Text: "hi"}}}}
		if !shouldHandleGroupMessage(msg, instanceConfig{}) {
			t.Error("private message should pass")
		}
	})
	t.Run("group disabled by default", func(t *testing.T) {
		if shouldHandleGroupMessage(groupMsg("hello"), instanceConfig{}) {
			t.Error("group message should be dropped when group_enabled unset")
		}
	})
	t.Run("group enabled, mention required by default", func(t *testing.T) {
		cfg := instanceConfig{}
		cfg.Config.GroupEnabled = boolPtr(true)
		cfg.Config.BotNickname = "小助手"
		if shouldHandleGroupMessage(groupMsg("hello"), cfg) {
			t.Error("should drop group message without mention")
		}
		if !shouldHandleGroupMessage(groupMsg("@小助手 在吗"), cfg) {
			t.Error("should pass group message with mention")
		}
	})
	t.Run("mention not required", func(t *testing.T) {
		cfg := instanceConfig{}
		cfg.Config.GroupEnabled = boolPtr(true)
		cfg.Config.RequireMention = boolPtr(false)
		if !shouldHandleGroupMessage(groupMsg("hello"), cfg) {
			t.Error("should pass when require_mention=false")
		}
	})
	t.Run("no nickname configured passes", func(t *testing.T) {
		cfg := instanceConfig{}
		cfg.Config.GroupEnabled = boolPtr(true)
		if !shouldHandleGroupMessage(groupMsg("hello"), cfg) {
			t.Error("should pass when bot_nickname empty")
		}
	})
}
