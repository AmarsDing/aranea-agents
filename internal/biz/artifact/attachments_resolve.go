package artifact

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
)

// Domain errors for attachment resolution.
var (
	// ErrArtifactServiceRequired is returned when attachment IDs are provided but the artifact service is nil.
	ErrArtifactServiceRequired = apierror.BadRequest(apierror.DomainArtifact, "artifact: attachments require artifact service")
	// ErrAttachmentLoadFailed is returned when an attachment ID cannot be loaded.
	ErrAttachmentLoadFailed = apierror.BadRequest(apierror.DomainArtifact, "artifact: attachment load failed")
	// ErrAttachmentWrongSession is returned when an attachment belongs to a different session.
	ErrAttachmentWrongSession = apierror.BadRequest(apierror.DomainArtifact, "artifact: attachment belongs to another session")
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
	// Batch-load all artifact metadata in a single call to avoid N+1 (S2 fix).
	metas, err := uc.LoadMetas(ctx, ids, 0)
	if err != nil {
		return nil, apierror.BadRequest(apierror.DomainArtifact, "load attachments: %s", err.Error()).WithCause(err)
	}
	loadedByID := make(map[string]Artifact, len(metas))
	for _, m := range metas {
		loadedByID[m.ID] = m
	}
	refs := make([]Ref, 0, len(ids))
	for _, id := range ids {
		meta, ok := loadedByID[id]
		if !ok {
			return nil, apierror.BadRequest(apierror.DomainArtifact, "attachment %s not found", id)
		}
		if strings.TrimSpace(meta.SessionID) != "" && sessionID != "" && meta.SessionID != sessionID {
			return nil, apierror.BadRequest(apierror.DomainArtifact, "attachment %s belongs to another session", id)
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
