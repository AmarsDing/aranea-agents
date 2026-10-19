package biz_test

import (
	"aranea-agents/internal/biz"
	"context"
	"testing"
)

// TestArtifactUsecase_DeleteVersionRemovesSingleVersion verifies that
// DeleteVersion removes exactly one version while leaving siblings intact.
func TestArtifactUsecase_DeleteVersionRemovesSingleVersion(t *testing.T) {
	uc := biz.NewArtifactUsecase(newMemArtifactRepoB(), nil)
	ctx := context.Background()

	v1, _ := uc.Save(ctx, "sess-4", "log.txt", "text/plain", []byte("v1"))
	v2, _ := uc.Save(ctx, "sess-4", "log.txt", "text/plain", []byte("v2"))
	v3, _ := uc.Save(ctx, "sess-4", "log.txt", "text/plain", []byte("v3"))

	if err := uc.DeleteVersion(ctx, v1.ID, v2.Version); err != nil {
		t.Fatalf("DeleteVersion: %v", err)
	}

	if _, _, err := uc.Load(ctx, v2.ID, 0); err == nil {
		t.Error("v2 should have been deleted")
	}

	if _, _, err := uc.Load(ctx, v1.ID, 0); err != nil {
		t.Errorf("v1 should remain: %v", err)
	}
	if _, _, err := uc.Load(ctx, v3.ID, 0); err != nil {
		t.Errorf("v3 should remain: %v", err)
	}

	if err := uc.DeleteVersion(ctx, v1.ID, 99); err == nil {
		t.Error("expected error for non-existent version 99")
	}
}
