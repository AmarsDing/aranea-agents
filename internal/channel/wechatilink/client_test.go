package wechatilink

import (
	"strings"
	"testing"
)

func TestBuildRequestHeaders(t *testing.T) {
	h := buildRequestHeaders("my_token")
	if h.Get("iLink-App-Id") != "bot" {
		t.Errorf("app-id want bot, got %s", h.Get("iLink-App-Id"))
	}
	if auth := h.Get("Authorization"); !strings.HasPrefix(auth, "Bearer my_token") {
		t.Errorf("auth want Bearer my_token, got %s", auth)
	}
	if h.Get("AuthorizationType") != "ilink_bot_token" {
		t.Errorf("authorization-type wrong: %s", h.Get("AuthorizationType"))
	}
	if uin := h.Get("X-WECHAT-UIN"); uin == "" {
		t.Error("X-WECHAT-UIN should not be empty")
	}
	if h.Get("iLink-App-ClientVersion") == "" {
		t.Error("iLink-App-ClientVersion should not be empty")
	}
}

func TestRandomUINUnique(t *testing.T) {
	a := randomUIN()
	b := randomUIN()
	if a == "" || b == "" {
		t.Fatal("UIN should not be empty")
	}
	if a == b {
		t.Error("two consecutive UINs should differ (anti-replay)")
	}
}

func TestNewClientBaseURL(t *testing.T) {
	c := newClient("", "tk", nil)
	if c.baseURL != defaultBaseURL {
		t.Errorf("default baseURL want %s, got %s", defaultBaseURL, c.baseURL)
	}
	c2 := newClient("https://custom.example.com/", "tk", nil)
	if c2.baseURL != "https://custom.example.com" {
		t.Errorf("trailing slash should be trimmed, got %s", c2.baseURL)
	}
}
