package service

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/knowledge"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/appctx"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

const maxIngestBytes = 32 << 20

var allowedIngestMIMEs = map[string]bool{
	"text/plain":         true,
	"text/markdown":      true,
	"text/csv":           true,
	"text/html":          true,
	"text/xml":           true,
	"application/json":   true,
	"application/xml":    true,
	"application/pdf":    true,
	"application/msword": true,
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true,
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true,
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true,
	// Phase 9：图片经 VisionExtractor（多模态 LLM）入库。
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
}

var (
	knowledgeIngestTotal = promauto.NewCounter(prometheus.CounterOpts{
		Name: "aranea_knowledge_ingest_documents_total",
		Help: "Total documents ingested into knowledge collections.",
	})
	knowledgeSearchDuration = promauto.NewHistogram(prometheus.HistogramOpts{
		Name:    "aranea_knowledge_search_duration_seconds",
		Help:    "Duration of knowledge search requests.",
		Buckets: []float64{.005, .01, .025, .05, .1, .25, .5, 1, 2.5},
	})
)

// KnowledgeService implements kratos knowledge.v1.
type KnowledgeSearchDeps struct {
	Retriever *knowledge.Retriever
	Router    *knowledge.AdaptiveRouter
	Evaluator *knowledge.RetrievalEvaluator
	// Federated 全库智能路由（US-14 检索免选择）：Search collection_id 留空时启用。
	Federated *knowledge.FederatedRetriever
}

type KnowledgeService struct {
	v1.UnimplementedKnowledgeServiceServer
	uc            *biz.KnowledgeUsecase
	embedder      knowledge.Embedder
	embedderAdmin knowledge.EmbedderAdmin
	search        KnowledgeSearchDeps
	organizer     *knowledge.MarkdownOrganizer // LLM 整理为 MD（nil 时跳过整理）
	extractors    *knowledge.ExtractorRegistry // 模态路由（Vision 优先于 Text；nil 时退化 TextExtractor）
	assets        *knowledge.AssetStore        // 原图留存（nil 时跳过血缘）
	eventBus      biz.EventBus
	systemSetting biz.SystemSettingRepo
	lg            loggateway.Logger
}

func NewKnowledgeService(uc *biz.KnowledgeUsecase, embedder knowledge.Embedder, searchDeps KnowledgeSearchDeps, organizer *knowledge.MarkdownOrganizer, extractors *knowledge.ExtractorRegistry, assets *knowledge.AssetStore, eventBus biz.EventBus, systemSetting biz.SystemSettingRepo, lg loggateway.Logger) *KnowledgeService {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	var admin knowledge.EmbedderAdmin
	if a, ok := embedder.(knowledge.EmbedderAdmin); ok {
		admin = a
	}
	return &KnowledgeService{
		uc:            uc,
		embedder:      embedder,
		embedderAdmin: admin,
		search:        searchDeps,
		organizer:     organizer,
		extractors:    extractors,
		assets:        assets,
		eventBus:      eventBus,
		systemSetting: systemSetting,
		lg:            lg,
	}
}

// CreateCollection creates a new vector collection.
func (s *KnowledgeService) CreateCollection(ctx context.Context, req *v1.CreateCollectionRequest) (*v1.KnowledgeCollection, error) {
	name := strings.TrimSpace(req.GetName())
	model := strings.TrimSpace(req.GetEmbeddingModel())
	if name == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "name is required")
	}
	if model == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "embedding_model is required")
	}
	if s.embedderAdmin != nil {
		_, _, embedderModel, _, configured, _ := s.embedderAdmin.Config()
		if configured && embedderModel != "" && embedderModel != model {
			return nil, apierror.BadRequest("KNOWLEDGE", "embedding_model does not match current embedder model "+embedderModel)
		}
	}
	in := biz.KnowledgeCollection{
		Name:           name,
		Description:    req.GetDescription(),
		EmbeddingModel: model,
	}
	// C-01: stamp caller workspace so collections are tenant-scoped.
	// System callers create shared collections (empty workspace).
	if !workspace.IsSystem(ctx) {
		in.Workspace = workspace.IDFromContext(ctx)
	}
	c, err := s.uc.CreateCollection(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProtoCollection(c), nil
}

