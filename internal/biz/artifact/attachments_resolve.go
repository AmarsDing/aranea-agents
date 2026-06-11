package artifact

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// Domain errors for attachment resolution — the Service layer maps these to apierror.
var (
	// ErrArtifactServiceRequired is returned when attachment IDs are provided but the artifact service is nil.
	ErrArtifactServiceRequired = errors.New("artifact: attachments require artifact service")
	// ErrAttachmentLoadFailed is returned when an attachment ID cannot be loaded.
	ErrAttachmentLoadFailed = errors.New("artifact: attachment load failed")
	// ErrAttachmentWrongSession is returned when an attachment belongs to a different session.
	ErrAttachmentWrongSession = errors.New("artifact: attachment belongs to another session")
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
		return nil, ErrArtifactServiceRequired
	}
	sessionID = strings.TrimSpace(sessionID)
	refs := make([]Ref, 0, len(ids))
	for _, id := range ids {
		meta, err := uc.LoadMeta(ctx, id, 0)
		if err != nil {
			return nil, fmt.Errorf("%w: load attachment %s: %s", ErrAttachmentLoadFailed, id, err.Error())
		}
		if strings.TrimSpace(meta.SessionID) != "" && sessionID != "" && meta.SessionID != sessionID {
			return nil, fmt.Errorf("%w: attachment %s", ErrAttachmentWrongSession, id)
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
