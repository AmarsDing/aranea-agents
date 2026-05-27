package service

import (
	"testing"

	artifactbiz "aranea-agents/internal/biz/artifact"
)

func TestHasImageAttachment(t *testing.T) {
	if !hasImageAttachment([]artifactbiz.Ref{{MimeType: "image/png"}}) {
		t.Fatal("expected image attachment")
	}
	if hasImageAttachment([]artifactbiz.Ref{{MimeType: "application/pdf"}}) {
		t.Fatal("did not expect image attachment")
	}
}
