package knowledge

import (
	"context"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// GovernanceResolutionRunner serializes proposal disposal and its durable state.
// Stability: Internal
type GovernanceResolutionRunner interface {
	ResolveGovernanceAtomically(context.Context, int64, func(context.Context, GovernanceProposalView) (string, error)) error
}

type governanceDerivedKey struct{}
type governanceDerivedQueue struct {
	docIDs map[string]PromoteTouchedDoc
}

// DeferKnowledgeReplay queues derived work until the governance transaction commits.
func DeferKnowledgeReplay(ctx context.Context, docID string) bool {
	return deferKnowledgeReplay(ctx, PromoteTouchedDoc{DocID: docID})
}

func deferKnowledgeReplay(ctx context.Context, touched PromoteTouchedDoc) bool {
	q, ok := ctx.Value(governanceDerivedKey{}).(*governanceDerivedQueue)
	if !ok {
		return false
	}
	touched.Created = touched.Created || q.docIDs[touched.DocID].Created
	q.docIDs[touched.DocID] = touched
	return true
}

func (u *Usecase) deferGovernanceBlockIndex(ctx context.Context, docID string) (bool, error) {
	if !DeferKnowledgeReplay(ctx, docID) {
		return false, nil
	}
	store, ok := u.documents.(TeamContentStore)
	if !ok {
		return true, nil
	}
	doc, err := u.documents.GetDocument(ctx, docID)
	if err != nil {
		return true, err
	}
	applied, err := store.ReplaceTeamDocumentContent(ctx, docID, HashContent(doc.ContentText), doc.ContentText)
	if err == nil && !applied {
		err = apierror.Conflict(apierror.DomainKnowledge, "document changed during governance")
	}
	return true, err
}

func (u *Usecase) replayGovernanceQueue(ctx context.Context, q *governanceDerivedQueue) {
	for docID, touched := range q.docIDs {
		doc, err := u.documents.GetDocument(ctx, docID)
		if err != nil {
			u.lg.Warn("治理派生任务读取失败", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
			continue
		}
		col, err := u.collections.GetCollection(ctx, doc.CollectionID)
		if err != nil {
			u.lg.Warn("治理派生任务读取库失败", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
			continue
		}
		if err := u.RebuildCurrentBlockIndex(ctx, doc.ID); err != nil {
			u.lg.Warn("治理正文已提交，块索引等待修复", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
		}
		if u.writeBackReplay != nil {
			// Another writer may already have indexed the new page after commit.
			touched.Created = touched.Created && doc.Status != string(DocumentStateIndexed)
			if err := u.writeBackReplay(ctx, col, []PromoteTouchedDoc{touched}); err != nil {
				u.lg.Warn("治理正文已提交，检索索引等待修复", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
			}
		}
	}
}

func (u *Usecase) GetGovernanceProposal(ctx context.Context, id int64) (GovernanceProposalView, error) {
	if u == nil || u.curate == nil {
		return GovernanceProposalView{}, apierror.Unavailable(apierror.DomainKnowledge, "knowledge curate repo not wired")
	}
	return u.curate.GetGovernanceProposal(ctx, id)
}

func (u *Usecase) replayGovernedDocument(ctx context.Context, view GovernanceProposalView) {
	docID, _ := view.Payload["doc_id"].(string)
	if view.Kind == ProposalKindOrphan && docID != "" {
		u.removeLinkIndexDoc(ctx, docID)
		return
	}
	factID, _ := view.Payload["target_fact_id"].(string)
	if view.Kind != ProposalKindConflict || docID == "" || factID == "" {
		return
	}
	err := u.RebuildCurrentBlockIndex(ctx, docID)
	if err != nil {
		u.lg.Warn("治理正文已提交，块索引等待修复", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
	}
	if u.writeBackReplay != nil && u.collections != nil {
		col, err := u.collections.GetCollection(ctx, view.CollectionID)
		if err == nil {
			err = u.writeBackReplay(ctx, col, []PromoteTouchedDoc{{DocID: docID}})
		}
		if err != nil {
			u.lg.Warn("治理正文已提交，检索索引等待修复", loggateway.StepID("knowledge.curate.conflict"), loggateway.Err(err))
		}
	}
}
