package rss

import "testing"

func TestParseFeedXML(t *testing.T) {
	xml := `<?xml version="1.0"?><rss version="2.0"><channel>
<item><title>Hello</title><link>https://example.com/1</link><description>Body</description></item>
</channel></rss>`
	items, _, err := ParseFeedXML([]byte(xml))
	if err != nil || len(items) != 1 || items[0].Title != "Hello" {
		t.Fatalf("got %#v err=%v", items, err)
	}
}
