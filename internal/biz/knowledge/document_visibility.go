package knowledge

import (
	"context"
	"strconv"
	"strings"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/auth"
)

// DocumentACLStore 文档可见性写口（可选接线；未接线时 UpdateDocumentVisibility 不可用）。
type DocumentACLStore interface {
	UpdateDocumentVisibility(ctx context.Context, id, visibility, ownerUserID string) error
}

func (u *Usecase) SetDocumentACLStore(s DocumentACLStore) {
	if u != nil {
		u.docACL = s
	}
}

func viewerUserID(ctx context.Context) string {
	if a, ok := auth.FromContext(ctx); ok && a != nil && a.UserID > 0 {
		return strconv.FormatInt(a.UserID, 10)
	}
	return ""
}

// DocumentVisibleTo reports whether ctx may read d. System sees all.
// collection (or empty) is visible to anyone with collection access; private
// is owner-only.
func DocumentVisibleTo(ctx context.Context, d Document) bool {
	if workspace.IsSystem(ctx) {
		return true
	}
	vis := strings.TrimSpace(d.Visibility)
	if vis == "" || vis == DocVisibilityCollection {
		return true
	}
	if vis != DocVisibilityPrivate {
		return false
	}
	owner := strings.TrimSpace(d.OwnerUserID)
	uid := viewerUserID(ctx)
	return owner != "" && owner == uid
}

func (u *Usecase) UpdateDocumentVisibility(ctx context.Context, id, visibility string) (Document, error) {
	if u == nil || u.documents == nil {
		return Document{}, ErrUnavailable
	}
	if u.docACL == nil {
		return Document{}, apierror.Unavailable("KNOWLEDGE", "document visibility is not configured")
	}
	id = strings.TrimSpace(id)
	if id == "" {
		return Document{}, ErrIDRequired
	}
	doc, err := u.documents.GetDocument(ctx, id)
	if err != nil {
		return Document{}, err
	}
	visibility = strings.TrimSpace(strings.ToLower(visibility))
	if visibility == "" {
		visibility = DocVisibilityCollection
	}
	if visibility != DocVisibilityCollection && visibility != DocVisibilityPrivate {
		return Document{}, apierror.BadRequest("KNOWLEDGE", "visibility must be collection or private")
	}
	uid := viewerUserID(ctx)
	owner := strings.TrimSpace(doc.OwnerUserID)
	switch visibility {
	case DocVisibilityPrivate:
		if uid == "" {
			return Document{}, apierror.BadRequest("KNOWLEDGE", "sign in required to mark a document private")
		}
		owner = uid
	case DocVisibilityCollection:
		// keep owner for audit; visibility change is the ACL switch
	}
	if err := u.docACL.UpdateDocumentVisibility(ctx, id, visibility, owner); err != nil {
		return Document{}, err
	}
	doc.Visibility = visibility
	doc.OwnerUserID = owner
	return doc, nil
}
