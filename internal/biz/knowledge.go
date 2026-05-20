package biz

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
)

// KnowledgeCollection is a named vector store.
type KnowledgeCollection struct {
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

// KnowledgeDocument is one source document ingested into a collection.
type KnowledgeDocument struct {
	ID           string
	CollectionID string
	Source       string
	MimeType     string
	SizeBytes    int64
	ChunkCount   int
	Status       string
	ErrorMessage string
	CreatedAt    string
	UpdatedAt    string
}

// KnowledgeChunk is one indexed text chunk with its embedding.
type KnowledgeChunk struct {
	ID           string
	DocID        string
	CollectionID string
	Content      string
	Embedding    []float32
	MetadataJSON string
	ChunkIndex   int
	Score        float32
}

// KnowledgeSearchQuery holds search parameters.
type KnowledgeSearchQuery struct {
	CollectionID     string
	Query            string
	TopK             int
	MinScore         float32
	FilterJSON       string
	UseRerank        *bool // nil = use global reranker when configured
	RerankCandidates int   // vector search limit before rerank; 0 = default oversample
}

// KnowledgeRepo is the persistence interface for knowledge base operations.
type KnowledgeRepo interface {
	CreateCollection(ctx context.Context, c KnowledgeCollection) (KnowledgeCollection, error)
	GetCollection(ctx context.Context, id string) (KnowledgeCollection, error)
	ListCollections(ctx context.Context, workspace string, limit, offset int) ([]KnowledgeCollection, int, error)
	DeleteCollection(ctx context.Context, id string) error
	UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error

	CreateDocument(ctx context.Context, d KnowledgeDocument) (KnowledgeDocument, error)
	GetDocument(ctx context.Context, id string) (KnowledgeDocument, error)
	UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error
	ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]KnowledgeDocument, int, error)
	DeleteDocument(ctx context.Context, id string) error

	InsertChunks(ctx context.Context, chunks []KnowledgeChunk) error
	DeleteChunksByDocument(ctx context.Context, docID string) error
	SearchChunks(ctx context.Context, q KnowledgeSearchQuery, queryEmbedding []float32) ([]KnowledgeChunk, error)
}

// ErrKnowledgeUnavailable is returned when Postgres/pgvector is not configured.
var ErrKnowledgeUnavailable = errors.ServiceUnavailable(
	"KNOWLEDGE",
	"knowledge base requires PostgreSQL with pgvector; configure data.postgres.source",
)

// KnowledgeUsecase implements collection/document/search operations.
type KnowledgeUsecase struct {
	repo KnowledgeRepo
}

// NewKnowledgeUsecase constructs a KnowledgeUsecase.
func NewKnowledgeUsecase(repo KnowledgeRepo) *KnowledgeUsecase {
	return &KnowledgeUsecase{repo: repo}
}

func (u *KnowledgeUsecase) requireRepo() error {
	if u == nil || u.repo == nil {
		return ErrKnowledgeUnavailable
	}
	return nil
}

func newKnowledgeID() string {
	buf := make([]byte, 10)
	if _, err := rand.Read(buf); err != nil {
		return "kn-fallback"
	}
	return hex.EncodeToString(buf)
}

// CreateCollection validates and persists a new collection.
func (u *KnowledgeUsecase) CreateCollection(ctx context.Context, in KnowledgeCollection) (KnowledgeCollection, error) {
	if err := u.requireRepo(); err != nil {
		return KnowledgeCollection{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.EmbeddingModel = strings.TrimSpace(in.EmbeddingModel)
	if in.Name == "" {
		return KnowledgeCollection{}, errors.BadRequest("KNOWLEDGE", "name is required")
	}
	if in.EmbeddingModel == "" {
		return KnowledgeCollection{}, errors.BadRequest("KNOWLEDGE", "embedding_model is required")
	}
	if in.Dim <= 0 {
		in.Dim = 1536 // default for text-embedding-3-small
	}
	if in.ID == "" {
		in.ID = newKnowledgeID()
	}
	if in.Status == "" {
		in.Status = "active"
	}
	return u.repo.CreateCollection(ctx, in)
}

// GetCollection returns a single collection.
func (u *KnowledgeUsecase) GetCollection(ctx context.Context, id string) (KnowledgeCollection, error) {
	if err := u.requireRepo(); err != nil {
		return KnowledgeCollection{}, err
	}
	if strings.TrimSpace(id) == "" {
		return KnowledgeCollection{}, errors.BadRequest("KNOWLEDGE", "id is required")
	}
	return u.repo.GetCollection(ctx, id)
}

// ListCollections returns all collections visible in the workspace.
func (u *KnowledgeUsecase) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]KnowledgeCollection, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListCollections(ctx, workspace, limit, offset)
}

// DeleteCollection removes a collection and all its documents/chunks.
func (u *KnowledgeUsecase) DeleteCollection(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("KNOWLEDGE", "id is required")
	}
	return u.repo.DeleteCollection(ctx, id)
}

// CreateDocument records a document and returns it (status=pending).
func (u *KnowledgeUsecase) CreateDocument(ctx context.Context, d KnowledgeDocument) (KnowledgeDocument, error) {
	if err := u.requireRepo(); err != nil {
		return KnowledgeDocument{}, err
	}
	d.Source = strings.TrimSpace(d.Source)
	d.CollectionID = strings.TrimSpace(d.CollectionID)
	if d.CollectionID == "" {
		return KnowledgeDocument{}, errors.BadRequest("KNOWLEDGE", "collection_id is required")
	}
	if d.Source == "" {
		return KnowledgeDocument{}, errors.BadRequest("KNOWLEDGE", "source is required")
	}
	if d.ID == "" {
		d.ID = newKnowledgeID()
	}
	if d.Status == "" {
		d.Status = "pending"
	}
	return u.repo.CreateDocument(ctx, d)
}

// ListDocuments returns documents for a collection.
func (u *KnowledgeUsecase) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]KnowledgeDocument, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListDocuments(ctx, collectionID, limit, offset)
}

// DeleteDocument removes a document and its chunks.
func (u *KnowledgeUsecase) DeleteDocument(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("KNOWLEDGE", "id is required")
	}
	return u.repo.DeleteDocument(ctx, id)
}

// InsertChunks stores indexed chunks for a document.
func (u *KnowledgeUsecase) InsertChunks(ctx context.Context, chunks []KnowledgeChunk) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	return u.repo.InsertChunks(ctx, chunks)
}

// Search performs a vector similarity search.
func (u *KnowledgeUsecase) Search(ctx context.Context, q KnowledgeSearchQuery, queryEmbedding []float32) ([]KnowledgeChunk, error) {
	if err := u.requireRepo(); err != nil {
		return nil, err
	}
	if strings.TrimSpace(q.CollectionID) == "" {
		return nil, errors.BadRequest("KNOWLEDGE", "collection_id is required")
	}
	if strings.TrimSpace(q.Query) == "" {
		return nil, errors.BadRequest("KNOWLEDGE", "query is required")
	}
	if q.TopK <= 0 {
		q.TopK = 5
	}
	return u.repo.SearchChunks(ctx, q, queryEmbedding)
}

// UpdateDocumentStatus marks a document's indexing state.
func (u *KnowledgeUsecase) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.repo.UpdateDocumentStatus(ctx, id, status, errMsg, chunkCount)
}

// UpdateCollectionCounts adjusts document/chunk tallies on a collection.
func (u *KnowledgeUsecase) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.repo.UpdateCollectionCounts(ctx, id, docDelta, chunkDelta)
}