// GetCollection returns one collection.
func (s *KnowledgeService) GetCollection(ctx context.Context, req *v1.GetCollectionRequest) (*v1.KnowledgeCollection, error) {
	c, err := s.uc.GetCollection(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, c); err != nil {
		return nil, err
	}
	return toProtoCollection(c), nil
}

// ListCollections returns collections visible in the caller's workspace.
func (s *KnowledgeService) ListCollections(ctx context.Context, req *v1.ListCollectionsRequest) (*v1.ListCollectionsResponse, error) {
	ws := ""
	// C-01: tenant callers list only their workspace; system sees all (ws="").
	if !workspace.IsSystem(ctx) {
		ws = workspace.IDFromContext(ctx)
	}
	cols, total, err := s.uc.ListCollections(ctx, ws, int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.KnowledgeCollection, 0, len(cols))
	for _, c := range cols {
		out = append(out, toProtoCollection(c))
	}
	return &v1.ListCollectionsResponse{Items: out, Total: int32(total)}, nil
}

// DeleteCollection removes a collection.
func (s *KnowledgeService) DeleteCollection(ctx context.Context, req *v1.DeleteCollectionRequest) (*emptypb.Empty, error) {
	c, err := s.uc.GetCollection(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, c); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.DeleteCollection(ctx, req.GetId())
}

// assertCollectionAccess rejects cross-tenant collection read access (C-01).
// Returns NotFound (not Forbidden) to avoid leaking collection existence.
// Empty collection.Workspace is treated as globally shared (system/legacy) for reads.
func (s *KnowledgeService) assertCollectionAccess(ctx context.Context, c biz.KnowledgeCollection) error {
	return s.checkCollectionAccess(ctx, c, false)
}

// assertCollectionMutateAccess rejects mutate on shared/cross-tenant collections.
// Shared collections (workspace="") are read-only for tenants (fail-closed).
func (s *KnowledgeService) assertCollectionMutateAccess(ctx context.Context, c biz.KnowledgeCollection) error {
	return s.checkCollectionAccess(ctx, c, true)
}

func (s *KnowledgeService) checkCollectionAccess(ctx context.Context, c biz.KnowledgeCollection, mutate bool) error {
	callerWS := workspace.IDFromContext(ctx)
	var err error
	if mutate {
		err = workspace.AssertWorkspaceMutate(callerWS, c.Workspace)
	} else {
		err = workspace.AssertWorkspaceOrShared(callerWS, c.Workspace)
	}
	if err != nil {
		s.lg.Warn("knowledge collection access denied: workspace mismatch",
			loggateway.StepID("knowledge.idor"),
			loggateway.Str("collection_id", c.ID),
			loggateway.Str("caller_ws", callerWS),
			loggateway.Str("resource_ws", c.Workspace))
		return apierror.NotFound("KNOWLEDGE", "collection not found")
	}
	return nil
}

