package knowledge

import (
	"context"
	"strings"

	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// TeamContentStore atomically replaces team text and invalidates its old chunks.
// Stability: Internal
type TeamContentStore interface {
	ReplaceTeamDocumentContent(context.Context, string, string, string) (bool, error)
}

func (u *Usecase) requireTeamCollection(ctx context.Context, id string) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	col, err := u.collections.GetCollection(ctx, id)
	if err != nil {
		return Collection{}, err
	}
	if col.RootPath != "" {
		return Collection{}, apierror.BadRequest(apierror.DomainKnowledge, "managed note requires a team library")
	}
	if !workspace.IsSystem(ctx) {
		if err := workspace.AssertWorkspaceMutate(workspace.IDFromContext(ctx), col.Workspace); err != nil {
			return Collection{}, apierror.NotFound(apierror.DomainKnowledge, "collection not found")
		}
	}
	return col, nil
}

func (u *Usecase) CreateTeamDocument(ctx context.Context, collectionID, relPath string) (Document, error) {
	col, err := u.requireTeamCollection(ctx, collectionID)
	if err != nil {
		return Document{}, err
	}
	rel, err := SanitizeRelPath(relPath)
	if err != nil {
		return Document{}, err
	}
	if !strings.HasSuffix(strings.ToLower(rel), ".md") {
		return Document{}, apierror.BadRequest(apierror.DomainKnowledge, "note path must end in .md")
	}
	doc, err := u.CreateDocument(ctx, Document{CollectionID: col.ID, Source: rel, RelPath: rel,
		MimeType: "text/markdown", ContentText: "# \n", ContentHash: HashContent("# \n"), Organized: true})
	if err != nil {
		return Document{}, err
	}
	u.replayTeamDocument(ctx, col, doc)
	return u.documents.GetDocument(ctx, doc.ID)
}

func (u *Usecase) UpdateTeamDocumentContent(ctx context.Context, id, content, baseHash string) (Document, bool, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, false, err
	}
	doc, err := u.documents.GetDocument(ctx, id)
	if err != nil {
		return Document{}, false, err
	}
	col, err := u.requireTeamCollection(ctx, doc.CollectionID)
	if err != nil {
		return Document{}, false, err
	}
	if !DocumentVisibleTo(ctx, doc) {
		return Document{}, false, apierror.NotFound(apierror.DomainKnowledge, "document not found")
	}
	if !VaultDocumentEditable(doc) {
		return Document{}, false, apierror.BadRequest(apierror.DomainKnowledge, "original document is read-only")
	}
	store, ok := u.documents.(TeamContentStore)
	if !ok {
		return Document{}, false, apierror.Unavailable(apierror.DomainKnowledge, "managed note writer not configured")
	}
	applied, err := store.ReplaceTeamDocumentContent(ctx, id, baseHash, content)
	if err != nil {
		return Document{}, false, err
	}
	if !applied {
		return doc, true, nil
	}
	doc.ContentText, doc.ContentHash, doc.Status = content, HashContent(content), string(DocumentStatePending)
	u.replayTeamDocument(ctx, col, doc)
	updated, err := u.documents.GetDocument(ctx, id)
	if err != nil {
		return Document{}, false, err
	}
	// Return the submitted version even if another writer has since committed.
	updated.ContentHash = doc.ContentHash
	updated.ContentText = content
	return updated, false, nil
}

func (u *Usecase) replayTeamDocument(ctx context.Context, col Collection, doc Document) {
	if err := u.RebuildCurrentBlockIndex(ctx, doc.ID); err != nil {
		u.lg.Warn("团队笔记已保存，块索引等待修复", loggateway.StepID("knowledge.document.save"), loggateway.Err(err))
	}
	if u.writeBackReplay != nil {
		if err := u.writeBackReplay(ctx, col, []PromoteTouchedDoc{{DocID: doc.ID}}); err != nil {
			u.lg.Warn("团队笔记已保存，检索索引等待修复", loggateway.StepID("knowledge.document.save"), loggateway.Err(err))
		}
	}
}
