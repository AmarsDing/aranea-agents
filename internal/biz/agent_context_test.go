package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/auth"
)

func TestResolveListCreatedByFilter(t *testing.T) {
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 42})

	tests := []struct {
		name   string
		ctx    context.Context
		filter string
		want   string
	}{
		{"empty", ctx, "", ""},
		{"all", ctx, "all", ""},
		{"mine", ctx, "mine", "42"},
		{"mine uppercase", ctx, "MINE", "42"},
		{"user id", ctx, "99", "99"},
		{"no auth mine", context.Background(), "mine", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ResolveListCreatedByFilter(tt.ctx, tt.filter)
			if got != tt.want {
				t.Fatalf("ResolveListCreatedByFilter(%q) = %q, want %q", tt.filter, got, tt.want)
			}
		})
	}
}
