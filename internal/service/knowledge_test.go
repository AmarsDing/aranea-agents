package service

import (
	"encoding/base64"
	"net/http"
	"testing"
)

func TestIsAllowedIngestMIME(t *testing.T) {
	tests := []struct {
		mime     string
		expected bool
	}{
		{"text/plain", true},
		{"text/markdown", true},
		{"application/json", true},
		{"application/pdf", true},
		{"image/png", true},
		{"image/jpeg", true},
		{"application/octet-stream", false},
		{"application/x-executable", false},
		{"text/x-custom", true},
	}
	for _, tt := range tests {
		if got := isAllowedIngestMIME(tt.mime); got != tt.expected {
			t.Errorf("isAllowedIngestMIME(%q) = %v, want %v", tt.mime, got, tt.expected)
		}
	}
}

func TestMaxIngestBytesLimit(t *testing.T) {
	small := make([]byte, 100)
	encoded := base64.StdEncoding.EncodeToString(small)
	decoded, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("decode small: %v", err)
	}
	if len(decoded) > maxIngestBytes {
		t.Errorf("small payload should be under limit, got %d bytes", len(decoded))
	}
}

func TestDetectContentTypeText(t *testing.T) {
	raw := []byte("hello world this is plain text")
	detected := http.DetectContentType(raw)
	if detected != "text/plain" {
		t.Errorf("expected text/plain for plain text, got %q", detected)
	}
}

func TestDetectContentTypePDF(t *testing.T) {
	raw := []byte("%PDF-1.4 test content")
	detected := http.DetectContentType(raw)
	if detected != "application/pdf" {
		t.Errorf("expected application/pdf for PDF header, got %q", detected)
	}
}
