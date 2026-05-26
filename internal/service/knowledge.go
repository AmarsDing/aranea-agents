package service

import (
	"context"
	"encoding/base64"
	"strings"

	v1 "aranea-agents/api/kratos/knowledge/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/event"
	"aranea-agents/internal/knowledge"
	"aranea-agents/pkg/safego"

	kerrors "github.com/go-kratos/kratos/v2/errors"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

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
type KnowledgeService struct {
	v1.UnimplementedKnowledgeServiceServer
	uc          *biz.KnowledgeUsecase
	chunker     *knowledge.Chunker
	embedder    *knowledge.Embedder
	retriever   *knowledge.Retriever
	bus         event.Bus
	systemSetting biz.SystemSettingRepo
}

// NewKnowledgeService constructs a KnowledgeService.
func NewKnowledgeService(uc *biz.KnowledgeUsecase, chunker *knowledge.Chunker, embedder *knowledge.Embedder, retriever *knowledge.Retriever, bus event.Bus, systemSetting biz.SystemSettingRepo) *KnowledgeService {
	return &KnowledgeService{uc: uc, chunker: chunker, embedder: embedder, retriever: retriever, bus: bus, systemSetting: systemSetting}
}

// CreateCollection creates a new vector collection.
func (s *KnowledgeService) CreateCollection(ctx context.Context, req *v1.CreateCollectionRequest) (*v1.KnowledgeCollection, error) {
	name := strings.TrimSpace(req.GetName())
	model := strings.TrimSpace(req.GetEmbeddingModel())
	if name == "" {
		return nil, kerrors.BadRequest("KNOWLEDGE", "name is required")
	}
	if model == "" {
		return nil, kerrors.BadRequest("KNOWLEDGE", "embedding_model is required")
	}
	c, err := s.uc.CreateCollection(ctx, biz.KnowledgeCollection{
		Name:           name,
		Description:    req.GetDescription(),
		EmbeddingModel: model,
	})
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
	return toProtoCollection(c), nil
}

// ListCollections returns all collections.
func (s *KnowledgeService) ListCollections(ctx context.Context, req *v1.ListCollectionsRequest) (*v1.ListCollectionsResponse, error) {
	cols, total, err := s.uc.ListCollections(ctx, "", int(req.GetLimit()), int(req.GetOffset()))
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
	return &emptypb.Empty{}, s.uc.DeleteCollection(ctx, req.GetId())
}

// IngestDocument ingests a document, chunks it, embeds the chunks, and indexes them.
// The heavy work runs asynchronously; the document record is returned immediately.
func (s *KnowledgeService) IngestDocument(ctx context.Context, req *v1.IngestDocumentRequest) (*v1.KnowledgeDocument, error) {
	raw, err := base64.StdEncoding.DecodeString(req.GetContentBase64())
	if err != nil {
		return nil, kerrors.BadRequest("KNOWLEDGE", "content_base64 is not valid base64")
	}
	if len(raw) == 0 {
		return nil, kerrors.BadRequest("KNOWLEDGE", "content is empty")
	}
	col, err := s.uc.GetCollection(ctx, req.GetCollectionId())
	if err != nil {
		return nil, err
	}
	doc, err := s.uc.CreateDocument(ctx, biz.KnowledgeDocument{
		CollectionID: req.GetCollectionId(),
		Source:       req.GetSource(),
		MimeType:     req.GetMimeType(),
		SizeBytes:    int64(len(raw)),
	})
	if err != nil {
		return nil, err
	}

	metaJSON, err := knowledge.NormalizeMetadataJSON(req.GetMetadataJson())
	if err != nil {
		return nil, kerrors.BadRequest("KNOWLEDGE", err.Error())
	}

	text, err := knowledge.ExtractDocumentText(raw, req.GetSource(), req.GetMimeType())
	if err != nil {
		return nil, kerrors.BadRequest("KNOWLEDGE", err.Error())
	}
	if strings.TrimSpace(text) == "" {
		return nil, kerrors.BadRequest("KNOWLEDGE", "document contains no extractable text")
	}

	chunkSize := int(req.GetChunkSize())
	if chunkSize <= 0 {
		chunkSize = 512
	}
	chunkOverlap := int(req.GetChunkOverlap())
	if chunkOverlap < 0 {
		chunkOverlap = 64
	}
	strategy := knowledge.ParseChunkStrategy(req.GetChunkStrategy())
	embedder := s.embedder
	uc := s.uc

	safego.Go(context.Background(), "knowledge-ingest", func() {
		bgCtx := context.Background()
		_ = uc.UpdateDocumentStatus(bgCtx, doc.ID, "indexing", "", 0)
		s.publishKnowledgeIngest(col.ID, doc.ID, "indexing", "", 0)

		bizChunks, err := knowledge.BuildIndexedChunks(bgCtx, embedder, knowledge.IngestParams{
			DocID:        doc.ID,
			CollectionID: col.ID,
			Text:         text,
			MetadataJSON: metaJSON,
			Strategy:     strategy,
			ChunkSize:    chunkSize,
			ChunkOverlap: chunkOverlap,
		})
		if err != nil {
			_ = uc.UpdateDocumentStatus(bgCtx, doc.ID, "error", err.Error(), 0)
			s.publishKnowledgeIngest(col.ID, doc.ID, "error", err.Error(), 0)
			return
		}

		if err := uc.InsertChunks(bgCtx, bizChunks); err != nil {
			_ = uc.UpdateDocumentStatus(bgCtx, doc.ID, "error", err.Error(), 0)
			s.publishKnowledgeIngest(col.ID, doc.ID, "error", err.Error(), 0)
			return
		}
		_ = uc.UpdateDocumentStatus(bgCtx, doc.ID, "indexed", "", len(bizChunks))
		_ = uc.UpdateCollectionCounts(bgCtx, col.ID, 1, len(bizChunks))
		s.publishKnowledgeIngest(col.ID, doc.ID, "indexed", "", len(bizChunks))
		knowledgeIngestTotal.Inc()
	})

	return toProtoDocument(doc), nil
}

// ListDocuments returns documents for a collection.
func (s *KnowledgeService) ListDocuments(ctx context.Context, req *v1.ListDocumentsRequest) (*v1.ListDocumentsResponse, error) {
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
	return &emptypb.Empty{}, s.uc.DeleteDocument(ctx, req.GetId())
}

// Search performs a semantic search over a collection.
func (s *KnowledgeService) Search(ctx context.Context, req *v1.SearchRequest) (*v1.SearchResponse, error) {
	timer := prometheus.NewTimer(knowledgeSearchDuration)
	defer timer.ObserveDuration()

	query := strings.TrimSpace(req.GetQuery())
	if query == "" {
		return nil, kerrors.BadRequest("KNOWLEDGE", "query is required")
	}

	if s.retriever == nil {
		return nil, kerrors.ServiceUnavailable("KNOWLEDGE", "knowledge retriever not configured")
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
	chunks, err := s.retriever.Search(ctx, q)
	if err != nil {
		return nil, kerrors.InternalServer("KNOWLEDGE", err.Error())
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
func (s *KnowledgeService) GetEmbedderConfig(ctx context.Context, _ *v1.GetEmbedderConfigRequest) (*v1.EmbedderConfig, error) {
	_ = ctx
	return s.embedderConfigProto(), nil
}

// UpdateEmbedderConfig applies runtime embedder settings from admin UI.
func (s *KnowledgeService) UpdateEmbedderConfig(ctx context.Context, req *v1.UpdateEmbedderConfigRequest) (*v1.UpdateEmbedderConfigResponse, error) {
	_ = ctx
	if s.embedder == nil {
		return nil, kerrors.InternalServer("KNOWLEDGE", "embedder not configured")
	}
	provider := strings.TrimSpace(req.GetProvider())
	if provider != "" && provider != knowledge.ProviderOpenAI && provider != knowledge.ProviderOllama &&
		provider != knowledge.ProviderGemini && provider != knowledge.ProviderHuggingFace {
		return nil, kerrors.BadRequest("KNOWLEDGE", "provider must be openai, ollama, gemini, or huggingface")
	}
	s.embedder.Update(provider, req.GetBaseUrl(), req.GetApiKey(), req.GetModel(), int(req.GetDim()))
	p, baseURL, model, dim, _, _ := s.embedder.Config()
	if err := PersistKnowledgeEmbed(ctx, s.systemSetting, p, baseURL, strings.TrimSpace(req.GetApiKey()), model, dim); err != nil {
		event.SysLogWarn("knowledge.embedder.persist", "写入 system_settings 失败",
			event.P("error", err.Error()))
	}
	return &v1.UpdateEmbedderConfigResponse{Config: s.embedderConfigProto()}, nil
}

func (s *KnowledgeService) embedderConfigProto() *v1.EmbedderConfig {
	if s.embedder == nil {
		return &v1.EmbedderConfig{}
	}
	provider, baseURL, model, dim, configured, hasAPIKey := s.embedder.Config()
	return &v1.EmbedderConfig{
		Provider:    provider,
		BaseUrl:     baseURL,
		Model:       model,
		Dim:         int32(dim),
		Configured:  configured,
		HasApiKey:   hasAPIKey,
	}
}

func (s *KnowledgeService) publishKnowledgeIngest(collectionID, docID, status, errMsg string, chunkCount int) {
	if s.bus == nil {
		return
	}
	env := event.NewEnvelope(event.EnvelopeTypeKnowledgeIngest, "knowledge", collectionID)
	env.Channel = "knowledge"
	env.Metadata = map[string]any{
		"collection_id": collectionID,
		"document_id":   docID,
		"status":        status,
		"error_message": errMsg,
		"chunk_count":   chunkCount,
	}
	s.bus.Publish(context.Background(), env)
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

// extractSupportedMimes is the set of MIME types the backend can currently
// extract and index as searchable text. Used to populate extract_supported in
// API responses so the frontend can show appropriate guidance (REV-D).
// Extend this set when new document readers are registered in trpc-agent-go.
var extractSupportedMimes = map[string]bool{
	"text/plain":              true,
	"text/markdown":           true,
	"text/csv":                true,
	"text/html":               true,
	"text/xml":                true,
	"application/json":        true,
	"application/xml":         true,
	"application/pdf":         true,
	"application/msword":      true, // .doc
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
	}
}
