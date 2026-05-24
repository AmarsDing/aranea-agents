package ctxuser

import (
	"context"
	"testing"

	"aranea-agents/pkg/auth"
)

func TestTRPCUserKeyDefault(t *testing.T) {
	if got := TRPCUserKey(context.Background()); got != DefaultUserID {
		t.Fatalf("TRPCUserKey() = %q, want %q", got, DefaultUserID)
	}
}

func TestTRPCUserKeyIgnoresAuth(t *testing.T) {
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "admin"})
	if got := TRPCUserKey(ctx); got != DefaultUserID {
		t.Fatalf("TRPCUserKey() = %q, want %q (auth must not affect trpc key)", got, DefaultUserID)
	}
}

func TestTRPCUserKeyExplicit(t *testing.T) {
	ctx := WithUserID(context.Background(), "user-42")
	if got := TRPCUserKey(ctx); got != "user-42" {
		t.Fatalf("TRPCUserKey() = %q, want user-42", got)
	}
}

func TestFromContextAuth(t *testing.T) {
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "admin"})
	if got := FromContext(ctx); got != "7" {
		t.Fatalf("FromContext() = %q, want 7", got)
	}
}

func TestFromContextExplicitWinsOverAuth(t *testing.T) {
	ctx := auth.NewContext(context.Background(), &auth.Auth{UserID: 7, Access: "admin"})
	ctx = WithUserID(ctx, "explicit")
	if got := FromContext(ctx); got != "explicit" {
		t.Fatalf("FromContext() = %q, want explicit", got)
	}
}
