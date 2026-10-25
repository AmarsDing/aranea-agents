package service

import (
	"context"
	"strings"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

// ReembedDocuments B1：从已存 content_text 重建 chunks+embedding（无需原始文件）。
// 修复缺当前 dim 向量的 UI 上传文档（G-39：换模型不再置 NULL 作废旧 dim 向量，
// 缺口按 embedder 当前 dim 判定补齐）——vault 文档经 vault_sync 自愈不进入本链路。
// 同步返回受理计数；重嵌入在单后台 goroutine 串行执行（复用摄取管线路径，
// 不打爆 embedder API）。
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
		pending, listErr := s.uc.ListDocumentsPendingReembed(ctx, col.ID, s.currentEmbedderDim(ctx))
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
			s.reembedOneDocument(reembedCtx, uc, embedder, col, doc, req.GetChunkSize(), req.GetChunkOverlap(), req.GetChunkStrategy(), flow)
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
	_, _, model, cfgDim, configured, _ := s.embedderAdmin.Config()
	if !configured {
		return nil, apierror.BadRequest("KNOWLEDGE", "embedder not configured")
	}
	// 台账/判定口径取实测维度（G-39：新写入按 len(embedding) 落桶，台账须同
	// 源方展示可信）；探测不可行时回落配置 dim。
	dim := s.currentEmbedderDim(ctx)
	if dim <= 0 {
		dim = cfgDim
	}
	if err := s.uc.EnableCollectionSemantic(ctx, col.ID, model, dim); err != nil {
		return nil, err
	}

	// 全集合有正文文档入队（复用 B1 同一串行管线；chunks 缺失/欠当前 dim 即 pending）。
	docs, err := s.uc.ListDocumentsPendingReembed(ctx, col.ID, dim)
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
			s.reembedOneDocument(reembedCtx, uc, embedder, col, doc, 0, 0, "", flow)
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
		docs, listErr := s.uc.ListDocumentsPendingReembed(ctx, col.ID, s.currentEmbedderDim(ctx))
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
			if s.reembedOneDocument(ctx, s.uc, embedder, col, doc, 0, 0, "", flow) {
				repaired++
			} else {
				failed++
			}
		}
	}
	return repaired, failed, nil
}

// currentEmbedderDim 取 embedder 当前输出维度（G-39 重嵌判定口径来源）：
// runtime 换模型即时生效，不读集合台账；优先 ProbeDim 实测（配置 dim 与模型
// 原生维度可能分叉——后者才是新写入落桶的真实维度，二者分叉时按配置值判定
// 会让缺维腿永不命中、全集合无限重嵌）；embedder 不支持探测或探测失败回落
// Dim() 配置值；未配置 embedder 返回 0（谓词跳过缺维判定，等价词法库只查
// chunks 缺失）。
func (s *KnowledgeService) currentEmbedderDim(ctx context.Context) int {
	if s == nil || s.embedder == nil {
		return 0
	}
	if prober, ok := s.embedder.(interface {
		ProbeDim(context.Context) (int, error)
	}); ok {
		d, err := prober.ProbeDim(ctx)
		if err == nil && d > 0 {
			return d
		}
		if err != nil {
			s.lg.Warn("embedder dim probe failed; falling back to configured dim",
				loggateway.StepID("knowledge.embedder.probe_dim"),
				loggateway.Err(err))
		}
	}
	return s.embedder.Dim()
}

