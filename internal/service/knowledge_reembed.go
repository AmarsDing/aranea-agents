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
	// 词法库（无 embedding_model）：允许纯词法重建（embedder=nil，只分块/FTS
	// 不产向量）——team 词法库写回词条 chunks 缺失时的自愈入口；此前一律拒绝
	// 导致词法库 chunks 永不修复（2026-08-15 词条页 pending 事故）。
	lexicalOnly := strings.TrimSpace(col.EmbeddingModel) == ""
	if !lexicalOnly && s.embedder == nil {
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
	if lexicalOnly {
		embedder = nil // 词法库纯分块/FTS 重建，不产向量
	}
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

// EnableCollectionSemantic B2：词法集合（embedding_model 为空）单向启用语义层——
// 绑定当前全局 embedder 的 model/dim（守卫式 UPDATE，并发/重复调用 → Conflict，
// 不可改绑/清空），随后全集合有正文文档进入 B1 同一串行重嵌入管线
// （启用后全部 chunks 缺失，恰为 ListDocumentsPendingReembed 集合）。
func (s *KnowledgeService) EnableCollectionSemantic(ctx context.Context, req *v1.EnableCollectionSemanticRequest) (*v1.EnableCollectionSemanticResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	// 单向启用：已有 embedding_model 则拒绝（防改绑/清空）。
	if strings.TrimSpace(col.EmbeddingModel) != "" {
		return nil, apierror.Conflict("KNOWLEDGE", "semantic layer already enabled")
	}
	if s.embedderAdmin == nil {
		return nil, apierror.BadRequest("KNOWLEDGE", "embedder not configured")
	}
	_, _, model, dim, configured, _ := s.embedderAdmin.Config()
	if !configured {
		return nil, apierror.BadRequest("KNOWLEDGE", "embedder not configured")
	}
	if err := s.uc.EnableCollectionSemantic(ctx, col.ID, model, dim); err != nil {
		return nil, err
	}

	// 全集合有正文文档入队（复用 B1 同一串行管线；chunks 缺失/NULL 即 pending）。
	docs, err := s.uc.ListDocumentsPendingReembed(ctx, col.ID)
	if err != nil {
		return nil, err
	}
	resp := &v1.EnableCollectionSemanticResponse{
		EnqueuedDocs:   int32(len(docs)),
		EmbeddingModel: model,
		Dim:            int32(dim),
	}
	if len(docs) == 0 {
		return resp, nil
	}

	// 流程日志：语义层启用批次开始；完成在 goroutine 末尾发射（共用同一 emitter）。
	flow := s.knowledgeFlow(ctx)
	flow.LogStart("knowledge.collection.enable_semantic", "集合语义层启用",
		event.P("collection_id", col.ID),
		event.P("embedding_model", model),
		event.P("dim", dim),
		event.P("doc_count", len(docs)))

	embedder := s.embedder
	uc := s.uc
	reembedCtx := appctx.Ctx()
	safego.Go(reembedCtx, "knowledge-enable-semantic", func() {
		s.lg.Info("knowledge enable-semantic worker started", // K7 启动
			loggateway.StepID("knowledge.collection.enable_semantic"),
			loggateway.Str("collection_id", col.ID),
			loggateway.Int("doc_count", len(docs)))
		defer s.lg.Info("knowledge enable-semantic worker exited", // K7 退出
			loggateway.StepID("knowledge.collection.enable_semantic"),
			loggateway.Str("collection_id", col.ID))
		for _, doc := range docs {
			s.reembedOneDocument(reembedCtx, uc, embedder, col, doc, 0, 0, flow)
		}
		flow.LogDone("knowledge.collection.enable_semantic", "集合语义层启用",
			event.P("collection_id", col.ID),
			event.P("doc_count", len(docs)))
	})
	return resp, nil
}

// RepairPendingKnowledgeIndexes synchronously repairs a bounded batch of
// database-backed documents whose source text is durable but chunks/embeddings
// are missing. It is the worker-facing counterpart of the user-triggered RPC.
func (s *KnowledgeService) RepairPendingKnowledgeIndexes(ctx context.Context, limit int) (repaired, failed int, err error) {
	if s == nil || s.uc == nil {
		return 0, 0, apierror.Unavailable(apierror.DomainKnowledge, "knowledge index repair is not configured")
	}
	if limit <= 0 {
		return 0, 0, nil
	}
	cols, _, err := s.uc.ListCollections(ctx, "", 1000, 0)
	if err != nil {
		return 0, 0, err
	}
	flow := s.knowledgeFlow(ctx)
	for _, col := range cols {
		if repaired+failed >= limit {
			break
		}
		docs, listErr := s.uc.ListDocumentsPendingReembed(ctx, col.ID)
		if listErr != nil {
			failed++
			s.lg.Warn("索引修复列举待处理文档失败",
				loggateway.StepID("knowledge.index_repair"),
				loggateway.Str("collection_id", col.ID),
				loggateway.Err(listErr))
			continue
		}
		embedder := s.embedder
		if strings.TrimSpace(col.EmbeddingModel) == "" {
			embedder = nil
		} else if embedder == nil {
			failed += len(docs)
			continue
		}
		for _, doc := range docs {
			if repaired+failed >= limit {
				break
			}
			if s.reembedOneDocument(ctx, s.uc, embedder, col, doc, 0, 0, flow) {
				repaired++
			} else {
				failed++
			}
		}
	}
	return repaired, failed, nil
}

// reembedOneDocument 单文档串行管线：DeleteChunksByDocument → indexing+WS →
// BuildIndexedChunks(content_text) → InsertChunks → indexed+WS。失败置 error 由调用方继续下一篇。
// 不触发 RebuildBlockIndex（content_text 未变，块/边不变——与 IngestDocument 的 SP1-C 钩子区分）。
func (s *KnowledgeService) reembedOneDocument(ctx context.Context, uc *biz.KnowledgeUsecase, embedder knowledge.Embedder, col biz.KnowledgeCollection, doc biz.KnowledgeDocument, chunkSize, chunkOverlap int32, flow *event.TraceEmitter) bool {
	if _, loaded := s.reembedRuns.LoadOrStore(doc.ID, struct{}{}); loaded {
		return false
	}
	defer s.reembedRuns.Delete(doc.ID)

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
		if flow != nil {
			flow.LogError("knowledge.reembed.done", "文档重嵌入失败",
				event.P("doc_id", doc.ID),
				event.P("error", err.Error()))
		}
		s.lg.Warn("knowledge reembed document failed",
			loggateway.StepID("knowledge.reembed.doc_fail"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Str("stage", stage),
			loggateway.Err(err))
	}

	if err := uc.DeleteChunksByDocument(ctx, doc.ID); err != nil {
		fail("delete_chunks", err)
		return false
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
		return false
	}
	if err := uc.InsertChunks(ctx, bizChunks); err != nil {
		fail("insert_chunks", err)
		return false
	}
	if err := uc.UpdateDocumentStatus(ctx, doc.ID, "indexed", "", len(bizChunks)); err != nil {
		s.lg.Error("failed to update document status to indexed",
			loggateway.StepID("knowledge.reembed.status_fail"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err))
		// Document was likely deleted mid-reembed; broadcast error to avoid stale UI state.
		s.publishKnowledgeIngest(col.ID, doc.ID, "error", "document deleted during reembed", 0)
		return false
	}
	s.publishKnowledgeIngest(col.ID, doc.ID, "indexed", "", len(bizChunks))
	s.lg.Info("knowledge reembed document done", // K7 每文档 done
		loggateway.StepID("knowledge.reembed.doc_done"),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Int("chunk_count", len(bizChunks)))
	return true
}
