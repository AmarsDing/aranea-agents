package knowledge

import (
	"context"

	"aranea-agents/pkg/apierror"
)

// RebuildCurrentBlockIndex converges on the latest durable body when writers overlap.
func (u *Usecase) RebuildCurrentBlockIndex(ctx context.Context, docID string) error {
	if u == nil || u.documents == nil {
		return ErrUnavailable
	}
	for attempt := 0; attempt < 3; attempt++ {
		doc, err := u.documents.GetDocument(ctx, docID)
		if err != nil {
			return err
		}
		if err := u.RebuildBlockIndex(ctx, doc.CollectionID, doc.ID, doc.ContentText); err != nil {
			return err
		}
		latest, err := u.documents.GetDocument(ctx, docID)
		if err != nil {
			return err
		}
		if HashContent(latest.ContentText) == HashContent(doc.ContentText) {
			return nil
		}
	}
	return apierror.Conflict(apierror.DomainKnowledge, "document changed repeatedly while rebuilding links")
}
