package tool

import (
	"aranea-agents/internal/biz/shared"
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

type stubToolRepo struct {
	ToolRepo
	getTool func(ctx context.Context, idOrKey string) (Tool, error)
}

func (s *stubToolRepo) GetTool(ctx context.Context, idOrKey string) (Tool, error) {
	if s.getTool != nil {
		return s.getTool(ctx, idOrKey)
	}
	return Tool{}, shared.ErrNotFound
}

func TestToolUsecase_ResolveToolKey(t *testing.T) {
	uc := NewToolUsecase(&stubToolRepo{
		getTool: func(_ context.Context, idOrKey string) (Tool, error) {
			if idOrKey == "tool_duckduckgo_search" {
				return Tool{ID: "tool_duckduckgo_search", Key: "duckduckgo_search"}, nil
			}
			if idOrKey == "duckduckgo_search" {
				return Tool{ID: "tool_duckduckgo_search", Key: "duckduckgo_search"}, nil
			}
			return Tool{}, shared.ErrNotFound
		},
	}, nil, loggateway.NewNoop())

	key, err := uc.ResolveToolKey(context.Background(), "tool_duckduckgo_search")
	if err != nil || key != "duckduckgo_search" {
		t.Fatalf("resolve by id: key=%q err=%v", key, err)
	}
	key, err = uc.ResolveToolKey(context.Background(), "duckduckgo_search")
	if err != nil || key != "duckduckgo_search" {
		t.Fatalf("resolve by key: key=%q err=%v", key, err)
	}
	if _, err := uc.ResolveToolKey(context.Background(), "missing"); err == nil {
		t.Fatal("expected error for missing tool")
	}
}