// IngestDocument ingests a document, chunks it, embeds the chunks, and indexes them.
// The heavy work runs asynchronously; the document record is returned immediately.
func (s *KnowledgeService) IngestDocument(ctx context.Context, req *v1.IngestDocumentRequest) (*v1.KnowledgeDocument, error) {
	raw, err := base64.StdEncoding.DecodeString(req.GetContentBase64())
	if err != nil {
		return nil, apierror.BadRequest("KNOWLEDGE", "content_base64 is not valid base64")
	}
	if len(raw) == 0 {
		return nil, apierror.BadRequest("KNOWLEDGE", "content is empty")
	}
	if len(raw) > maxIngestBytes {
		return nil, apierror.BadRequest("KNOWLEDGE", "file too large: max 32MB")
	}
	detected := http.DetectContentType(raw[:min(512, len(raw))])
	if !resolveIngestMIMEAllowed(detected, req.GetMimeType(), req.GetSource()) {
		return nil, apierror.BadRequest("KNOWLEDGE", "unsupported content type: "+detected)
	}
	col, err := s.resolveIngestCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}

	// Validate metadata BEFORE creating the document record.
	isImage := knowledge.IsImageSource(req.GetSource(), req.GetMimeType())
	modality, extractorName := "text", "text"
	if isImage {
		modality, extractorName = "image", "vision"
	}
	metaJSON, err := mergeIngestMetadata(req.GetMetadataJson(), modality, extractorName)
	if err != nil {
		return nil, apierror.BadRequest("KNOWLEDGE", err.Error())
	}

	// 文本类：同步提取（本地解析快），失败即 400 不落孤儿文档。
	// 图片：视觉 LLM 提取为耗时远程调用（最长 60s），先落文档后异步提取，
	// 失败置 status=error（NFR-12），与 indexing → indexed/error 状态流一致。
	text := ""
	organized := isImage
	if !isImage {
		var extractErr error
		text, extractErr = s.extractText(ctx, raw, req.GetSource(), req.GetMimeType())
		if extractErr != nil {
			return nil, apierror.BadRequest("KNOWLEDGE", extractErr.Error())
		}
		if strings.TrimSpace(text) == "" {
			return nil, apierror.BadRequest("KNOWLEDGE", "document contains no extractable text")
		}

		// 可选：LLM 整理为结构化 Markdown（unset 默认开启；.md 本身已是 Markdown 跳过）。
		// 失败一律降级原文本，不阻塞入库（NFR-11）。
		if s.organizer != nil && organizeToMarkdownEnabled(req) && !isMarkdownSource(req.GetSource(), req.GetMimeType()) {
			if md, ok, orgErr := s.organizer.Organize(ctx, text, req.GetSource(), req.GetMimeType()); orgErr != nil {
				s.lg.Warn("Markdown 整理异常，使用原文本继续",
					loggateway.StepID("knowledge.ingest.organize_err"),
					loggateway.Str("source", req.GetSource()),
					loggateway.Err(orgErr),
				)
			} else if ok {
				text = md
				organized = true
			}
		}
	}

	doc := biz.KnowledgeDocument{
		CollectionID: col.ID,
		Source:       req.GetSource(),
		MimeType:     req.GetMimeType(),
		SizeBytes:    int64(len(raw)),
		ContentText:  text,
		Organized:    organized,
	}
	// Phase 9：原图留存血缘（asset_uri）；asset 文件名依赖 doc ID，故提前生成。
	if isImage && s.assets != nil {
		doc.ID = uuid.NewString()
		if uri, saveErr := s.assets.Save(doc.ID, doc.Source, raw); saveErr != nil {
			s.lg.Warn("原图留存失败，跳过血缘",
				loggateway.StepID("knowledge.ingest.asset_save"),
				loggateway.Str("source", doc.Source),
				loggateway.Err(saveErr),
			)
		} else {
			doc.AssetURI = uri
		}
	}
	created, err := s.uc.CreateDocument(ctx, doc)
	if err != nil {
		return nil, err
	}
	doc = created

	strategy := knowledge.ParseChunkStrategy(req.GetChunkStrategy())
	embedder := s.embedder
	uc := s.uc

	params := knowledge.IngestParams{
		DocID:        doc.ID,
		CollectionID: col.ID,
		Text:         text,
		MetadataJSON: metaJSON,
		Strategy:     strategy,
		ChunkSize:    int(req.GetChunkSize()),
		ChunkOverlap: int(req.GetChunkOverlap()),
	}
	params.ApplyDefaults()

	ingestCtx := appctx.Ctx()
	safego.Go(ingestCtx, "knowledge-ingest", func() {
		if err := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "indexing", "", 0); err != nil {
			s.lg.Error("failed to update document status to indexing",
				loggateway.StepID("knowledge.ingest.status_fail"),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err),
			)
		}
		s.publishKnowledgeIngest(col.ID, doc.ID, "indexing", "", 0)

		// 图片：后台经 VisionExtractor 提取为 Markdown（耗时远程调用，最长 60s），
		// 成功后回写 content_text/organized（预览可用），再走统一 chunk/embed 流程；
		// 失败置 status=error（NFR-12）。
		if isImage {
			md, extractErr := s.extractors.Extract(ingestCtx, raw, doc.Source, doc.MimeType)
			if extractErr != nil || strings.TrimSpace(md) == "" {
				if extractErr == nil {
					extractErr = fmt.Errorf("vision extract %q: empty response", doc.Source)
				}
				if statusErr := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "error", extractErr.Error(), 0); statusErr != nil {
					s.lg.Error("failed to update document status to error",
						loggateway.StepID("knowledge.ingest.status_fail"),
						loggateway.Str("doc_id", doc.ID),
						loggateway.Err(statusErr),
						loggateway.Str("original_error", extractErr.Error()),
					)
				}
				s.publishKnowledgeIngest(col.ID, doc.ID, "error", extractErr.Error(), 0)
				return
			}
			if contentErr := uc.UpdateDocumentContent(ingestCtx, doc.ID, md, true); contentErr != nil {
				s.lg.Warn("图片正文回写失败，继续向量化",
					loggateway.StepID("knowledge.ingest.content_fail"),
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(contentErr),
				)
			}
			params.Text = md
		}

		bizChunks, err := knowledge.BuildIndexedChunks(ingestCtx, embedder, params)
		if err != nil {
			if statusErr := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "error", err.Error(), 0); statusErr != nil {
				s.lg.Error("failed to update document status to error",
					loggateway.StepID("knowledge.ingest.status_fail"),
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(statusErr),
					loggateway.Str("original_error", err.Error()),
				)
			}
			s.publishKnowledgeIngest(col.ID, doc.ID, "error", err.Error(), 0)
			return
		}

		if err := uc.InsertChunks(ingestCtx, bizChunks); err != nil {
			if statusErr := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "error", err.Error(), 0); statusErr != nil {
				s.lg.Error("failed to update document status to error",
					loggateway.StepID("knowledge.ingest.status_fail"),
					loggateway.Str("doc_id", doc.ID),
					loggateway.Err(statusErr),
					loggateway.Str("original_error", err.Error()),
				)
			}
			s.publishKnowledgeIngest(col.ID, doc.ID, "error", err.Error(), 0)
			return
		}
		if err := uc.UpdateDocumentStatus(ingestCtx, doc.ID, "indexed", "", len(bizChunks)); err != nil {
			s.lg.Error("failed to update document status to indexed",
				loggateway.StepID("knowledge.ingest.status_fail"),
				loggateway.Str("doc_id", doc.ID),
				loggateway.Err(err),
			)
			// Document was likely deleted mid-ingest; skip counter update to avoid drift.
			s.publishKnowledgeIngest(col.ID, doc.ID, "error", "document deleted during ingest", 0)
			return
		}
		if err := uc.UpdateCollectionCounts(ingestCtx, col.ID, 1, len(bizChunks)); err != nil {
			s.lg.Error("failed to update collection counts",
				loggateway.StepID("knowledge.ingest.counts_fail"),
				loggateway.Str("col_id", col.ID),
				loggateway.Err(err),
			)
		}
		s.publishKnowledgeIngest(col.ID, doc.ID, "indexed", "", len(bizChunks))
		knowledgeIngestTotal.Inc()
	})

	return toProtoDocument(doc), nil
}

