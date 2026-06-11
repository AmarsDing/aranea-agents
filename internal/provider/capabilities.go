package provider

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/biz"
	artifactbiz "aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"
)

// ValidateAttachmentCapabilities checks whether the given provider/model supports
// the attachment types present in refs. Returns nil when there are no attachments
// or when all attachment types are supported.
func ValidateAttachmentCapabilities(ctx context.Context, catalog biz.TeamModelCatalog, prov, mod string, refs []artifactbiz.Ref, lg loggateway.Logger) error {
	if len(refs) == 0 {
		return nil
	}
	if HasImageAttachment(refs) && !ModelSupportsImageAttachments(ctx, catalog, prov, mod, lg) {
		return fmt.Errorf("%s/%s: %w", strings.TrimSpace(prov), strings.TrimSpace(mod), ErrImageNotSupported)
	}
	if HasFileAttachment(refs) && !ModelSupportsFileAttachments(ctx, catalog, prov, mod, lg) {
		return fmt.Errorf("%s/%s: %w", strings.TrimSpace(prov), strings.TrimSpace(mod), ErrFileNotSupported)
	}
	return nil
}

// HasImageAttachment returns true if any ref has an image/* MIME type.
func HasImageAttachment(refs []artifactbiz.Ref) bool {
	for _, ref := range refs {
		if strings.HasPrefix(strings.ToLower(strings.TrimSpace(ref.MimeType)), "image/") {
			return true
		}
	}
	return false
}

// HasFileAttachment returns true if any ref has a non-empty, non-image MIME type.
func HasFileAttachment(refs []artifactbiz.Ref) bool {
	for _, ref := range refs {
		mime := strings.ToLower(strings.TrimSpace(ref.MimeType))
		if mime != "" && !strings.HasPrefix(mime, "image/") {
			return true
		}
	}
	return false
}
