package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"
)

type mockRepo struct {
	collCreateFn  func(ctx context.Context, c Collection) (Collection, error)
	collGetFn     func(ctx context.Context, id string) (Collection, error)
	collListFn    func(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error)
	collDeleteFn  func(ctx context.Context, id string) error
	collUpdateFn  func(ctx context.Context, id string, docDelta, chunkDelta int) error
	collSyncFn    func(ctx context.Context, id, state string, lastSyncAt time.Time) error
	docCreateFn   func(ctx context.Context, d Document) (Document, error)
	docGetFn      func(ctx context.Context, id string) (Document, error)
	docGetByRelFn func(ctx context.Context, collectionID, relPath string) (Document, error)
	docRelPathFn  func(ctx context.Context, id, newRelPath string) error
	docSyncMetaFn func(ctx context.Context, id string, meta DocumentSyncMeta) error
	docUpdateFn   func(ctx context.Context, id, status, errMsg string, chunkCount int) error
	docContentFn  func(ctx context.Context, id, contentText string, organized bool) error
	docListFn     func(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error)
	docDeleteFn   func(ctx context.Context, id string) error
	docMoveFn     func(ctx context.Context, id, targetCollectionID string) (Document, error)
	chunkInsertFn func(ctx context.Context, chunks []Chunk) error
	chunkDeleteFn func(ctx context.Context, docID string) error
	chunkSearchFn func(ctx context.Context, q SearchQuery, emb []float32) ([]Chunk, error)
}

func (m *mockRepo) CreateCollection(ctx context.Context, c Collection) (Collection, error) {
	return m.collCreateFn(ctx, c)
}
func (m *mockRepo) GetCollection(ctx context.Context, id string) (Collection, error) {
	return m.collGetFn(ctx, id)
}
func (m *mockRepo) ListCollections(ctx context.Context, workspace string, limit, offset int) ([]Collection, int, error) {
	return m.collListFn(ctx, workspace, limit, offset)
}
func (m *mockRepo) DeleteCollection(ctx context.Context, id string) error {
	return m.collDeleteFn(ctx, id)
}
func (m *mockRepo) UpdateCollectionCounts(ctx context.Context, id string, docDelta, chunkDelta int) error {
	return m.collUpdateFn(ctx, id, docDelta, chunkDelta)
}
func (m *mockRepo) UpdateCollectionSyncState(ctx context.Context, id, state string, lastSyncAt time.Time) error {
	return m.collSyncFn(ctx, id, state, lastSyncAt)
}
func (m *mockRepo) CreateDocument(ctx context.Context, d Document) (Document, error) {
	return m.docCreateFn(ctx, d)
}
func (m *mockRepo) GetDocument(ctx context.Context, id string) (Document, error) {
	return m.docGetFn(ctx, id)
}
func (m *mockRepo) GetDocumentByRelPath(ctx context.Context, collectionID, relPath string) (Document, error) {
	return m.docGetByRelFn(ctx, collectionID, relPath)
}
func (m *mockRepo) UpdateDocumentRelPath(ctx context.Context, id, newRelPath string) error {
	return m.docRelPathFn(ctx, id, newRelPath)
}
func (m *mockRepo) UpdateDocumentSyncMeta(ctx context.Context, id string, meta DocumentSyncMeta) error {
	return m.docSyncMetaFn(ctx, id, meta)
}
func (m *mockRepo) UpdateDocumentStatus(ctx context.Context, id, status, errMsg string, chunkCount int) error {
	return m.docUpdateFn(ctx, id, status, errMsg, chunkCount)
}
func (m *mockRepo) UpdateDocumentContent(ctx context.Context, id, contentText string, organized bool) error {
	return m.docContentFn(ctx, id, contentText, organized)
}
func (m *mockRepo) ListDocuments(ctx context.Context, collectionID string, limit, offset int) ([]Document, int, error) {
	return m.docListFn(ctx, collectionID, limit, offset)
}
func (m *mockRepo) DeleteDocument(ctx context.Context, id string) error {
	return m.docDeleteFn(ctx, id)
}
func (m *mockRepo) MoveDocument(ctx context.Context, id, targetCollectionID string) (Document, error) {
	return m.docMoveFn(ctx, id, targetCollectionID)
}
func (m *mockRepo) InsertChunks(ctx context.Context, chunks []Chunk) error {
	return m.chunkInsertFn(ctx, chunks)
}
func (m *mockRepo) DeleteChunksByDocument(ctx context.Context, docID string) error {
	return m.chunkDeleteFn(ctx, docID)
}
func (m *mockRepo) SearchChunks(ctx context.Context, q SearchQuery, emb []float32) ([]Chunk, error) {
	return m.chunkSearchFn(ctx, q, emb)
}

