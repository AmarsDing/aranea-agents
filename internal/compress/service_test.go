package compress

import (
	"context"
	"net/http"
	"testing"

	"aranea-agents/internal/biz"
)

func TestLLMService_Compress_nilCatalog(t *testing.T) {
	s := NewLLMService(nil, &http.Client{})
	_, err := s.Compress(context.Background(), Request{Transcript: "x", Provider: "openai", Model: "gpt-4o-mini"})
	if err != ErrCatalogRequired {
		t.Fatalf("got %v want ErrCatalogRequired", err)
	}
}

func TestLLMService_Compress_nilHTTP(t *testing.T) {
	s := NewLLMService(&biz.LlmProviderModelUsecase{}, nil)
	_, err := s.Compress(context.Background(), Request{Transcript: "x", Provider: "openai", Model: "gpt-4o-mini"})
	if err != ErrHTTPClientRequired {
		t.Fatalf("got %v want ErrHTTPClientRequired", err)
	}
}
