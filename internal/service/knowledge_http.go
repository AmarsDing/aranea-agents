package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"

	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

func writeKnowledgeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func writeKnowledgeJSONError(w http.ResponseWriter, err error) {
	switch {
	case apierror.IsCode(err, apierror.CodeNotFound):
		writeKnowledgeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case apierror.IsCode(err, apierror.CodeBadRequest):
		writeKnowledgeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
	case apierror.IsCode(err, apierror.CodeForbidden):
		writeKnowledgeJSON(w, http.StatusNotFound, map[string]string{"error": "not found"})
	case apierror.IsCode(err, apierror.CodeConflict):
		writeKnowledgeJSON(w, http.StatusConflict, map[string]string{"error": "conflict"})
	default:
		writeKnowledgeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
	}
}

func (s *KnowledgeService) knowledgeDocCollection(ctx context.Context, docID string) (bizknowledge.Document, bizknowledge.Collection, error) {
	doc, err := s.uc.GetDocument(ctx, docID)
	if err != nil {
		return bizknowledge.Document{}, bizknowledge.Collection{}, err
	}
	if err := s.assertDocumentReadable(ctx, doc); err != nil {
		return bizknowledge.Document{}, bizknowledge.Collection{}, err
	}
	col, err := s.uc.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return bizknowledge.Document{}, bizknowledge.Collection{}, err
	}
	return doc, col, nil
}