func noOpMockRepo() *mockRepo {
	return &mockRepo{
		collCreateFn: func(_ context.Context, c Collection) (Collection, error) { return c, nil },
		collGetFn:    func(_ context.Context, id string) (Collection, error) { return Collection{ID: id}, nil },
		collListFn:   func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) { return nil, 0, nil },
		collDeleteFn: func(_ context.Context, _ string) error { return nil },
		collUpdateFn: func(_ context.Context, _ string, _, _ int) error { return nil },
		collSyncFn:   func(_ context.Context, _, _ string, _ time.Time) error { return nil },
		docCreateFn:  func(_ context.Context, d Document) (Document, error) { return d, nil },
		docGetFn:     func(_ context.Context, id string) (Document, error) { return Document{ID: id}, nil },
		docGetByRelFn: func(_ context.Context, _, relPath string) (Document, error) {
			return Document{RelPath: relPath}, nil
		},
		docRelPathFn:  func(_ context.Context, _, _ string) error { return nil },
		docSyncMetaFn: func(_ context.Context, _ string, _ DocumentSyncMeta) error { return nil },
		docUpdateFn:   func(_ context.Context, _, _, _ string, _ int) error { return nil },
		docContentFn:  func(_ context.Context, _, _ string, _ bool) error { return nil },
		docListFn:     func(_ context.Context, _ string, _, _ int) ([]Document, int, error) { return nil, 0, nil },
		docDeleteFn:   func(_ context.Context, _ string) error { return nil },
		docMoveFn: func(_ context.Context, id, target string) (Document, error) {
			return Document{ID: id, CollectionID: target}, nil
		},
		chunkInsertFn: func(_ context.Context, _ []Chunk) error { return nil },
		chunkDeleteFn: func(_ context.Context, _ string) error { return nil },
		chunkSearchFn: func(_ context.Context, _ SearchQuery, _ []float32) ([]Chunk, error) { return nil, nil },
	}
}

func TestUsecase_CreateCollection(t *testing.T) {
	tests := []struct {
		name      string
		in        Collection
		repoFn    func(_ context.Context, c Collection) (Collection, error)
		wantErr   bool
		wantErrIs error
		check     func(t *testing.T, got Collection)
	}{
		{
			"empty name rejected",
			Collection{EmbeddingModel: "text-embedding-ada-002"},
			nil, true, ErrNameRequired, nil,
		},
		{
			"whitespace name rejected",
			Collection{Name: "  ", EmbeddingModel: "text-embedding-ada-002"},
			nil, true, ErrNameRequired, nil,
		},
		{
			"empty embedding model rejected",
			Collection{Name: "test"},
			nil, true, ErrEmbeddingModelRequired, nil,
		},
		{
			"whitespace embedding model rejected",
			Collection{Name: "test", EmbeddingModel: "  "},
			nil, true, ErrEmbeddingModelRequired, nil,
		},
		{
			"defaults applied dim and status",
			Collection{Name: "test", EmbeddingModel: "text-embedding-ada-002"},
			func(_ context.Context, c Collection) (Collection, error) { return c, nil },
			false, nil,
			func(t *testing.T, got Collection) {
				if got.Dim != 1536 {
					t.Errorf("Dim = %d, want 1536", got.Dim)
				}
				if got.Status != "active" {
					t.Errorf("Status = %q, want %q", got.Status, "active")
				}
				if got.ID == "" {
					t.Error("ID should be auto-generated")
				}
			},
		},
		{
			"explicit dim preserved",
			Collection{Name: "test", EmbeddingModel: "m", Dim: 768, Status: "inactive", ID: "custom-id"},
			func(_ context.Context, c Collection) (Collection, error) { return c, nil },
			false, nil,
			func(t *testing.T, got Collection) {
				if got.Dim != 768 {
					t.Errorf("Dim = %d, want 768", got.Dim)
				}
				if got.Status != "inactive" {
					t.Errorf("Status = %q, want %q", got.Status, "inactive")
				}
				if got.ID != "custom-id" {
					t.Errorf("ID = %q, want %q", got.ID, "custom-id")
				}
			},
		},
		{
			"repo error propagated",
			Collection{Name: "test", EmbeddingModel: "m"},
			func(_ context.Context, _ Collection) (Collection, error) {
				return Collection{}, fmt.Errorf("knowledge: db error")
			},
			true, nil, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.collCreateFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			got, err := u.CreateCollection(context.Background(), tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tt.wantErrIs)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestUsecase_GetCollection(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		repoFn    func(_ context.Context, id string) (Collection, error)
		wantErr   bool
		wantErrIs error
	}{
		{
			"empty id rejected",
			"",
			nil, true, ErrIDRequired,
		},
		{
			"whitespace id rejected",
			"  ",
			nil, true, ErrIDRequired,
		},
		{
			"valid id passed through",
			"col-123",
			func(_ context.Context, id string) (Collection, error) {
				return Collection{ID: id}, nil
			},
			false, nil,
		},
		{
			"repo not found",
			"col-missing",
			func(_ context.Context, _ string) (Collection, error) {
				return Collection{}, fmt.Errorf("not found")
			},
			true, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.collGetFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			got, err := u.GetCollection(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err == nil && tt.id != "" && tt.id != "  " && got.ID != tt.id {
				t.Errorf("ID = %q, want %q", got.ID, tt.id)
			}
			if err != nil && tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tt.wantErrIs)
				}
			}
		})
	}
}

