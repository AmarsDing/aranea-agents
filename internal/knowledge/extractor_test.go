package knowledge

import (
	"context"
	"strings"
	"testing"
)

func TestTextExtractorSupports(t *testing.T) {
	ex := NewTextExtractor()
	supported := []struct{ ext, mime string }{
		{".txt", "text/plain"},
		{".md", "text/markdown"},
		{".xml", "application/xml"},
		{".yaml", "text/yaml"},
		{".html", "text/html"},
		{".pdf", "application/pdf"},
		{".docx", "application/vnd.openxmlformats-officedocument.wordprocessingml.document"},
		{".xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet"},
		{".pptx", "application/vnd.openxmlformats-officedocument.presentationml.presentation"},
	}
	for _, tt := range supported {
		if !ex.Supports(tt.ext, tt.mime) {
			t.Errorf("Supports(%q, %q) = false, want true", tt.ext, tt.mime)
		}
	}

	// 图片不属于文本提取器（Phase 9 VisionExtractor 接管）。
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		if ex.Supports(ext, "image/png") {
			t.Errorf("Supports(%q, image/png) = true, want false (vision extractor territory)", ext)
		}
	}
	if ex.Supports(".bin", "application/octet-stream") {
		t.Error("Supports(.bin) = true, want false")
	}
}

func TestTextExtractorExtractPlainAndHTML(t *testing.T) {
	ex := NewTextExtractor()
	ctx := context.Background()

	text, err := ex.Extract(ctx, []byte("hello world"), "note.txt", "text/plain")
	if err != nil || text != "hello world" {
		t.Fatalf("txt extract = %q, %v", text, err)
	}

	raw := []byte(`<html><head><style>body{color:#fff}</style><script>var x=1;</script></head><body><h1>Title</h1><p>Body text</p></body></html>`)
	text, err = ex.Extract(ctx, raw, "page.html", "text/html")
	if err != nil {
		t.Fatalf("html extract err: %v", err)
	}
	if !strings.Contains(text, "Title") || !strings.Contains(text, "Body text") {
		t.Fatalf("html extract missing visible text: %q", text)
	}
	if strings.Contains(text, "var x") || strings.Contains(text, "color:#fff") {
		t.Fatalf("html extract leaked script/style: %q", text)
	}
}

func TestTextExtractorRejectsImage(t *testing.T) {
	ex := NewTextExtractor()
	_, err := ex.Extract(context.Background(), []byte("fake-png"), "photo.png", "image/png")
	if err == nil {
		t.Fatal("image extract should fail in Phase 8")
	}
	if !strings.Contains(err.Error(), "multimodal") {
		t.Fatalf("error should mention multimodal, got: %v", err)
	}
}

func TestExtractorRegistryRouting(t *testing.T) {
	reg := NewExtractorRegistry(NewTextExtractor())
	ctx := context.Background()

	text, err := reg.Extract(ctx, []byte("hi"), "a.md", "text/markdown")
	if err != nil || text != "hi" {
		t.Fatalf("registry extract = %q, %v", text, err)
	}

	// 不支持的类型返回明确错误。
	_, err = reg.Extract(ctx, []byte("x"), "a.bin", "application/octet-stream")
	if err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("unsupported type should error, got: %v", err)
	}

	if !reg.Supports("a.pdf", "application/pdf") {
		t.Error("registry Supports(pdf) = false, want true")
	}
	if reg.Supports("a.bin", "application/octet-stream") {
		t.Error("registry Supports(.bin) = true, want false")
	}
	// 图片在 Phase 8 无提取器可用 → Supports false（service 守卫据此给出明确错误）。
	if reg.Supports("a.png", "image/png") {
		t.Error("registry Supports(png) = true, want false in Phase 8")
	}
}

func TestExtractorRegistryNilSafe(t *testing.T) {
	var reg *ExtractorRegistry
	if reg.Supports("a.txt", "text/plain") {
		t.Error("nil registry Supports = true, want false")
	}
	if _, err := reg.Extract(context.Background(), []byte("x"), "a.txt", "text/plain"); err == nil {
		t.Error("nil registry Extract should error")
	}
}
