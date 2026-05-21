package knowledge

import (
	"strings"
	"testing"
)

func TestParseChunkStrategy(t *testing.T) {
	t.Parallel()
	if ParseChunkStrategy("") != ChunkByChar {
		t.Fatal("empty -> char")
	}
	if ParseChunkStrategy("markdown") != ChunkByMarkdown {
		t.Fatal("markdown")
	}
	if ParseChunkStrategy("UNKNOWN") != ChunkByChar {
		t.Fatal("unknown -> char")
	}
}

func TestExtractDocumentText_plain(t *testing.T) {
	t.Parallel()
	text, err := ExtractDocumentText([]byte("hello"), "note.txt", "text/plain")
	if err != nil || text != "hello" {
		t.Fatalf("got %q err %v", text, err)
	}
}

func TestExtractDocumentText_html(t *testing.T) {
	t.Parallel()
	raw := []byte("<html><body><p>Hi</p></body></html>")
	text, err := ExtractDocumentText(raw, "page.html", "text/html")
	if err != nil || !strings.Contains(text, "Hi") {
		t.Fatalf("got %q err %v", text, err)
	}
}
