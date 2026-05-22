package lark

import (
	"testing"

	larkim "github.com/larksuite/oapi-sdk-go/v3/service/im/v1"
)

func strPtr(s string) *string { return &s }

func TestInboundEventFromWSMessage_userText(t *testing.T) {
	textType := "text"
	user := "user"
	content := `{"text":"你好"}`
	msgID := "om_test_1"
	openID := "ou_user"
	ev, ok := InboundEventFromWSMessage(&larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{
				SenderType: &user,
				SenderId:   &larkim.UserId{OpenId: &openID},
			},
			Message: &larkim.EventMessage{
				MessageType: &textType,
				MessageId:   &msgID,
				Content:     &content,
				ChatType:    strPtr("p2p"),
			},
		},
	})
	if !ok || ev.IdempotencyKey != "feishu:"+msgID || ev.Text != "你好" {
		t.Fatalf("unexpected event: ok=%v ev=%+v", ok, ev)
	}
}

func TestInboundEventFromWSMessage_rejectsAppSender(t *testing.T) {
	textType := "text"
	app := "app"
	content := `{"text":"你好"}`
	_, ok := InboundEventFromWSMessage(&larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{SenderType: &app},
			Message: &larkim.EventMessage{
				MessageType: &textType,
				MessageId:   strPtr("om_bot"),
				Content:     &content,
			},
		},
	})
	if ok {
		t.Fatal("expected bot sender rejected")
	}
}

func TestInboundEventFromWSMessage_rejectsMissingSenderType(t *testing.T) {
	textType := "text"
	content := `{"text":"你好"}`
	_, ok := InboundEventFromWSMessage(&larkim.P2MessageReceiveV1{
		Event: &larkim.P2MessageReceiveV1Data{
			Sender: &larkim.EventSender{},
			Message: &larkim.EventMessage{
				MessageType: &textType,
				MessageId:   strPtr("om_x"),
				Content:     &content,
			},
		},
	})
	if ok {
		t.Fatal("expected missing sender_type rejected")
	}
}
