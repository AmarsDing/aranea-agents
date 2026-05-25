package artifact_test

import (
	"testing"

	"aranea-agents/internal/biz/artifact"
)

func TestValidateUploadSize(t *testing.T) {
	if err := artifact.ValidateUploadSize(artifact.MaxUploadBytes); err != nil {
		t.Fatalf("at limit: %v", err)
	}
	if err := artifact.ValidateUploadSize(artifact.MaxUploadBytes + 1); err == nil {
		t.Fatal("expected error above limit")
	}
}
