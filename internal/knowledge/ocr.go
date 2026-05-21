package knowledge

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// OCRProvider extracts text from image or scanned document bytes.
type OCRProvider interface {
	Extract(ctx context.Context, raw []byte, mimeType, source string) (string, error)
}

type noopOCR struct{}

func (noopOCR) Extract(context.Context, []byte, string, string) (string, error) {
	return "", nil
}

type stubOCR struct{}

func (stubOCR) Extract(_ context.Context, _ []byte, _, _ string) (string, error) {
	return "[ocr: text extraction pending — configure KNOWLEDGE_OCR provider]", nil
}

// NewOCRProviderFromEnv returns the configured OCR backend (default noop).
func NewOCRProviderFromEnv() OCRProvider {
	switch strings.ToLower(strings.TrimSpace(os.Getenv("KNOWLEDGE_OCR"))) {
	case "stub", "placeholder":
		return stubOCR{}
	case "tesseract", "docling":
		// Framework OCR packages require optional deps; stub until wired in deployment.
		return stubOCR{}
	default:
		return noopOCR{}
	}
}

func isImageMime(mimeType string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(mimeType)), "image/")
}

func tryOCR(ctx context.Context, ocr OCRProvider, raw []byte, mimeType, source string) (string, error) {
	if ocr == nil {
		return "", fmt.Errorf("ocr provider not configured")
	}
	text, err := ocr.Extract(ctx, raw, mimeType, source)
	if err != nil {
		return "", err
	}
	text = strings.TrimSpace(text)
	if text == "" {
		return "", fmt.Errorf("ocr returned empty text")
	}
	return text, nil
}
