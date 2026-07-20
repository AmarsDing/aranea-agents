// Package knowledge implements knowledge base collection/document/search workflows.
package knowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"aranea-agents/pkg/apierror"
)

// Collection is a named vector store.
type Collection struct {
	ID             string
	Name           string
	Description    string
	EmbeddingModel string
	Dim            int
	Status         string
	DocumentCount  int
	ChunkCount     int
	Workspace      string
	CreatedAt      string
	UpdatedAt      string
}

// Document is one source document ingested into a collection.
type Document struct {
	ID           string
	CollectionID string
	Source       string
	MimeType     string
	SizeBytes    int64
	ChunkCount   int
	Status       string
	ErrorMessage string
	// ContentText 是提取/整理后的全文（organized=true 时为结构化 Markdown），供预览与血缘。
	ContentText string
	// Organized 标记内容是否经 LLM 整理为 Markdown。
	Organized bool
	// AssetURI 是原始文件留存路径（Phase 9 多模态血缘），文本类文档为空。
	AssetURI    string
	CreatedAt   string
	UpdatedAt   string
}

// Chunk is one indexed text chunk with its embedding.
type Chunk struct {
	ID           string
	DocID        string
	CollectionID string
	Content      string
	Embedding    []float32
	MetadataJSON string
	ChunkIndex   int
	Score        float32
}

// SearchQuery holds search parameters.
type SearchQuery struct {
	CollectionID     string
	Query            string
	TopK             int
	MinScore         float32
	FilterJSON       string
	UseRerank        *bool
	RerankCandidates int
}

// Repo is the persistence interface for knowledge base operations.
type CollectionRepo interface {
	CreateCollection(ctx context.Context, c Collection) (Collection, error)
	GetCollection(ctx context.Context, id string) (Collection, error)
	ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error)
	DeleteCollection(ctx context.Context, id string) error
	UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error
}

type DocumentRepo interface {
	CreateDocument(ctx context.Context, d Document) (Document, error)
	GetDocument(ctx context.Context, id string) (Document, error)
	UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error
	ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
	DeleteDocument(ctx context.Context, id string) error
}

type ChunkRepo interface {
	InsertChunks(ctx context.Context, chunks []Chunk) error
	DeleteChunksByDocument(ctx context.Context, docID string) error
	SearchChunks(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error)
}

type Repo interface {
	CollectionRepo
	DocumentRepo
	ChunkRepo
}

// SparseSearcher is the interface for BM25/full-text search over knowledge chunks.
type SparseSearcher interface {
	SearchChunksBM25(ctx context.Context, q SearchQuery) ([]Chunk, error)
}

// KnowledgeEmbedder generates text embeddings using a remote API and exposes
// runtime configuration for the admin UI. The concrete HTTP implementation
// lives in the data layer; biz depends only on this interface.
type KnowledgeEmbedder interface {
	// Embed returns a single embedding vector for the input text.
	Embed(ctx context.Context, text string) ([]float32, error)
	// EmbedWithTaskType returns a single embedding with a task type hint (e.g. "RETRIEVAL_QUERY").
	EmbedWithTaskType(ctx context.Context, text string, taskType string) ([]float32, error)
	// EmbedBatch returns embeddings for a slice of texts using provider batch APIs when available.
	EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
	// EmbedBatchWithTaskType returns embeddings with an optional task type hint.
	EmbedBatchWithTaskType(ctx context.Context, texts []string, taskType string) ([][]float32, error)
	// Config returns a redacted view of embedder settings.
	Config() (provider, baseURL, model string, dim int, configured bool, hasAPIKey bool)
	// Update applies runtime embedder settings from admin UI.
	Update(provider, baseURL, apiKey, model string, dim int)
}

// Domain errors for knowledge biz layer — the Service layer maps these to apierror.
var (
	ErrUnavailable            = apierror.Internal("KNOWLEDGE", "knowledge base requires PostgreSQL with pgvector; configure data.postgres.source")
	ErrNameRequired           = apierror.BadRequest("KNOWLEDGE", "name is required")
	ErrEmbeddingModelRequired = apierror.BadRequest("KNOWLEDGE", "embedding_model is required")
	ErrIDRequired             = apierror.BadRequest("KNOWLEDGE", "id is required")
	ErrCollectionIDRequired   = apierror.BadRequest("KNOWLEDGE", "collection_id is required")
	ErrSourceRequired         = apierror.BadRequest("KNOWLEDGE", "source is required")
	ErrQueryRequired          = apierror.BadRequest("KNOWLEDGE", "query is required")
	ErrDimensionMismatch      = apierror.BadRequest("KNOWLEDGE", "embedding dimension mismatch")
	ErrEmbeddingEmpty         = apierror.BadRequest("KNOWLEDGE", "embedding is empty")
)

// Usecase implements collection/document/search operations.
type Usecase struct {
	collections CollectionRepo
	documents   DocumentRepo
	chunks      ChunkRepo
}

// NewUsecase constructs a KnowledgeUsecase from individual sub-interfaces.
func NewUsecase(collections CollectionRepo, documents DocumentRepo, chunks ChunkRepo) *Usecase {
	return &Usecase{collections: collections, documents: documents, chunks: chunks}
}

// NewUsecaseFromRepo constructs a KnowledgeUsecase from the combined Repo interface.
func NewUsecaseFromRepo(repo Repo) *Usecase {
	return &Usecase{collections: repo, documents: repo, chunks: repo}
}

func (u *Usecase) requireRepo() error {
	if u == nil || u.collections == nil || u.documents == nil || u.chunks == nil {
		return ErrUnavailable
	}
	return nil
}

