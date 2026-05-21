package biz

import "testing"

func TestValidateHookConfigForSave_notifyRequiresURL(t *testing.T) {
	err := ValidateHookConfigForSave(`{"callback_point":"after_agent","action":{"type":"notify"}}`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateHookConfigForSave_blocksPrivateWebhook(t *testing.T) {
	err := ValidateHookConfigForSave(`{"callback_point":"after_agent","action":{"type":"notify","webhook_url":"http://127.0.0.1/h"}}`)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestValidateHookConfigForSave_allowsPublicHTTPS(t *testing.T) {
	err := ValidateHookConfigForSave(`{"callback_point":"after_agent","action":{"type":"notify","webhook_url":"https://example.com/hook"}}`)
	if err != nil {
		t.Fatal(err)
	}
}
