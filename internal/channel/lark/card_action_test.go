package lark

import (
	"testing"
)

func TestParseWebhookPost_cardActionTriggerV1Flat(t *testing.T) {
	raw := []byte(`{
		"open_id": "ou_test",
		"open_message_id": "om_x",
		"action": {"value": {"action": "cancel", "session_run_id": "run-2"}, "tag": "button"}
	}`)
	res, err := ParseWebhookPost(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.EventType != "card.action.trigger_v1" {
		t.Fatalf("event_type=%q", res.EventType)
	}
	action, ok := CardActionPayloadFromWebhook(res)
	if !ok {
		t.Fatal("expected card action payload")
	}
	if action.Action != "cancel" || action.SessionRunID != "run-2" {
		t.Fatalf("action=%+v", action)
	}
	if action.OperatorOpenID != "ou_test" || action.OpenMessageID != "om_x" {
		t.Fatalf("context=%+v", action)
	}
}

func TestParseWebhookPost_cardActionTrigger(t *testing.T) {
	raw := []byte(`{
		"schema": "2.0",
		"header": {"event_type": "card.action.trigger"},
		"event": {
			"operator": {"open_id": "ou_test"},
			"action": {"value": {"action": "background", "session_run_id": "run-1"}},
			"context": {"open_chat_id": "oc_x", "open_message_id": "om_x"}
		}
	}`)
	res, err := ParseWebhookPost(raw, "")
	if err != nil {
		t.Fatal(err)
	}
	if res.EventType != "card.action.trigger" {
		t.Fatalf("event_type=%q", res.EventType)
	}
	action, ok := CardActionPayloadFromWebhook(res)
	if !ok {
		t.Fatal("expected card action payload")
	}
	if action.Action != "background" || action.SessionRunID != "run-1" {
		t.Fatalf("action=%+v", action)
	}
	if action.OperatorOpenID != "ou_test" || action.OpenChatID != "oc_x" {
		t.Fatalf("context=%+v", action)
	}
}
