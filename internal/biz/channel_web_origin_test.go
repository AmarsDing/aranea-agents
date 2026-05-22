package biz

import "testing"

func TestResolveChannelWebOrigin_prefersWebApp(t *testing.T) {
	raw := `{"web_app_origin":"https://admin.test","public_webhook_origin":"https://hook.test"}`
	if got := ResolveChannelWebOrigin(raw); got != "https://admin.test" {
		t.Fatalf("got %q", got)
	}
}

func TestResolveChannelWebOrigin_fallbackWebhook(t *testing.T) {
	raw := `{"public_webhook_origin":"https://hook.test/"}`
	if got := ResolveChannelWebOrigin(raw); got != "https://hook.test" {
		t.Fatalf("got %q", got)
	}
}
