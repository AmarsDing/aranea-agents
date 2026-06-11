package channelicons_test

import (
	"testing"

	"aranea-agents/internal/biz/channelicons"
	"aranea-agents/pkg/apierror"
)

func isAPIErrorCode(err error, code apierror.Code) bool {
	ae, ok := apierror.From(err)
	return ok && ae.Code == code
}

func TestLoadPNG_EmptyKey(t *testing.T) {
	_, err := channelicons.LoadPNG("")
	if err == nil {
		t.Fatal("expected error for empty key, got nil")
	}
	if !isAPIErrorCode(err, apierror.CodeBadRequest) {
		t.Fatalf("expected BadRequest error, got %v", err)
	}
}

func TestLoadPNG_NonexistentKey(t *testing.T) {
	_, err := channelicons.LoadPNG("nonexistent_icon")
	if err == nil {
		t.Fatal("expected error for nonexistent key, got nil")
	}
}

func TestLoadPNG_MultipleValidKeys(t *testing.T) {
	keys := []string{
		"channel_feishu",
		"channel_dingtalk",
		"channel_wecom",
		"channel_telegram",
		"channel_slack",
		"channel_discord",
		"channel_whatsapp",
	}
	for _, key := range keys {
		t.Run(key, func(t *testing.T) {
			data, err := channelicons.LoadPNG(key)
			if err != nil {
				t.Fatalf("LoadPNG(%q) error: %v", key, err)
			}
			if len(data) < 64 {
				t.Fatalf("LoadPNG(%q): png too small: %d bytes", key, len(data))
			}
			if data[0] != 0x89 || data[1] != 'P' {
				t.Fatalf("LoadPNG(%q): not a PNG (header: %x %x)", key, data[0], data[1])
			}
		})
	}
}
