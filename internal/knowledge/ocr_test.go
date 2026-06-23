package knowledge

import (
	"context"
	"strings"
	"testing"
)

type fixedOCR struct{ text string }

func (f fixedOCR) Extract(context.Context, []byte, string, string) (string, error) {
	return f.text, nil
}

func TestExtractDocumentTextWithOCRImage(t *testing.T) {
	raw := []byte{0xff, 0xd8, 0xff}
	text, err := ExtractDocumentTextWithOCR(context.Background(), fixedOCR{text: "hello ocr"}, raw, "scan.jpg", "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, "hello ocr") {
		t.Fatalf("unexpected text: %q", text)
	}
}

func TestNewOCRProviderFromEnvStub(t *testing.T) {
	t.Setenv("KNOWLEDGE_OCR", "stub")
	p := NewOCRProviderFromEnv()
	_, err := p.Extract(context.Background(), nil, "image/png", "x.png")
	if err == nil {
		t.Fatal("stub ocr should return error, not placeholder text")
	}
}
