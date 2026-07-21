package service

import (
	"encoding/base64"
	"net/http"
	"strings"
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
		{"image/webp", true},
		{"image/gif", false},
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
	if !strings.HasPrefix(detected, "text/plain") {
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

// OOXML（DOCX/XLSX/PPTX）是 ZIP 容器，http.DetectContentType 返回 application/zip，
// 必须按声明 MIME / 扩展名二次判定，否则 Office 文件被白名单误拒。
func TestResolveIngestMIME(t *testing.T) {
	tests := []struct {
		name     string
		detected string
		declared string
		source   string
		allowed  bool
	}{
		{"docx by declared mime", "application/zip", "application/vnd.openxmlformats-officedocument.wordprocessingml.document", "a.docx", true},
		{"xlsx by declared mime", "application/zip", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", "a.xlsx", true},
		{"pptx by declared mime", "application/zip", "application/vnd.openxmlformats-officedocument.presentationml.presentation", "a.pptx", true},
		{"docx by extension fallback", "application/zip", "", "report.docx", true},
		{"xlsx by extension fallback", "application/zip", "application/octet-stream", "data.xlsx", true},
		{"plain zip rejected", "application/zip", "", "archive.zip", false},
		{"zip with wrong ext rejected", "application/zip", "", "archive.rar", false},
		{"pdf passthrough", "application/pdf", "", "doc.pdf", true},
		{"text passthrough", "text/plain; charset=utf-8", "", "note.txt", true},
		{"image allowed", "image/png", "image/png", "photo.png", true},
		{"exe rejected", "application/x-executable", "", "run.exe", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := resolveIngestMIMEAllowed(tt.detected, tt.declared, tt.source); got != tt.allowed {
				t.Errorf("resolveIngestMIMEAllowed(%q, %q, %q) = %v, want %v",
					tt.detected, tt.declared, tt.source, got, tt.allowed)
			}
		})
	}
}

// Phase 9：图片模态判定 —— 图片走 VisionExtractor，跳过 MarkdownOrganizer。
func TestIsImageIngest(t *testing.T) {
	tests := []struct {
		source string
		mime   string
		want   bool
	}{
		{"photo.png", "image/png", true},
		{"photo.jpg", "", true},
		{"photo.jpeg", "", true},
		{"photo.webp", "", true},
		{"PHOTO.PNG", "", true},
		{"note.md", "text/markdown", false},
		{"doc.pdf", "application/pdf", false},
		{"archive.bin", "image/png", true}, // 声明 MIME 优先
	}
	for _, tt := range tests {
		if got := isImageIngest(tt.source, tt.mime); got != tt.want {
			t.Errorf("isImageIngest(%q, %q) = %v, want %v", tt.source, tt.mime, got, tt.want)
		}
	}
}

// Phase 9：metadata 合并 —— 用户 metadata 与系统模态标记合并为一个 JSON 对象。
func TestMergeIngestMetadata(t *testing.T) {
	got, err := mergeIngestMetadata(`{"team":"quant"}`, "image", "vision")
	if err != nil {
		t.Fatalf("mergeIngestMetadata error: %v", err)
	}
	for _, want := range []string{`"team":"quant"`, `"modality":"image"`, `"extractor":"vision"`} {
		if !strings.Contains(got, want) {
			t.Errorf("merged metadata %q missing %s", got, want)
		}
	}

	// 空用户 metadata。
	got, err = mergeIngestMetadata("", "text", "text")
	if err != nil {
		t.Fatalf("mergeIngestMetadata(empty) error: %v", err)
	}
	if !strings.Contains(got, `"modality":"text"`) {
		t.Errorf("merged metadata %q missing modality", got)
	}

	// 非法用户 metadata 报错（不静默吞掉）。
	if _, err = mergeIngestMetadata("{bad", "text", "text"); err == nil {
		t.Error("expected error for invalid user metadata")
	}
}
