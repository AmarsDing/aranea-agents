package service

import (
	"context"
	"testing"
	"time"

	airefinev1 "aranea-agents/api/kratos/ai_refine/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/pkg/auth"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestUserIDFromCtx(t *testing.T) {
	t.Run("with_auth", func(t *testing.T) {
		a := &auth.Auth{UserID: 42}
		ctx := auth.NewContext(context.Background(), a)
		got := userIDFromCtx(ctx)
		if got != "42" {
			t.Errorf("userIDFromCtx = %q, want %q", got, "42")
		}
	})

	t.Run("without_auth", func(t *testing.T) {
		got := userIDFromCtx(context.Background())
		if got != "" {
			t.Errorf("userIDFromCtx = %q, want empty", got)
		}
	})
}

func TestScopeMap(t *testing.T) {
	tests := []struct {
		name  string
		scope airefinev1.RefineScope
		want  biz.FieldScope
	}{
		{"category_industry", airefinev1.RefineScope_REFINE_SCOPE_CATEGORY_INDUSTRY, biz.ScopeCategoryIndustry},
		{"category_dept", airefinev1.RefineScope_REFINE_SCOPE_CATEGORY_DEPT, biz.ScopeCategoryDepartment},
		{"category_position", airefinev1.RefineScope_REFINE_SCOPE_CATEGORY_POSITION, biz.ScopeCategoryPosition},
		{"agent_description", airefinev1.RefineScope_REFINE_SCOPE_AGENT_DESCRIPTION, biz.ScopeAgentDescription},
		{"agent_file", airefinev1.RefineScope_REFINE_SCOPE_AGENT_FILE, biz.ScopeAgentFile},
		{"spec_extract", airefinev1.RefineScope_REFINE_SCOPE_SPEC_EXTRACT, biz.ScopeSpecExtract},
		{"unknown", airefinev1.RefineScope(999), biz.FieldScope("")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := scopeMap[tt.scope]
			if got != tt.want {
				t.Errorf("scopeMap[%v] = %q, want %q", tt.scope, got, tt.want)
			}
		})
	}
}

func TestRefineRateLimiter_GlobalBurst(t *testing.T) {
	rl := newRefineRateLimiter(3, 5*time.Minute, 10)
	ctx := context.Background()

	for i := 0; i < 3; i++ {
		if err := rl.allow(ctx, ""); err != nil {
			t.Fatalf("allow %d should succeed, got %v", i, err)
		}
	}
	if err := rl.allow(ctx, ""); err == nil {
		t.Fatal("4th request should exceed global burst")
	}
}

func TestRefineRateLimiter_PerUserLimit(t *testing.T) {
	rl := newRefineRateLimiter(100, 5*time.Minute, 2)
	ctx := context.Background()

	if err := rl.allow(ctx, "user-1"); err != nil {
		t.Fatalf("first allow: %v", err)
	}
	if err := rl.allow(ctx, "user-1"); err != nil {
		t.Fatalf("second allow: %v", err)
	}
	if err := rl.allow(ctx, "user-1"); err == nil {
		t.Fatal("3rd request for same user should exceed per-user limit")
	}
}

func TestRefineRateLimiter_DifferentUsers(t *testing.T) {
	rl := newRefineRateLimiter(100, 5*time.Minute, 1)
	ctx := context.Background()

	if err := rl.allow(ctx, "user-1"); err != nil {
		t.Fatalf("user-1 first: %v", err)
	}
	if err := rl.allow(ctx, "user-2"); err != nil {
		t.Fatalf("user-2 first: %v", err)
	}
}

func TestRefineRateLimiter_AnonymousNoPerUserLimit(t *testing.T) {
	rl := newRefineRateLimiter(100, 5*time.Minute, 1)
	ctx := context.Background()

	for i := 0; i < 5; i++ {
		if err := rl.allow(ctx, ""); err != nil {
			t.Fatalf("anonymous allow %d: %v", i, err)
		}
	}
}

func TestNewAIRefineService(t *testing.T) {
	svc := NewAIRefineService(nil)
	if svc == nil {
		t.Fatal("expected non-nil service")
	}
	if svc.rateLimit == nil {
		t.Fatal("expected non-nil rate limiter")
	}
}

func TestRefineRateLimiter_ErrorIsKratosError(t *testing.T) {
	rl := newRefineRateLimiter(1, 5*time.Minute, 1)
	ctx := context.Background()

	_ = rl.allow(ctx, "user-1")

	err := rl.allow(ctx, "user-1")
	if err == nil {
		t.Fatal("expected error")
	}
	ke := kerrors.FromError(err)
	if ke == nil {
		t.Fatalf("expected kratos error, got %T", err)
	}
	if ke.Code != 429 {
		t.Errorf("Code = %d, want 429", ke.Code)
	}
}

func TestRefineRateLimiter_GlobalBurstReasonCode(t *testing.T) {
	rl := newRefineRateLimiter(1, 5*time.Minute, 100)
	ctx := context.Background()

	_ = rl.allow(ctx, "user-1")

	err := rl.allow(ctx, "user-2")
	if err == nil {
		t.Fatal("expected error for global burst exceeded")
	}
	ke := kerrors.FromError(err)
	if ke.Reason != refineReasonRateLimit {
		t.Errorf("Reason = %q, want %q", ke.Reason, refineReasonRateLimit)
	}
}

func TestRefineRateLimiter_PerUserReasonCode(t *testing.T) {
	rl := newRefineRateLimiter(100, 5*time.Minute, 1)
	ctx := context.Background()

	_ = rl.allow(ctx, "user-1")

	err := rl.allow(ctx, "user-1")
	if err == nil {
		t.Fatal("expected error for per-user limit")
	}
	ke := kerrors.FromError(err)
	if ke.Reason != refineReasonRateLimitUser {
		t.Errorf("Reason = %q, want %q", ke.Reason, refineReasonRateLimitUser)
	}
}
