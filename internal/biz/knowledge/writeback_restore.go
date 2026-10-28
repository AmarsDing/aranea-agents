package knowledge

import (
	"context"
	"strings"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// RestoreTombstonedFact 把 DELETE 自动失效的事实段从最新墓碑写回正文（G-40 c1）。
// 最新版本 NewBody 非空、或没有墓碑时拒绝；正文已含该 fact_id 时幂等返回。
// 成功后再记一条反向版本（OldBody 固定为 tombstone，NewBody 为恢复段）。
func (u *Usecase) RestoreTombstonedFact(ctx context.Context, docID, factID string) error {
	if u == nil || u.documents == nil {
		return apierror.Unavailable(apierror.DomainKnowledge, "knowledge usecase is not configured")
	}
	reader, ok := u.factVersions.(FactVersionReader)
	if !ok {
		return apierror.Unavailable(apierror.DomainKnowledge, "fact version reader is not configured")
	}
	docID = strings.TrimSpace(docID)
	factID = strings.TrimSpace(factID)
	v, found, err := reader.LatestFactVersion(ctx, docID, factID)
	if err != nil {
		return err
	}
	if !found || strings.TrimSpace(v.NewBody) != "" || strings.TrimSpace(v.OldBody) == "" {
		return apierror.Conflict(apierror.DomainKnowledge, "fact %s has no tombstone to restore", factID)
	}
	doc, err := u.GetDocument(ctx, docID)
	if err != nil {
		return err
	}
	marker := "fact_id: `" + factID + "`"
	if strings.Contains(doc.ContentText, marker) {
		return nil
	}
	restored := strings.TrimSpace(v.OldBody)
	body := strings.TrimRight(doc.ContentText, "\n") + "\n\n" + restored + "\n"
	if err := u.documents.UpdateDocumentContent(ctx, doc.ID, body, true); err != nil {
		return err
	}
	u.recordFactVersion(ctx, v.CollectionID, doc.ID, factID, "tombstone", restored)
	if err := u.RebuildBlockIndex(ctx, doc.CollectionID, doc.ID, body); err != nil {
		u.lg.Warn("事实段恢复后块索引重建失败（正文已落，重建可自愈）",
			loggateway.StepID("knowledge.writeback.restore"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err),
		)
	}
	return nil
}
