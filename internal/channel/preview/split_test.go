package preview

import (
	"strings"
	"testing"
)

func TestSplitPages_longText(t *testing.T) {
	longBody := strings.Repeat("段落内容。\n\n", 3000)
	pages := SplitPages(longBody, PlatformTextLimit("feishu"))
	if len(pages) < 2 {
		t.Fatalf("expected multiple pages, got %d", len(pages))
	}
	for i, page := range pages {
		if len([]rune(page)) > PlatformTextLimit("feishu") {
			t.Fatalf("page %d exceeds limit: %d runes", i, len([]rune(page)))
		}
	}
}
