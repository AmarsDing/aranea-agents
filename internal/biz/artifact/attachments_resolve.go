package artifact

import (
	"context"
	"fmt"
	"strings"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

// NormalizeAttachmentIDs trims and drops empty IDs while preserving order.
func NormalizeAttachmentIDs(ids []string) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			out = append(out, id)
		}
	}
	return out
}

// ResolveAttachmentRefs validates attachment IDs for sessionID and returns metadata refs.
// Every non-empty ID must load successfully and belong to the session when both IDs are set.
func ResolveAttachmentRefs(ctx context.Context, uc *Usecase, sessionID string, ids []string) ([]Ref, error) {
	ids = NormalizeAttachmentIDs(ids)
	if len(ids) == 0 {
		return nil, nil
	}
	if uc == nil {
		return nil, kerrors.BadRequest("ARTIFACT", "attachments require artifact service")
	}
	sessionID = strings.TrimSpace(sessionID)
	refs := make([]Ref, 0, len(ids))
	for _, id := range ids {
		meta, err := uc.LoadMeta(ctx, id, 0)
		if err != nil {
			return nil, kerrors.BadRequest("ARTIFACT", fmt.Sprintf("load attachment %s: %s", id, err.Error()))
		}
		if strings.TrimSpace(meta.SessionID) != "" && sessionID != "" && meta.SessionID != sessionID {
			return nil, kerrors.BadRequest("ARTIFACT", fmt.Sprintf("attachment %s belongs to another session", id))
		}
		refs = append(refs, Ref{
			ID:       meta.ID,
			Name:     meta.Name,
			MimeType: meta.MimeType,
			Size:     meta.Size,
		})
	}
	return refs, nil
}