func TestUsecase_ListCollections(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		offset    int
		wantLimit int
	}{
		{"zero limit defaults to 20", 0, 0, 20},
		{"negative limit defaults to 20", -1, 0, 20},
		{"positive limit preserved", 50, 10, 50},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedLimit int
			mr := noOpMockRepo()
			mr.collListFn = func(_ context.Context, _ string, limit, _ int) ([]Collection, int, error) {
				capturedLimit = limit
				return nil, 0, nil
			}
			u := NewUsecaseFromRepo(mr)
			_, _, err := u.ListCollections(context.Background(), "ws", tt.limit, tt.offset)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if capturedLimit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", capturedLimit, tt.wantLimit)
			}
		})
	}
}

func TestUsecase_DeleteCollection(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		wantErr   bool
		wantErrIs error
	}{
		{"empty id rejected", "", true, ErrIDRequired},
		{"whitespace id rejected", "  ", true, ErrIDRequired},
		{"valid id passes", "col-1", false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			u := NewUsecaseFromRepo(mr)
			err := u.DeleteCollection(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tt.wantErrIs)
				}
			}
		})
	}
}

func TestUsecase_CreateDocument(t *testing.T) {
	tests := []struct {
		name      string
		in        Document
		repoFn    func(_ context.Context, d Document) (Document, error)
		wantErr   bool
		wantErrIs error
		check     func(t *testing.T, got Document)
	}{
		{
			"empty collection id rejected",
			Document{Source: "file.txt"},
			nil, true, ErrCollectionIDRequired, nil,
		},
		{
			"whitespace collection id rejected",
			Document{CollectionID: "  ", Source: "file.txt"},
			nil, true, ErrCollectionIDRequired, nil,
		},
		{
			"empty source rejected",
			Document{CollectionID: "col-1"},
			nil, true, ErrSourceRequired, nil,
		},
		{
			"whitespace source rejected",
			Document{CollectionID: "col-1", Source: "  "},
			nil, true, ErrSourceRequired, nil,
		},
		{
			"defaults applied id and status",
			Document{CollectionID: "col-1", Source: "file.txt"},
			func(_ context.Context, d Document) (Document, error) { return d, nil },
			false, nil,
			func(t *testing.T, got Document) {
				if got.ID == "" {
					t.Error("ID should be auto-generated")
				}
				if got.Status != "pending" {
					t.Errorf("Status = %q, want %q", got.Status, "pending")
				}
			},
		},
		{
			"explicit id and status preserved",
			Document{ID: "doc-1", CollectionID: "col-1", Source: "file.txt", Status: "ready"},
			func(_ context.Context, d Document) (Document, error) { return d, nil },
			false, nil,
			func(t *testing.T, got Document) {
				if got.ID != "doc-1" {
					t.Errorf("ID = %q, want %q", got.ID, "doc-1")
				}
				if got.Status != "ready" {
					t.Errorf("Status = %q, want %q", got.Status, "ready")
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.docCreateFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			got, err := u.CreateDocument(context.Background(), tt.in)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tt.wantErrIs)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestUsecase_ListDocuments(t *testing.T) {
	tests := []struct {
		name      string
		limit     int
		wantLimit int
	}{
		{"zero limit defaults to 20", 0, 20},
		{"negative limit defaults to 20", -5, 20},
		{"positive limit preserved", 10, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedLimit int
			mr := noOpMockRepo()
			mr.docListFn = func(_ context.Context, _ string, limit, _ int) ([]Document, int, error) {
				capturedLimit = limit
				return nil, 0, nil
			}
			u := NewUsecaseFromRepo(mr)
			_, _, err := u.ListDocuments(context.Background(), "col-1", tt.limit, 0)
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if capturedLimit != tt.wantLimit {
				t.Errorf("limit = %d, want %d", capturedLimit, tt.wantLimit)
			}
		})
	}
}

func TestUsecase_Search(t *testing.T) {
	tests := []struct {
		name      string
		q         SearchQuery
		wantErr   bool
		wantErrIs error
		check     func(t *testing.T, q SearchQuery)
	}{
		{
			"empty collection id rejected",
			SearchQuery{Query: "hello"},
			true, ErrCollectionIDRequired, nil,
		},
		{
			"whitespace collection id rejected",
			SearchQuery{CollectionID: "  ", Query: "hello"},
			true, ErrCollectionIDRequired, nil,
		},
		{
			"empty query rejected",
			SearchQuery{CollectionID: "col-1"},
			true, ErrQueryRequired, nil,
		},
		{
			"whitespace query rejected",
			SearchQuery{CollectionID: "col-1", Query: "  "},
			true, ErrQueryRequired, nil,
		},
		{
			"zero topk defaults to 5",
			SearchQuery{CollectionID: "col-1", Query: "hello"},
			false, nil,
			func(t *testing.T, q SearchQuery) {
				if q.TopK != 5 {
					t.Errorf("TopK = %d, want 5", q.TopK)
				}
			},
		},
		{
			"negative topk defaults to 5",
			SearchQuery{CollectionID: "col-1", Query: "hello", TopK: -1},
			false, nil,
			func(t *testing.T, q SearchQuery) {
				if q.TopK != 5 {
					t.Errorf("TopK = %d, want 5", q.TopK)
				}
			},
		},
		{
			"explicit topk preserved",
			SearchQuery{CollectionID: "col-1", Query: "hello", TopK: 10},
			false, nil,
			func(t *testing.T, q SearchQuery) {
				if q.TopK != 10 {
					t.Errorf("TopK = %d, want 10", q.TopK)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var capturedQ SearchQuery
			mr := noOpMockRepo()
			mr.chunkSearchFn = func(_ context.Context, q SearchQuery, _ []float32) ([]Chunk, error) {
				capturedQ = q
				return nil, nil
			}
			u := NewUsecaseFromRepo(mr)
			_, err := u.Search(context.Background(), tt.q, nil)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tt.wantErrIs)
				}
			}
			if tt.check != nil {
				tt.check(t, capturedQ)
			}
		})
	}
}

func TestUsecase_GetDocument(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		repoFn    func(_ context.Context, id string) (Document, error)
		wantErr   bool
		wantErrIs error
		check     func(t *testing.T, got Document)
	}{
		{"empty id rejected", "", nil, true, ErrIDRequired, nil},
		{"whitespace id rejected", "  ", nil, true, ErrIDRequired, nil},
		{
			"valid id returns content fields",
			"doc-1",
			func(_ context.Context, id string) (Document, error) {
				return Document{ID: id, ContentText: "# 标题\n正文", Organized: true, AssetURI: ""}, nil
			},
			false, nil,
			func(t *testing.T, got Document) {
				if got.ID != "doc-1" {
					t.Errorf("ID = %q, want %q", got.ID, "doc-1")
				}
				if got.ContentText != "# 标题\n正文" {
					t.Errorf("ContentText = %q, want organized markdown", got.ContentText)
				}
				if !got.Organized {
					t.Error("Organized = false, want true")
				}
			},
		},
		{
			"repo error propagated",
			"doc-missing",
			func(_ context.Context, _ string) (Document, error) { return Document{}, fmt.Errorf("not found") },
			true, nil, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.docGetFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			got, err := u.GetDocument(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.wantErrIs != nil {
				if !errors.Is(err, tt.wantErrIs) {
					t.Errorf("error = %v, want errors.Is(err, %v)", err, tt.wantErrIs)
				}
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestUsecase_RequireRepo(t *testing.T) {
	tests := []struct {
		name string
		u    *Usecase
		want bool
	}{
		{"nil usecase", nil, true},
		{"nil collections", &Usecase{}, true},
		{"all present", NewUsecaseFromRepo(noOpMockRepo()), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.u.requireRepo()
			got := err != nil
			if got != tt.want {
				t.Errorf("requireRepo() error = %v, want error %v", err, tt.want)
			}
		})
	}
}
