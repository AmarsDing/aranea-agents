package biz

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"
)

func TestExtractUserIDFromTaskMeta(t *testing.T) {
	if got := extractUserIDFromTaskMeta(nil); got != "" {
		t.Fatalf("nil -> %q", got)
	}
	if got := extractUserIDFromTaskMeta(map[string]any{}); got != "" {
		t.Fatalf("empty -> %q", got)
	}
	raw := map[string]any{"metadata_json": `{"user_id":"u-1"}`}
	if got := extractUserIDFromTaskMeta(raw); got != "u-1" {
		t.Fatalf("got %q want u-1", got)
	}
}

func TestSessionUserIDResolver_UsedWhenMetaEmpty(t *testing.T) {
	// vec non-nil so constructor returns a usecase (admin may be nil).
	uc := NewMemoryAdminUsecase(nil, &MemoryUsecase{}, nil, nil, loggateway.NewNoop())
	if uc == nil {
		t.Fatal("usecase nil")
	}
	called := false
	uc.SetSessionUserIDResolver(func(ctx context.Context, sessionID string) (string, error) {
		called = true
		if sessionID != "sess-1" {
			t.Fatalf("sessionID=%s", sessionID)
		}
		return "user-from-session", nil
	})
	userID := extractUserIDFromTaskMeta(map[string]any{})
	if userID == "" && uc.resolveUserID != nil {
		uid, err := uc.resolveUserID(context.Background(), "sess-1")
		if err != nil {
			t.Fatal(err)
		}
		userID = uid
	}
	if !called || userID != "user-from-session" {
		t.Fatalf("called=%v userID=%q", called, userID)
	}
}
