package knowledge

import (
	"bytes"
	"strings"

	"golang.org/x/net/html"
)

// stripHTML extracts visible text from HTML (no script/style content).
func stripHTML(raw string) string {
	root, err := html.Parse(strings.NewReader(raw))
	if err != nil {
		return strings.TrimSpace(raw)
	}
	var buf bytes.Buffer
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.TextNode {
			text := strings.TrimSpace(n.Data)
			if text != "" {
				if buf.Len() > 0 {
					buf.WriteByte(' ')
				}
				buf.WriteString(text)
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			if c.Type == html.ElementNode && (c.Data == "script" || c.Data == "style") {
				continue
			}
			walk(c)
		}
	}
	walk(root)
	return strings.TrimSpace(buf.String())
}
