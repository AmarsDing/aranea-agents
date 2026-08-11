package service

import (
	"context"
	"testing"

	v1 "aranea-agents/api/kratos/llm_provider_model/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

// stubLlmProviderModelReader implements biz.LlmProviderModelReader for List tests.
type stubLlmProviderModelReader struct {
	listItems   []biz.ProviderModel
	searchQuery biz.ProviderModelListQuery
	searchCalls int
	listCalls   int
}

func (s *stubLlmProviderModelReader) ListProviderModels(context.Context) ([]biz.ProviderModel, error) {
	s.listCalls++
	return s.listItems, nil
}

func (s *stubLlmProviderModelReader) SearchProviderModels(_ context.Context, q biz.ProviderModelListQuery) (biz.ProviderModelListResult, error) {
	s.searchCalls++
	s.searchQuery = q
	return biz.ProviderModelListResult{Items: s.listItems, Total: 42, Limit: q.Limit, Offset: q.Offset}, nil
}

func (s *stubLlmProviderModelReader) GetProviderModel(context.Context, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, nil
}

func (s *stubLlmProviderModelReader) GetProviderModelByProviderAndModel(context.Context, string, string) (biz.ProviderModel, error) {
	return biz.ProviderModel{}, nil
}

func newListTestService(reader biz.LlmProviderModelReader) *LlmProviderModelService {
	uc := biz.NewLlmProviderModelUsecase(reader, nil, nil, nil, nil, nil, nil, nil, loggateway.NewNoop())
	return NewLlmProviderModelService(uc, nil, loggateway.NewNoop())
}

// Regression: ListProviderModels previously took google.protobuf.Empty, so the
// generated TS client silently dropped page/page_size/search and the admin
// registry UI search/pagination never reached the server.
func TestListProviderModels_PagedRequestPassesQueryToUsecase(t *testing.T) {
	reader := &stubLlmProviderModelReader{
		listItems: []biz.ProviderModel{{ID: "m1", Provider: "openai", Model: "gpt-4o"}},
	}
	svc := newListTestService(reader)

	resp, err := svc.ListProviderModels(context.Background(), &v1.ListProviderModelsRequest{
		Page: 2, PageSize: 10, Search: " gpt ",
	})
	if err != nil {
		t.Fatalf("ListProviderModels: %v", err)
	}
	if reader.searchCalls != 1 {
		t.Fatalf("expected SearchProviderModels called once, got %d", reader.searchCalls)
	}
	if reader.searchQuery.Search != "gpt" {
		t.Fatalf("search: got %q, want %q (trimmed)", reader.searchQuery.Search, "gpt")
	}
	if reader.searchQuery.Limit != 10 || reader.searchQuery.Offset != 10 {
		t.Fatalf("limit/offset: got %d/%d, want 10/10", reader.searchQuery.Limit, reader.searchQuery.Offset)
	}
	if resp.GetTotal() != 42 || resp.GetPage() != 2 || resp.GetPageSize() != 10 {
		t.Fatalf("resp page fields: total=%d page=%d pageSize=%d", resp.GetTotal(), resp.GetPage(), resp.GetPageSize())
	}
	if len(resp.GetItems()) != 1 || resp.GetItems()[0].GetId() != "m1" {
		t.Fatalf("items mismatch: %+v", resp.GetItems())
	}
}

// Pickers/health callers pass no pagination: keep the legacy full-list path.
func TestListProviderModels_EmptyRequestFallsBackToFullList(t *testing.T) {
	reader := &stubLlmProviderModelReader{listItems: []biz.ProviderModel{{ID: "m1"}, {ID: "m2"}}}
	svc := newListTestService(reader)

	resp, err := svc.ListProviderModels(context.Background(), &v1.ListProviderModelsRequest{})
	if err != nil {
		t.Fatalf("ListProviderModels: %v", err)
	}
	if reader.listCalls != 1 {
		t.Fatalf("expected legacy List path once, got %d", reader.listCalls)
	}
	if reader.searchCalls != 0 {
		t.Fatalf("SearchProviderModels should not be called, got %d", reader.searchCalls)
	}
	if resp.GetTotal() != 2 {
		t.Fatalf("total: got %d, want 2", resp.GetTotal())
	}
}
