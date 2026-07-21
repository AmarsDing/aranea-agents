package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type mockVisionLLM struct {
	resp   string
	err    error
	images []biz.LLMImage
	model  string
}

func (m *mockVisionLLM) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	m.images = req.Images
	m.model = req.Model
	if m.err != nil {
		return "", 0, m.err
	}
	return m.resp, 0, nil
}

func newVisionTestExtractor(llm biz.LLMCaller) *VisionExtractor {
	return NewVisionExtractor(llm, nil, stubCatalogLister{models: []biz.ProviderModel{
		{Provider: "openai", Model: "gpt-4o", Enabled: true, Capabilities: biz.ModelCapabilities{Vision: true}},
	}}, loggateway.NewNoop())
}

func TestVisionExtractorSupports(t *testing.T) {
	v := newVisionTestExtractor(&mockVisionLLM{resp: "# 图"})
	for _, ext := range []string{".png", ".jpg", ".jpeg", ".webp"} {
		if !v.Supports(ext, "") {
			t.Errorf("Supports(%q) = false, want true", ext)
		}
	}
	if !v.Supports("", "image/png") {
		t.Error("Supports(\"\", image/png) = false, want true")
	}
	for _, ext := range []string{".txt", ".pdf", ".md"} {
		if v.Supports(ext, "") {
			t.Errorf("Supports(%q) = true, want false (text extractor territory)", ext)
		}
	}
}

func TestVisionExtractorExtractReturnsMarkdown(t *testing.T) {
	llm := &mockVisionLLM{resp: "```markdown\n# 图片内容\n图中文字：你好\n```"}
	v := newVisionTestExtractor(llm)
	md, err := v.Extract(context.Background(), []byte{0x89, 0x50}, "photo.png", "image/png")
	if err != nil {
		t.Fatalf("Extract error: %v", err)
	}
	if !strings.Contains(md, "# 图片内容") {
		t.Errorf("md = %q, want organized markdown", md)
	}
	if strings.Contains(md, "```") {
		t.Errorf("code fence not stripped: %q", md)
	}
	if len(llm.images) != 1 || string(llm.images[0].Data) != string([]byte{0x89, 0x50}) {
		t.Errorf("image bytes not passed to LLM: %+v", llm.images)
	}
	if llm.images[0].Format != "png" {
		t.Errorf("image format = %q, want png", llm.images[0].Format)
	}
	if llm.model != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", llm.model)
	}
}

func TestVisionExtractorNoLLM(t *testing.T) {
	v := NewVisionExtractor(nil, nil, nil, loggateway.NewNoop())
	_, err := v.Extract(context.Background(), []byte{1}, "a.png", "image/png")
	if err == nil {
		t.Fatal("expected explicit error when LLM unavailable (NFR-12)")
	}
}

func TestVisionExtractorNoVisionModel(t *testing.T) {
	v := NewVisionExtractor(&mockVisionLLM{resp: "x"}, nil, stubCatalogLister{}, loggateway.NewNoop())
	_, err := v.Extract(context.Background(), []byte{1}, "a.png", "image/png")
	if err == nil {
		t.Fatal("expected explicit error when no vision model configured (NFR-12)")
	}
}

func TestVisionExtractorLLMFailure(t *testing.T) {
	v := newVisionTestExtractor(&mockVisionLLM{err: errors.New("provider down")})
	_, err := v.Extract(context.Background(), []byte{1}, "a.png", "image/png")
	if err == nil {
		t.Fatal("expected error to propagate (doc lands status=error)")
	}
}

func TestVisionExtractorEmptyResponse(t *testing.T) {
	v := newVisionTestExtractor(&mockVisionLLM{resp: "  "})
	_, err := v.Extract(context.Background(), []byte{1}, "a.png", "image/png")
	if err == nil {
		t.Fatal("expected error on empty vision response")
	}
}
