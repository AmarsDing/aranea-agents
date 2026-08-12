package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ReembedDocuments B1：从已存 content_text 重建 chunks+embedding（无需原始文件）。
// 修复维度对账（reconcileEmbeddingDim）置 NULL 的 UI 上传文档——vault 文档经
// vault_sync 自愈不进入本链路。同步返回受理计数；重嵌入在单后台 goroutine
// 串行执行（复用摄取管线路径，不打爆 embedder API）。
func (s *KnowledgeService) ReembedDocuments(ctx context.Context, req *v1.ReembedDocumentsRequest) (*v1.ReembedDocumentsResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	// 词法库（无 embedding_model）：重嵌入无语义层无意义。
	if strings.TrimSpace(col.EmbeddingModel) == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "collection has no semantic layer (embedding_model is empty)")
	}
	if s.embedder == nil {
		return nil, apierror.BadRequest("KNOWLEDGE", "embedder not configured")
	}

	// 筛选目标：显式 doc_ids（per-id GetDocument + 跳过规则）或默认全集合待重嵌入。
	var docs []biz.KnowledgeDocument
	skipped := 0
	if len(req.GetDocIds()) > 0 {
		for _, id := range req.GetDocIds() {
			d, getErr := s.uc.GetDocument(ctx, id)
			if getErr != nil || d.CollectionID != col.ID ||
				strings.TrimSpace(d.ContentText) == "" || d.Status == "indexing" {
				skipped++
				continue
			}
			docs = append(docs, d)
		}
	} else {
		pending, listErr := s.uc.ListDocumentsPendingReembed(ctx, col.ID)
		if listErr != nil {
			return nil, listErr
		}
		docs = pending
	}
	if len(docs) == 0 {
		return &v1.ReembedDocumentsResponse{AcceptedCount: 0, SkippedCount: int32(skipped)}, nil
	}

	// 流程日志：重嵌入批次开始（per-doc parse/embed 细节由 BuildIndexedChunks 发射，
	// 批次完成在 goroutine 末尾发射，共用同一 emitter）。
	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.reembed.start", "文档重嵌入开始",
		event.P("collection_id", col.ID),
		event.P("doc_count", len(docs)))

	embedder := s.embedder
	uc := s.uc
	reembedCtx := appctx.Ctx()
	safego.Go(reembedCtx, "knowledge-reembed", func() {
		s.lg.Info("knowledge reembed worker started", // K7 启动
			loggateway.StepID("knowledge.reembed.worker_start"),
			loggateway.Str("collection_id", col.ID),
			loggateway.Int("doc_count", len(docs)))
		defer s.lg.Info("knowledge reembed worker exited", // K7 退出
			loggateway.StepID("knowledge.reembed.worker_exit"),
			loggateway.Str("collection_id", col.ID))
		for _, doc := range docs {
			s.reembedOneDocument(reembedCtx, uc, embedder, col, doc, req.GetChunkSize(), req.GetChunkOverlap(), flow)
		}
		flow.LogDone("knowledge.reembed.done", "文档重嵌入完成",
			event.P("collection_id", col.ID),
			event.P("doc_count", len(docs)))
	})
	return &v1.ReembedDocumentsResponse{AcceptedCount: int32(len(docs)), SkippedCount: int32(skipped)}, nil
}

// reembedOneDocument 单文档串行管线：DeleteChunksByDocument → indexing+WS →
// BuildIndexedChunks(content_text) → InsertChunks → indexed+WS。失败置 error 由调用方继续下一篇。
// 不触发 RebuildBlockIndex（content_text 未变，块/边不变——与 IngestDocument 的 SP1-C 钩子区分）。
func (s *KnowledgeService) reembedOneDocument(ctx context.Context, uc *biz.KnowledgeUsecase, embedder knowledge.Embedder, col biz.KnowledgeCollection, doc biz.KnowledgeDocument, chunkSize, chunkOverlap int32, flow *event.TraceEmitter) {
	// fail 置文档 error 终态 + WS 广播 + 流程/进程双轨错误日志（K2/K3），不回滚已删旧块
	// （重嵌入幂等，下次调用或维度对账后可重试）。
	fail := func(stage string, err error) {
		if statusErr := uc.UpdateDocumentStatus(ctx, doc.ID, "error", err.Error(), 0); statusErr != nil {
			s.lg.Error("failed to update document status to error",
				loggateway.StepID("knowledge.reembed.status_fail"),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(statusErr),
				loggateway.Str("original_error", err.Error()))
		}
		s.publishKnowledgeIngest(col.ID, doc.ID, "error", err.Error(), 0)
		flow.LogError("knowledge.reembed.done", "文档重嵌入失败",
			event.P("doc_id", doc.ID),
			event.P("error", err.Error()))
		s.lg.Warn("knowledge reembed document failed",
			loggateway.StepID("knowledge.reembed.doc_fail"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Str("stage", stage),
			loggateway.Err(err))
	}

	if err := uc.DeleteChunksByDocument(ctx, doc.ID); err != nil {
		fail("delete_chunks", err)
		return
	}
	if err := uc.UpdateDocumentStatus(ctx, doc.ID, "indexing", "", 0); err != nil {
		s.lg.Error("failed to update document status to indexing",
			loggateway.StepID("knowledge.reembed.status_fail"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err))
	}
	s.publishKnowledgeIngest(col.ID, doc.ID, "indexing", "", 0)

	params := knowledge.IngestParams{
		DocID:        doc.ID,
		CollectionID: col.ID,
		Text:         doc.ContentText,
		ChunkSize:    int(chunkSize),
		ChunkOverlap: int(chunkOverlap),
	}
	params.ApplyDefaults()
	bizChunks, err := knowledge.BuildIndexedChunks(ctx, embedder, params, flow)
	if err != nil {
		fail("build_chunks", err)
		return
	}
	if err := uc.InsertChunks(ctx, bizChunks); err != nil {
		fail("insert_chunks", err)
		return
	}
	if err := uc.UpdateDocumentStatus(ctx, doc.ID, "indexed", "", len(bizChunks)); err != nil {
		s.lg.Error("failed to update document status to indexed",
			loggateway.StepID("knowledge.reembed.status_fail"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err))
		// Document was likely deleted mid-reembed; broadcast error to avoid stale UI state.
		s.publishKnowledgeIngest(col.ID, doc.ID, "error", "document deleted during reembed", 0)
		return
	}
	s.publishKnowledgeIngest(col.ID, doc.ID, "indexed", "", len(bizChunks))
	s.lg.Info("knowledge reembed document done", // K7 每文档 done
		loggateway.StepID("knowledge.reembed.doc_done"),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Int("chunk_count", len(bizChunks)))
}