// IsUnavailable reports whether the knowledge backend (Postgres/pgvector) is not configured.
// When true, knowledge_search/knowledge_reflect tools should not be registered for agents.
func (u *Usecase) IsUnavailable() bool {
	return u == nil || u.collections == nil || u.documents == nil || u.chunks == nil
}

func newKnowledgeID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "kn-fallback"
	}
	return hex.EncodeToString(buf)
}

// CreateCollection validates and persists a new collection.
func (u *Usecase) CreateCollection(ctx context.Context, in Collection) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.EmbeddingModel = strings.TrimSpace(in.EmbeddingModel)
	if in.Name == "" {
		return Collection{}, ErrNameRequired
	}
	if in.EmbeddingModel == "" {
		return Collection{}, ErrEmbeddingModelRequired
	}
	if in.Dim <= 0 {
		in.Dim = 1536
	}
	if in.ID == "" {
		in.ID = newKnowledgeID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	return u.collections.CreateCollection(ctx, in)
}

// GetCollection returns a single collection.
func (u *Usecase) GetCollection(ctx context.Context, id string) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Collection{}, ErrIDRequired
	}
	return u.collections.GetCollection(ctx, id)
}

// ListCollections returns all collections visible in the workspace.
func (u *Usecase) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.collections.ListCollections(ctx, workspace, limit, offset)
}

// DeleteCollection removes a collection and all its documents/chunks.
func (u *Usecase) DeleteCollection(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return ErrIDRequired
	}
	return u.collections.DeleteCollection(ctx, id)
}

// CreateDocument records a document and returns it (status=pending).
func (u *Usecase) CreateDocument(ctx context.Context, d Document) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	d.Source = strings.TrimSpace(d.Source)
	d.CollectionID = strings.TrimSpace(d.CollectionID)
	if d.CollectionID == "" {
		return Document{}, ErrCollectionIDRequired
	}
	if d.Source == "" {
		return Document{}, ErrSourceRequired
	}
	if d.ID == "" {
		d.ID = newKnowledgeID()
	}
	if d.Status == "" {
		d.Status = "pending"
	}
	return u.documents.CreateDocument(ctx, d)
}

// ListDocuments returns documents for a collection.
func (u *Usecase) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.documents.ListDocuments(ctx, collectionID, limit, offset)
}

// GetDocument returns a single document including its extracted/organized content.
func (u *Usecase) GetDocument(ctx context.Context, id string) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Document{}, ErrIDRequired
	}
	return u.documents.GetDocument(ctx, id)
}

// DeleteDocument removes a document and its chunks. Repo implementations MUST
// keep `knowledge_collections.document_count / chunk_count` in sync atomically.
// DAT-02 / KB-04: prior repo implementations only deleted the document row
// (relying on FK cascade for chunks) but never decremented the collection tally.
func (u *Usecase) DeleteDocument(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return ErrIDRequired
	}
	return u.documents.DeleteDocument(ctx, id)
}

// InsertChunks stores indexed chunks for a document.
func (u *Usecase) InsertChunks(ctx context.Context, chunks []Chunk) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	if err := u.chunks.InsertChunks(ctx, chunks); err != nil {
		if strings.Contains(err.Error(), "dimension mismatch") {
			return ErrDimensionMismatch
		}
		return apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	return nil
}

// Search performs a vector similarity search.
func (u *Usecase) Search(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(q.CollectionID) == "" {
		return nil, ErrCollectionIDRequired
	}
	if strings.TrimSpace(q.Query) == "" {
		return nil, ErrQueryRequired
	}
	if q.TopK <= 0 {
		q.TopK = 5
	}
	chunks, err := u.chunks.SearchChunks(ctx, q, queryEmbedding)
	if err != nil {
		if strings.Contains(err.Error(), "embedding is empty") {
			return nil, ErrEmbeddingEmpty
		}
		return nil, apierror.Wrap(err, apierror.CodeInternal, apierror.DomainKnowledge)
	}
	return chunks, nil
}

// UpdateDocumentStatus marks a document's indexing state.
func (u *Usecase) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.documents.UpdateDocumentStatus(ctx, id, status, errMsg, chunkCount)
}

// UpdateCollectionCounts adjusts document/chunk tallies on a collection.
func (u *Usecase) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.collections.UpdateCollectionCounts(ctx, id, docDelta, chunkDelta)
}

// ── Embed settings ────────────────────────────────────────────────────────────

// EmbedSetting is persisted platform default for the knowledge embedder.
type EmbedSetting struct {
	Provider  string
	BaseURL   string
	Model     string
	Dim       int
	APIKey    string
	HasAPIKey bool
}

// EmbedConfigured reports whether stored settings are sufficient for the provider.
func EmbedConfigured(s EmbedSetting) bool {
	p := strings.TrimSpace(s.Provider)
	if p == "" {
		return false
	}
	switch p {
	case "ollama":
		return true
	case "huggingface":
		return strings.TrimSpace(s.BaseURL) != ""
	default:
		return s.HasAPIKey
	}
}

// ApplyEmbedPatch merges an update onto current settings.
func ApplyEmbedPatch(cur EmbedSetting, provider, baseURL, apiKey, model string, dim int, updateAPIKey bool) EmbedSetting {
	out := cur
	if p := strings.TrimSpace(provider); p != "" {
		out.Provider = p
	}
	out.BaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	out.Model = strings.TrimSpace(model)
	if dim > 0 {
		out.Dim = dim
	}
	if updateAPIKey && strings.TrimSpace(apiKey) != "" {
		out.APIKey = strings.TrimSpace(apiKey)
		out.HasAPIKey = true
	}
	return out
}