// ServeAutolinkPreview GET /v1/knowledge/documents/{id}/autolink-preview
func (s *KnowledgeService) ServeAutolinkPreview(w http.ResponseWriter, r *http.Request, docID string) {
	_, col, err := s.knowledgeDocCollection(r.Context(), docID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	prev, err := s.uc.PreviewOutgoingAutolinks(r.Context(), docID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{
		"doc_id":       prev.DocID,
		"replacements": prev.Replacements,
		"preview":      prev.Preview,
		"unchanged":    prev.Unchanged,
	})
}

// ServeAutolinkApply POST /v1/knowledge/documents/{id}/autolink
func (s *KnowledgeService) ServeAutolinkApply(w http.ResponseWriter, r *http.Request, docID string) {
	_, col, err := s.knowledgeDocCollection(r.Context(), docID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionMutateAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	res, err := s.uc.ApplyOutgoingAutolinks(r.Context(), docID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{
		"doc_id":       res.DocID,
		"replacements": res.Replacements,
	})
}

// ServeCollectionHealth GET /v1/knowledge/collections/{id}/health
func (s *KnowledgeService) ServeCollectionHealth(w http.ResponseWriter, r *http.Request, collectionID string) {
	col, err := s.uc.GetCollection(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	h, err := s.uc.CollectionHealthSnapshot(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{
		"document_count":   h.DocumentCount,
		"edge_count":       h.EdgeCount,
		"explicit_edges":   h.ExplicitEdges,
		"isolated_count":   h.IsolatedCount,
		"orphan_rate":      h.OrphanRate,
		"link_density":     h.LinkDensity,
		"dangling_count":   h.DanglingCount,
		"writeback_notes":  h.WriteBackNotes,
		"writeback_latest": h.WriteBackLatest,
	})
}

// ServeCollectionExperts GET /v1/knowledge/collections/{id}/experts
func (s *KnowledgeService) ServeCollectionExperts(w http.ResponseWriter, r *http.Request, collectionID string) {
	col, err := s.uc.GetCollection(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	items, err := s.uc.ListCollectionExperts(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, e := range items {
		out = append(out, map[string]any{
			"agent_id":   e.AgentID,
			"user_id":    e.UserID,
			"fact_count": e.FactCount,
			"last_kind":  e.LastKind,
		})
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"items": out})
}

// ServeWriteBackPending GET /v1/knowledge/collections/{id}/writeback-pending
func (s *KnowledgeService) ServeWriteBackPending(w http.ResponseWriter, r *http.Request, collectionID string) {
	col, err := s.uc.GetCollection(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	items, err := s.uc.ListPendingWriteBack(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		out = append(out, map[string]any{
			"fact_id":    it.Fact.FactID,
			"statement":  it.Fact.Statement,
			"kind":       it.Fact.FactKind,
			"confidence": it.Fact.Confidence,
			"agent_id":   it.AgentID,
			"user_id":    it.UserID,
			"session_id": it.SessionID,
			"source":     it.Fact.SourceKind,
		})
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"items": out})
}

type writeBackPendingApplyBody struct {
	FactIDs []string `json:"fact_ids"`
}

// ServeWriteBackPendingApply POST /v1/knowledge/collections/{id}/writeback-pending/apply
func (s *KnowledgeService) ServeWriteBackPendingApply(w http.ResponseWriter, r *http.Request, collectionID string) {
	col, err := s.uc.GetCollection(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionMutateAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	var body writeBackPendingApplyBody
	if r.Body != nil {
		raw, _ := io.ReadAll(io.LimitReader(r.Body, 1<<20))
		if len(strings.TrimSpace(string(raw))) > 0 {
			_ = json.Unmarshal(raw, &body)
		}
	}
	res, err := s.uc.ApplyPendingWriteBack(r.Context(), collectionID, body.FactIDs)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if res.DocID != "" {
		replayed, failed := s.replayPromotedDocChunks(r.Context(), col, []bizknowledge.PromoteTouchedDoc{{
			DocID:   res.DocID,
			Created: res.Created,
		}})
		if failed > 0 {
			s.lg.Warn("待确认写回 chunk 重放部分失败",
				loggateway.StepID("knowledge.writeback.pending"),
				loggateway.Str("doc_id", res.DocID),
				loggateway.Int("replayed", replayed),
				loggateway.Int("failed", failed),
			)
		}
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{
		"collection_id": res.CollectionID,
		"doc_id":        res.DocID,
		"appended":      res.Appended,
		"created":       res.Created,
	})
}

// ServeWriteBackHome GET /v1/knowledge/writeback-home
// 只解析、不创建。专家 / pending / 健康度提示用此落点，避免扫一眼就造团队库。
func (s *KnowledgeService) ServeWriteBackHome(w http.ResponseWriter, r *http.Request) {
	ws := workspace.IDFromContext(r.Context())
	col, found, err := s.uc.LookupWriteBackHome(r.Context(), ws)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if !found {
		writeKnowledgeJSON(w, http.StatusOK, map[string]any{"found": false})
		return
	}
	if err := s.assertCollectionAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{
		"found":         true,
		"collection_id": col.ID,
		"name":          col.Name,
		"vault_backend": col.VaultBackend,
	})
}

// ServeAutolinkBackfill POST /v1/knowledge/collections/{id}/autolink-backfill
// 显式回填存量出链后再重建块索引（US-45）。与 RebuildKnowledgeIndex 共用 rebuildRuns 互斥门。
func (s *KnowledgeService) ServeAutolinkBackfill(w http.ResponseWriter, r *http.Request, collectionID string) {
	col, err := s.uc.GetCollection(r.Context(), collectionID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionMutateAccess(r.Context(), col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.startCollectionIndexJob(r.Context(), col, true); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{"status": bizknowledge.SyncStateRebuilding})
}

// ServeDocumentVisibility GET/POST /v1/knowledge/documents/{id}/visibility
func (s *KnowledgeService) ServeDocumentVisibility(w http.ResponseWriter, r *http.Request, docID string) {
	ctx := r.Context()
	doc, col, err := s.knowledgeDocCollection(ctx, docID)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	if r.Method == http.MethodGet {
		writeKnowledgeJSON(w, http.StatusOK, map[string]any{
			"id":            doc.ID,
			"visibility":    doc.Visibility,
			"owner_user_id": doc.OwnerUserID,
		})
		return
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	var body struct {
		Visibility string `json:"visibility"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		writeKnowledgeJSON(w, http.StatusBadRequest, map[string]string{"error": "bad request"})
		return
	}
	updated, err := s.uc.UpdateDocumentVisibility(ctx, docID, body.Visibility)
	if err != nil {
		writeKnowledgeJSONError(w, err)
		return
	}
	writeKnowledgeJSON(w, http.StatusOK, map[string]any{
		"id":            updated.ID,
		"visibility":    updated.Visibility,
		"owner_user_id": updated.OwnerUserID,
	})
}