// ListDocuments returns documents for a collection.
func (s *KnowledgeService) ListDocuments(ctx context.Context, req *v1.ListDocumentsRequest) (*v1.ListDocumentsResponse, error) {
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	docs, total, err := s.uc.ListDocuments(ctx, req.GetCollectionId(), int(req.GetLimit()), int(req.GetOffset()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.KnowledgeDocument, 0, len(docs))
	for _, d := range docs {
		out = append(out, toProtoDocument(d))
	}
	return &v1.ListDocumentsResponse{Items: out, Total: int32(total)}, nil
}

// DeleteDocument removes a document and its chunks.
func (s *KnowledgeService) DeleteDocument(ctx context.Context, req *v1.DeleteDocumentRequest) (*emptypb.Empty, error) {
	doc, err := s.uc.GetDocument(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	col, err := s.uc.GetCollection(ctx, doc.CollectionID)
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionMutateAccess(ctx, col); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, s.uc.DeleteDocument(ctx, req.GetId())
}

// GetDocumentContent returns the full extracted/organized text of one document (preview).
func (s *KnowledgeService) GetDocumentContent(ctx context.Context, req *v1.GetDocumentContentRequest) (*v1.DocumentContent, error) {
	doc, err := s.uc.GetDocument(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return &v1.DocumentContent{
		Id:          doc.ID,
		ContentText: doc.ContentText,
		Organized:   doc.Organized,
	}, nil
}

// MoveDocument moves a document (with its chunks) to another collection (US-14 整理归档).
func (s *KnowledgeService) MoveDocument(ctx context.Context, req *v1.MoveDocumentRequest) (*v1.KnowledgeDocument, error) {
	doc, err := s.uc.MoveDocument(ctx, req.GetId(), req.GetTargetCollectionId())
	if err != nil {
		return nil, err
	}
	return toProtoDocument(doc), nil
}

// Search performs a semantic search over a collection.
func (s *KnowledgeService) Search(ctx context.Context, req *v1.SearchRequest) (*v1.SearchResponse, error) {
	timer := prometheus.NewTimer(knowledgeSearchDuration)
	defer timer.ObserveDuration()

	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	if err := s.assertCollectionAccess(ctx, col); err != nil {
		return nil, err
	}
	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, apierror.BadRequest("KNOWLEDGE", "query is required")
	}

	if s.search.Retriever == nil {
		return nil, apierror.Unavailable("KNOWLEDGE", "knowledge retriever not configured")
	}
	q := biz.KnowledgeSearchQuery{
		CollectionID:     req.GetCollectionId(),
		Query:            query,
		TopK:             int(req.GetTopK()),
		MinScore:         req.GetMinScore(),
		FilterJSON:       req.GetFilterJson(),
		RerankCandidates: int(req.GetRerankCandidates()),
	}
	if req.UseRerank != nil {
		v := req.GetUseRerank()
		q.UseRerank = &v
	}

	var chunks []biz.KnowledgeChunk

	if q.CollectionID == "" {
		// US-14 检索免选择：collection_id 留空 → 全库智能路由（Route 策略，
		// 名称/描述匹配度 top N=3、阈值 0.3；无匹配自动降级 Broadcast）。
		if s.search.Federated == nil {
			return nil, apierror.Unavailable("KNOWLEDGE", "federated retriever not configured for collection-free search")
		}
		modeOverride := knowledge.ParseHybridSearchMode(req.GetHybridSearch())
		chunks, err = s.search.Federated.SearchAll(ctx, q, nil, modeOverride)
	} else if s.search.Router != nil {
		var rewriteResult *knowledge.QueryRewriteResult
		strategy := knowledge.ParseRewriteStrategy(req.GetRewriteStrategy())
		if strategy != knowledge.RewriteNone {
			rewriter := s.search.Router.QueryRewriter()
			if rewriter != nil {
				rr, rewriteErr := rewriter.Rewrite(ctx, query, strategy)
				if rewriteErr != nil {
					s.lg.Warn("query rewrite failed, using original query",
						loggateway.StepID("knowledge.search.rewrite_fail"),
						loggateway.Err(rewriteErr),
					)
				} else {
					rewriteResult = rr
				}
			}
		}
		modeOverride := knowledge.ParseHybridSearchMode(req.GetHybridSearch())
		chunks, err = s.search.Router.Search(ctx, q, rewriteResult, modeOverride)
	} else {
		chunks, err = s.search.Retriever.Search(ctx, q)
	}
	if err != nil {
		return nil, err
	}

	var assessor knowledge.ChunkAssessor
	if s.search.Evaluator != nil {
		assessor = s.search.Evaluator
	}
	chunks, err = knowledge.SearchWithEvaluation(ctx, s.search.Retriever, assessor, query, q, chunks, s.lg)
	if err != nil {
		return nil, err
	}

	out := make([]*v1.KnowledgeChunk, 0, len(chunks))
	for _, ch := range chunks {
		out = append(out, &v1.KnowledgeChunk{
			Id:           ch.ID,
			DocId:        ch.DocID,
			CollectionId: ch.CollectionID,
			Content:      ch.Content,
			MetadataJson: ch.MetadataJSON,
			ChunkIndex:   int32(ch.ChunkIndex),
			Score:        ch.Score,
		})
	}
	return &v1.SearchResponse{Chunks: out}, nil
}

// GetEmbedderConfig returns redacted embedder settings (EP-KN-01).
func (s *KnowledgeService) GetEmbedderConfig(_ context.Context, _ *v1.GetEmbedderConfigRequest) (*v1.EmbedderConfig, error) {
	return s.embedderConfigProto(), nil
}

// UpdateEmbedderConfig applies runtime embedder settings from admin UI.
func (s *KnowledgeService) UpdateEmbedderConfig(ctx context.Context, req *v1.UpdateEmbedderConfigRequest) (*v1.UpdateEmbedderConfigResponse, error) {
	if s.embedderAdmin == nil {
		return nil, apierror.Internal("KNOWLEDGE", "embedder not configured")
	}
	provider := strings.TrimSpace(req.GetProvider())
	if provider != "" && provider != knowledge.ProviderOpenAI && provider != knowledge.ProviderOllama &&
		provider != knowledge.ProviderGemini && provider != knowledge.ProviderHuggingFace {
		return nil, apierror.BadRequest("KNOWLEDGE", "provider must be openai, ollama, gemini, or huggingface")
	}
	// Persist to DB FIRST. If persistence fails we must NOT update the in-memory
	// embedder, because that would diverge the in-memory config from the DB
	// config and the new setting would be silently lost on the next restart.
	// The previous code logged a warning and returned success, leaving the
	// admin UI with a false "saved" impression.
	if err := PersistKnowledgeEmbed(ctx, s.systemSetting,
		provider, req.GetBaseUrl(), strings.TrimSpace(req.GetApiKey()), req.GetModel(), int(req.GetDim())); err != nil {
		s.lg.Error("写入 system_settings 失败 — 拒绝更新内存态以避免与 DB 不一致",
			loggateway.StepID("knowledge.embedder.persist"),
			loggateway.Err(err),
		)
		return nil, apierror.Internal("KNOWLEDGE", "failed to persist embedder config: %v", err)
	}
	// Persist succeeded — now it is safe to apply the same patch to the
	// in-memory embedder. After this, in-memory and DB state are consistent.
	s.embedderAdmin.Update(provider, req.GetBaseUrl(), req.GetApiKey(), req.GetModel(), int(req.GetDim()))
	return &v1.UpdateEmbedderConfigResponse{Config: s.embedderConfigProto()}, nil
}

func (s *KnowledgeService) embedderConfigProto() *v1.EmbedderConfig {
	if s.embedderAdmin == nil {
		return &v1.EmbedderConfig{}
	}
	provider, baseURL, model, dim, configured, hasAPIKey := s.embedderAdmin.Config()
	return &v1.EmbedderConfig{
		Provider:   provider,
		BaseUrl:    baseURL,
		Model:      model,
		Dim:        int32(dim),
		Configured: configured,
		HasApiKey:  hasAPIKey,
	}
}

// publishKnowledgeIngest publishes a knowledge ingest lifecycle event as a
// v2 SystemNoticeEvent (NOT persisted, WS-only broadcast).
func (s *KnowledgeService) publishKnowledgeIngest(collectionID, docID, status, errMsg string, chunkCount int) {
	if s.eventBus == nil {
		return
	}
	meta := map[string]any{
		"event_type":    "knowledge_ingest",
		"collection_id": collectionID,
		"document_id":   docID,
		"status":        status,
		"error_message": errMsg,
		"chunk_count":   chunkCount,
	}
	msg := "Knowledge document ingest: " + status
	s.eventBus.Publish(context.Background(), biz.NewSystemNoticeEvent("", "knowledge_ingest", msg, meta))
}

// --- proto conversion helpers ---

func toProtoCollection(c biz.KnowledgeCollection) *v1.KnowledgeCollection {
	return &v1.KnowledgeCollection{
		Id:             c.ID,
		Name:           c.Name,
		Description:    c.Description,
		EmbeddingModel: c.EmbeddingModel,
		Dim:            int32(c.Dim),
		Status:         c.Status,
		DocumentCount:  int32(c.DocumentCount),
		ChunkCount:     int32(c.ChunkCount),
		Workspace:      c.Workspace,
		CreatedAt:      c.CreatedAt,
		UpdatedAt:      c.UpdatedAt,
	}
}

// organizeToMarkdownEnabled 解析 organize_to_markdown 开关：unset 默认 true。
func organizeToMarkdownEnabled(req *v1.IngestDocumentRequest) bool {
	return req.OrganizeToMarkdown == nil || req.GetOrganizeToMarkdown()
}

// isMarkdownSource 判定来源本身已是 Markdown（无需再经 LLM 整理）。
func isMarkdownSource(source, mimeType string) bool {
	if strings.EqualFold(strings.TrimSpace(mimeType), "text/markdown") {
		return true
	}
	switch strings.ToLower(filepath.Ext(source)) {
	case ".md", ".markdown":
		return true
	}
	return false
}

func toProtoDocument(d biz.KnowledgeDocument) *v1.KnowledgeDocument {
	return &v1.KnowledgeDocument{
		Id:               d.ID,
		CollectionId:     d.CollectionID,
		Source:           d.Source,
		MimeType:         d.MimeType,
		SizeBytes:        d.SizeBytes,
		ChunkCount:       int32(d.ChunkCount),
		Status:           d.Status,
		ErrorMessage:     d.ErrorMessage,
		CreatedAt:        d.CreatedAt,
		UpdatedAt:        d.UpdatedAt,
		ExtractSupported: isExtractSupported(d.MimeType),
		Organized:        d.Organized,
	}
}

// extractSupportedMimes is the set of MIME types the backend can currently
// extract and index as searchable text. Used to populate extract_supported in
// API responses so the frontend can show appropriate guidance (REV-D).
// Extend this set when new document readers are registered in trpc-agent-go.
var extractSupportedMimes = map[string]bool{
	"text/plain":         true,
	"text/markdown":      true,
	"text/csv":           true,
	"text/html":          true,
	"text/xml":           true,
	"application/json":   true,
	"application/xml":    true,
	"application/pdf":    true,
	"application/msword": true, // .doc
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   true, // .docx
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         true, // .xlsx
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": true, // .pptx
}

// isExtractSupported returns true when the backend is expected to be able to
// extract searchable text from this MIME type. Falls back to true for text/*
// types that are not in the explicit set (they are plain UTF-8 readable).
func isExtractSupported(mimeType string) bool {
	m := strings.ToLower(strings.TrimSpace(mimeType))
	if extractSupportedMimes[m] {
		return true
	}
	return strings.HasPrefix(m, "text/")
}

func isAllowedIngestMIME(detected string) bool {
	if allowedIngestMIMEs[detected] {
		return true
	}
	return strings.HasPrefix(detected, "text/")
}

// ooxmlIngestMIMETypes 是 OOXML（DOCX/XLSX/PPTX）声明 MIME 白名单。
// 这些文件是 ZIP 容器，http.DetectContentType 返回 application/zip，
// 必须按声明 MIME 或扩展名二次判定，否则 Office 文件被白名单误拒、
// 或普通 zip 因 text/* 兜底被误放行。
var ooxmlIngestMIMETypes = map[string]struct{}{
	"application/vnd.openxmlformats-officedocument.wordprocessingml.document":   {},
	"application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":         {},
	"application/vnd.openxmlformats-officedocument.presentationml.presentation": {},
}

// resolveIngestMIMEAllowed 判定一次上传是否允许入库。
// 非 ZIP 直接走 isAllowedIngestMIME；ZIP 容器仅 OOXML 放行（声明 MIME 优先，扩展名兜底）。
func resolveIngestMIMEAllowed(detected, declared, source string) bool {
	if detected != "application/zip" {
		return isAllowedIngestMIME(detected)
	}
	if _, ok := ooxmlIngestMIMETypes[strings.ToLower(strings.TrimSpace(declared))]; ok {
		return true
	}
	switch strings.ToLower(filepath.Ext(source)) {
	case ".docx", ".xlsx", ".pptx":
		return true
	}
	return false
}

// extractText 经 ExtractorRegistry 按模态路由提取文本；
// registry 为 nil 时退化 TextExtractor（单测/兼容路径）。
func (s *KnowledgeService) extractText(ctx context.Context, raw []byte, source, mimeType string) (string, error) {
	if s.extractors != nil {
		return s.extractors.Extract(ctx, raw, source, mimeType)
	}
	return knowledge.ExtractDocumentText(raw, source, mimeType)
}

// resolveIngestCollection 解析入库目标 Collection（US-14 上传免预选）：
// collectionID 非空时按原逻辑校验存在性；为空时按当前 Embedder 配置懒创建
// 「默认知识库」并返回——用户无需在上传前手动建库。
func (s *KnowledgeService) resolveIngestCollection(ctx context.Context, collectionID string) (biz.KnowledgeCollection, error) {
	if strings.TrimSpace(collectionID) != "" {
		return s.uc.GetCollection(ctx, collectionID)
	}
	model, dim := "", 0
	if s.embedderAdmin != nil {
		_, _, m, d, _, _ := s.embedderAdmin.Config()
		model, dim = m, d
	}
	if model == "" {
		return biz.KnowledgeCollection{}, apierror.BadRequest("KNOWLEDGE", "embedder not configured; cannot ensure default collection")
	}
	return s.uc.EnsureDefaultCollection(ctx, model, dim)
}

// isImageIngest 报告本次入库是否为图片模态（委托 knowledge.IsImageSource）。
func isImageIngest(source, mimeType string) bool {
	return knowledge.IsImageSource(source, mimeType)
}

// mergeIngestMetadata 将用户 metadata 与系统模态标记（modality/extractor）
// 合并为一个 JSON 对象；用户字段与系统键冲突时以系统键为准（血缘可信）。
func mergeIngestMetadata(userJSON, modality, extractor string) (string, error) {
	merged := map[string]any{}
	if s := strings.TrimSpace(userJSON); s != "" {
		if !json.Valid([]byte(s)) {
			return "", fmt.Errorf("metadata_json must be valid JSON")
		}
		if err := json.Unmarshal([]byte(s), &merged); err != nil {
			return "", fmt.Errorf("metadata_json must be a JSON object")
		}
	}
	merged["modality"] = modality
	merged["extractor"] = extractor
	out, err := json.Marshal(merged)
	if err != nil {
		return "", err
	}
	return string(out), nil
}
