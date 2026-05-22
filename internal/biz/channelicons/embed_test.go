package channelicons_test

import (
	"testing"

	"aranea-agents/internal/biz/channelicons"
)

func TestLoadPNGEmbeddedFeishu(t *testing.T) {
	data, err := channelicons.LoadPNG("channel_feishu")
	if err != nil {
		t.Fatal(err)
	}
	if len(data) < 64 {
		t.Fatalf("png too small: %d", len(data))
	}
	if data[0] != 0x89 || data[1] != 'P' {
		t.Fatal("not a PNG")
	}
}
