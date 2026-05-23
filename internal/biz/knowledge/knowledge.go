// Package knowledge implements knowledge base collection/document/search workflows.
package knowledge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"strings"

	"github.com/go-kratos/kratos/v2/errors"
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
	CreatedAt    string
	UpdatedAt    string
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
type Repo interface {
	CreateCollection(ctx context.Context, c Collection) (Collection, error)
	GetCollection(ctx context.Context, id string) (Collection, error)
	ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error)
	DeleteCollection(ctx context.Context, id string) error
	UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error

	CreateDocument(ctx context.Context, d Document) (Document, error)
	GetDocument(ctx context.Context, id string) (Document, error)
	UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error
	ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
	DeleteDocument(ctx context.Context, id string) error

	InsertChunks(ctx context.Context, chunks []Chunk) error
	DeleteChunksByDocument(ctx context.Context, docID string) error
	SearchChunks(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error)
}

// ErrUnavailable is returned when Postgres/pgvector is not configured.
var ErrUnavailable = errors.ServiceUnavailable(
	"KNOWLEDGE",
	"knowledge base requires PostgreSQL with pgvector; configure data.postgres.source",
)

// Usecase implements collection/document/search operations.
type Usecase struct {
	repo Repo
}

// NewUsecase constructs a KnowledgeUsecase.
func NewUsecase(repo Repo) *Usecase {
	return &Usecase{repo: repo}
}

func (u *Usecase) requireRepo() error {
	if u == nil || u.repo == nil {
		return ErrUnavailable
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
func (u *Usecase) CreateCollection(ctx context.Context, in Collection) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	in.Name = strings.TrimSpace(in.Name)
	in.EmbeddingModel = strings.TrimSpace(in.EmbeddingModel)
	if in.Name == "" {
		return Collection{}, errors.BadRequest("KNOWLEDGE", "name is required")
	}
	if in.EmbeddingModel == "" {
		return Collection{}, errors.BadRequest("KNOWLEDGE", "embedding_model is required")
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
	return u.repo.CreateCollection(ctx, in)
}

// GetCollection returns a single collection.
func (u *Usecase) GetCollection(ctx context.Context, id string) (Collection, error) {
	if err := u.requireRepo(); err != nil {
		return Collection{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Collection{}, errors.BadRequest("KNOWLEDGE", "id is required")
	}
	return u.repo.GetCollection(ctx, id)
}

// ListCollections returns all collections visible in the workspace.
func (u *Usecase) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListCollections(ctx, workspace, limit, offset)
}

// DeleteCollection removes a collection and all its documents/chunks.
func (u *Usecase) DeleteCollection(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("KNOWLEDGE", "id is required")
	}
	return u.repo.DeleteCollection(ctx, id)
}

// CreateDocument records a document and returns it (status=pending).
func (u *Usecase) CreateDocument(ctx context.Context, d Document) (Document, error) {
	if err := u.requireRepo(); err != nil {
		return Document{}, err
	}
	d.Source = strings.TrimSpace(d.Source)
	d.CollectionID = strings.TrimSpace(d.CollectionID)
	if d.CollectionID == "" {
		return Document{}, errors.BadRequest("KNOWLEDGE", "collection_id is required")
	}
	if d.Source == "" {
		return Document{}, errors.BadRequest("KNOWLEDGE", "source is required")
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
func (u *Usecase) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error) {
	if err := u.requireRepo(); err != nil {
		return nil, 0, err
	}
	if limit <= 0 {
		limit = 20
	}
	return u.repo.ListDocuments(ctx, collectionID, limit, offset)
}

// DeleteDocument removes a document and its chunks.
func (u *Usecase) DeleteDocument(ctx context.Context, id string) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if strings.TrimSpace(id) == "" {
		return errors.BadRequest("KNOWLEDGE", "id is required")
	}
	return u.repo.DeleteDocument(ctx, id)
}

// InsertChunks stores indexed chunks for a document.
func (u *Usecase) InsertChunks(ctx context.Context, chunks []Chunk) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	if len(chunks) == 0 {
		return nil
	}
	return u.repo.InsertChunks(ctx, chunks)
}

// Search performs a vector similarity search.
func (u *Usecase) Search(ctx context.Context, q SearchQuery, queryEmbedding []float32) ([]Chunk, error) {
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
func (u *Usecase) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.repo.UpdateDocumentStatus(ctx, id, status, errMsg, chunkCount)
}

// UpdateCollectionCounts adjusts document/chunk tallies on a collection.
func (u *Usecase) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error {
	if err := u.requireRepo(); err != nil {
		return err
	}
	return u.repo.UpdateCollectionCounts(ctx, id, docDelta, chunkDelta)
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
	if b := strings.TrimRight(strings.TrimSpace(baseURL), "/"); b != "" {
		out.BaseURL = b
	}
	if m := strings.TrimSpace(model); m != "" {
		out.Model = m
	}
	if dim > 0 {
		out.Dim = dim
	}
	if updateAPIKey && strings.TrimSpace(apiKey) != "" {
		out.APIKey = strings.TrimSpace(apiKey)
		out.HasAPIKey = true
	}
	return out
}