// reembedOneDocument 单文档串行管线：indexing+WS → BuildIndexedChunks(content_text)
// → CommitIndexedDocument（同一事务删旧插新 + indexed + 子块计数）。失败置 error 由调用方继续下一篇。
// 分块失败时旧 chunks 仍在。不触发 RebuildBlockIndex（content_text 未变）。
// 分块形状三级回退（G-06，修复重嵌恒 char+512/64 丢形状的存量 bug）：
// 显式入参（chunkStrategy/chunkSize/chunkOverlap 非空/非零）> 文档摄取落库形状
// > 包默认（char + 512/64）。req 覆盖为一次性生效，不回写文档形状列；
// parent_chunk_size 不落库，parent_child 重嵌归一为 4×chunk_size。
func (s *KnowledgeService) reembedOneDocument(ctx context.Context, uc *biz.KnowledgeUsecase, embedder knowledge.Embedder, col biz.KnowledgeCollection, doc biz.KnowledgeDocument, chunkSize, chunkOverlap int32, chunkStrategy string, flow *event.TraceEmitter) bool {
	release, err := s.acquireJob(ctx, &s.reembedRuns, doc.ID, knowledgeReembedJobKey(doc.ID),
		"reembed already running for document %s", doc.ID)
	if err != nil {
		s.lg.Warn("reembed skipped: job lock not acquired",
			loggateway.StepID("knowledge.reembed.lock"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err))
		return false
	}
	defer release()
	ctx = bizknowledge.WithDocumentContentVersion(ctx, doc.ID, doc.ContentText)

	// fail 置文档 error 终态 + WS 广播 + 流程/进程双轨错误日志（K2/K3）。
	// 旧块只在 CommitIndexedDocument 成功时被替换；分块失败可重试且不丢索引。
	fail := func(stage string, err error) {
		if apierror.IsCode(err, apierror.CodeConflict) {
			s.lg.Warn("重嵌入期间正文已更新，保留新版本", loggateway.StepID("knowledge.reembed.doc_fail"), loggateway.Err(err))
			return
		}
		if statusErr := uc.UpdateDocumentStatus(ctx, doc.ID, "error", err.Error(), doc.ChunkCount); statusErr != nil {
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

	if err := uc.UpdateDocumentStatus(ctx, doc.ID, "indexing", "", doc.ChunkCount); err != nil {
		s.lg.Error("failed to update document status to indexing",
			loggateway.StepID("knowledge.reembed.status_fail"),
			loggateway.Str("doc_id", doc.ID),
			loggateway.Err(err))
		return false
	}
	s.publishKnowledgeIngest(col.ID, doc.ID, "indexing", "", 0)

	// 形状三级回退：req 显式 > 文档摄取落库 > 包默认（ApplyDefaults）。
	strategy := knowledge.ChunkStrategy("")
	if raw := strings.TrimSpace(chunkStrategy); raw != "" {
		strategy = knowledge.ParseChunkStrategy(raw)
	} else if raw := strings.TrimSpace(doc.ChunkStrategy); raw != "" {
		strategy = knowledge.ParseChunkStrategy(raw)
	}
	size := int(chunkSize)
	if size <= 0 && doc.ChunkSize > 0 {
		size = doc.ChunkSize
	}
	overlap := int(chunkOverlap)
	if overlap <= 0 && doc.ChunkOverlap > 0 {
		overlap = doc.ChunkOverlap
	}

	params := knowledge.IngestParams{
		DocID:        doc.ID,
		CollectionID: col.ID,
		Text:         doc.ContentText,
		ChunkSize:    size,
		ChunkOverlap: overlap,
		Strategy:     strategy,
	}
	params.ApplyDefaults()
	var bizChunks []biz.KnowledgeChunk
	if strings.TrimSpace(doc.ContentText) != "" {
		bizChunks, err = knowledge.BuildIndexedChunks(ctx, embedder, params, flow)
	}
	if err != nil {
		fail("build_chunks", err)
		return false
	}
	childCount := 0
	for _, ch := range bizChunks {
		if !ch.IsParent {
			childCount++
		}
	}
	docDelta := 0
	if doc.Status != "indexed" && doc.ChunkCount == 0 {
		docDelta = 1
	}
	versionCtx := bizknowledge.WithDocumentContentVersion(ctx, doc.ID, doc.ContentText)
	commitStats, err := uc.CommitIndexedDocument(versionCtx, col.ID, doc.ID, bizChunks, docDelta)
	if err != nil {
		fail("commit_indexed", err)
		return false
	}
	// G-32 T5 决策9：diff 帧先于终态 indexed 帧。
	s.publishKnowledgeChunksDiff(col.ID, doc.ID, commitStats, "reembed")
	s.publishKnowledgeIngest(col.ID, doc.ID, "indexed", "", childCount)
	if err := uc.RebuildCurrentBlockIndex(ctx, doc.ID); err != nil {
		s.lg.Warn("重嵌入完成，链接索引等待修复", loggateway.StepID("knowledge.reembed.doc_done"), loggateway.Err(err))
	}
	s.lg.Info("knowledge reembed document done", // K7 每文档 done
		loggateway.StepID("knowledge.reembed.doc_done"),
		loggateway.Str("doc_id", doc.ID),
		loggateway.Int("chunk_count", childCount))
	return true
}
