package service

import (
	"testing"

	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/internal/provider"
)

func TestHasImageAttachment(t *testing.T) {
	if !provider.HasImageAttachment([]artifactbiz.Ref{{MimeType: "image/png"}}) {
		t.Fatal("expected image attachment")
	}
	if provider.HasImageAttachment([]artifactbiz.Ref{{MimeType: "application/pdf"}}) {
		t.Fatal("did not expect image attachment")
	}
}
