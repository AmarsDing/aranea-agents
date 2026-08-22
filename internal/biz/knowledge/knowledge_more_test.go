package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"aranea-agents/pkg/apierror"
)

func TestUsecase_UpdateDocumentStatus(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		status     string
		errMsg     string
		chunkCount int
		repoFn     func(_ context.Context, id, status, errMsg string, chunkCount int) error
		wantErr    bool
		wantErrIs  error
	}{
		{
			"delegates to repo on pending→indexing",
			"doc-1", "indexing", "", 5,
			func(_ context.Context, id, status, errMsg string, chunkCount int) error {
				if id != "doc-1" {
					t.Errorf("id = %q, want %q", id, "doc-1")
				}
				if status != "indexing" {
					t.Errorf("status = %q, want %q", status, "indexing")
				}
				if chunkCount != 5 {
					t.Errorf("chunkCount = %d, want 5", chunkCount)
				}
				return nil
			},
			false, nil,
		},
		{
			"unknown status rejected",
			"doc-1", "ready", "", 0,
			nil,
			true, ErrInvalidDocumentStatus,
		},
		{
			"repo error propagated",
			"doc-1", "error", "parse error", 0,
			func(_ context.Context, _, _, _ string, _ int) error {
				return fmt.Errorf("knowledge: db error")
			},
			true, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.docUpdateFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			err := u.UpdateDocumentStatus(context.Background(), tt.id, tt.status, tt.errMsg, tt.chunkCount)
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

func TestUsecase_UpdateDocumentStatus_NilUsecase(t *testing.T) {
	var u *Usecase
	err := u.UpdateDocumentStatus(context.Background(), "doc-1", "indexing", "", 0)
	if err == nil {
		t.Fatal("nil usecase should return error")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestUsecase_UpdateDocumentStatus_RejectsPendingToIndexed(t *testing.T) {
	mr := noOpMockRepo()
	var wrote bool
	mr.docUpdateFn = func(context.Context, string, string, string, int) error {
		wrote = true
		return nil
	}
	u := NewUsecaseFromRepo(mr)
	err := u.UpdateDocumentStatus(context.Background(), "doc-1", "indexed", "", 3)
	if err == nil {
		t.Fatal("pending→indexed should be rejected")
	}
	if wrote {
		t.Fatal("repo must not be written on illegal transition")
	}
}

func TestUsecase_CommitIndexedDocument_RequiresIndexing(t *testing.T) {
	mr := noOpMockRepo()
	u := NewUsecaseFromRepo(mr)
	err := u.CommitIndexedDocument(context.Background(), "col-1", "doc-1", []Chunk{{ID: "c1", CollectionID: "col-1"}}, 1)
	if err == nil {
		t.Fatal("commit from pending should fail")
	}
}

func TestUsecase_CommitIndexedDocument_SequentialFallback(t *testing.T) {
	mr := noOpMockRepo()
	mr.docGetFn = func(_ context.Context, id string) (Document, error) {
		return Document{ID: id, Status: "indexing"}, nil
	}
	var inserted, status, counts bool
	mr.chunkInsertFn = func(_ context.Context, chunks []Chunk) error {
		inserted = len(chunks) == 1
		return nil
	}
	mr.docUpdateFn = func(_ context.Context, _, statusVal, _ string, cc int) error {
		status = statusVal == "indexed" && cc == 1
		return nil
	}
	mr.collUpdateFn = func(_ context.Context, id string, docDelta, chunkDelta int) error {
		counts = id == "col-1" && docDelta == 1 && chunkDelta == 1
		return nil
	}
	u := NewUsecaseFromRepo(mr)
	if err := u.CommitIndexedDocument(context.Background(), "col-1", "doc-1", []Chunk{{ID: "c1", CollectionID: "col-1"}}, 1); err != nil {
		t.Fatalf("commit: %v", err)
	}
	if !inserted || !status || !counts {
		t.Fatalf("fallback writes incomplete: insert=%v status=%v counts=%v", inserted, status, counts)
	}
}

// Phase 9：图片异步提取完成后回写 content_text/organized。
func TestUsecase_UpdateDocumentContent(t *testing.T) {
	mr := noOpMockRepo()
	mr.docContentFn = func(_ context.Context, id, contentText string, organized bool) error {
		if id != "doc-9" {
			t.Errorf("id = %q, want doc-9", id)
		}
		if contentText != "# 图描述" {
			t.Errorf("contentText = %q", contentText)
		}
		if !organized {
			t.Error("organized = false, want true")
		}
		return nil
	}
	u := NewUsecaseFromRepo(mr)
	if err := u.UpdateDocumentContent(context.Background(), "doc-9", "# 图描述", true); err != nil {
		t.Fatalf("UpdateDocumentContent error: %v", err)
	}

	mr.docContentFn = func(_ context.Context, _, _ string, _ bool) error {
		return fmt.Errorf("db boom")
	}
	if err := u.UpdateDocumentContent(context.Background(), "doc-9", "x", false); err == nil {
		t.Fatal("expected repo error propagated")
	}
}

func TestUsecase_UpdateDocumentContent_NilUsecase(t *testing.T) {
	var u *Usecase
	err := u.UpdateDocumentContent(context.Background(), "doc-1", "x", true)
	if err == nil {
		t.Fatal("nil usecase should return error")
	}
	if !errors.Is(err, ErrUnavailable) {
		t.Errorf("expected ErrUnavailable, got %v", err)
	}
}

func TestUsecase_UpdateCollectionCounts(t *testing.T) {
	tests := []struct {
		name       string
		id         string
		docDelta   int
		chunkDelta int
		repoFn     func(_ context.Context, id string, docDelta, chunkDelta int) error
		wantErr    bool
		wantErrIs  error
	}{
		{
			"delegates to repo on success",
			"col-1", 1, 5,
			func(_ context.Context, id string, docDelta, chunkDelta int) error {
				if id != "col-1" {
					t.Errorf("id = %q, want %q", id, "col-1")
				}
				if docDelta != 1 {
					t.Errorf("docDelta = %d, want 1", docDelta)
				}
				if chunkDelta != 5 {
					t.Errorf("chunkDelta = %d, want 5", chunkDelta)
				}
				return nil
			},
			false, nil,
		},
		{
			"negative deltas passed through",
			"col-1", -1, -3,
			func(_ context.Context, _ string, docDelta, chunkDelta int) error {
				if docDelta != -1 {
					t.Errorf("docDelta = %d, want -1", docDelta)
				}
				if chunkDelta != -3 {
					t.Errorf("chunkDelta = %d, want -3", chunkDelta)
				}
				return nil
			},
			false, nil,
		},
		{
			"repo error propagated",
			"col-1", 1, 1,
			func(_ context.Context, _ string, _, _ int) error {
				return fmt.Errorf("knowledge: db error")
			},
			true, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.collUpdateFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			err := u.UpdateCollectionCounts(context.Background(), tt.id, tt.docDelta, tt.chunkDelta)
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

func TestUsecase_UpdateCollectionCounts_NilUsecase(t *testing.T) {
	var u *Usecase
	err := u.UpdateCollectionCounts(context.Background(), "col-1", 1, 1)
	if err == nil {
		t.Fatal("nil usecase should return error")
	}
}

func TestUsecase_InsertChunks(t *testing.T) {
	tests := []struct {
		name      string
		chunks    []Chunk
		repoFn    func(_ context.Context, chunks []Chunk) error
		wantErr   bool
		wantErrIs error
	}{
		{
			"nil slice short circuits",
			nil,
			func(_ context.Context, _ []Chunk) error {
				t.Error("repo should not be called for nil chunks")
				return nil
			},
			false, nil,
		},
		{
			"empty slice short circuits",
			[]Chunk{},
			func(_ context.Context, _ []Chunk) error {
				t.Error("repo should not be called for empty chunks")
				return nil
			},
			false, nil,
		},
		{
			"non-empty chunks delegates to repo",
			[]Chunk{{ID: "c1", Content: "hello"}, {ID: "c2", Content: "world"}},
			func(_ context.Context, chunks []Chunk) error {
				if len(chunks) != 2 {
					t.Errorf("len(chunks) = %d, want 2", len(chunks))
				}
				return nil
			},
			false, nil,
		},
		{
			"repo error propagated",
			[]Chunk{{ID: "c1"}},
			func(_ context.Context, _ []Chunk) error {
				return fmt.Errorf("knowledge: insert error")
			},
			true, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.chunkInsertFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			err := u.InsertChunks(context.Background(), tt.chunks)
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

func TestUsecase_InsertChunks_NilUsecase(t *testing.T) {
	var u *Usecase
	err := u.InsertChunks(context.Background(), []Chunk{{ID: "c1"}})
	if err == nil {
		t.Fatal("nil usecase should return error")
	}
}

func TestUsecase_InsertChunks_NilUsecase_EmptySlice(t *testing.T) {
	var u *Usecase
	err := u.InsertChunks(context.Background(), nil)
	if err == nil {
		t.Fatal("nil usecase should return error even with empty chunks (requireRepo runs first)")
	}
}

// US-14：默认知识库懒创建——按 name 复用 / 不存在时创建。
func TestUsecase_EnsureDefaultCollection(t *testing.T) {
	t.Run("reuses existing default collection by name", func(t *testing.T) {
		mr := noOpMockRepo()
		mr.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
			return []Collection{
				{ID: "col-a", Name: "产品文档"},
				{ID: "col-default", Name: DefaultCollectionName, EmbeddingModel: "m1", Dim: 1024},
			}, 2, nil
		}
		mr.collCreateFn = func(_ context.Context, _ Collection) (Collection, error) {
			t.Error("CreateCollection should not be called when default exists")
			return Collection{}, nil
		}
		u := NewUsecaseFromRepo(mr)
		got, err := u.EnsureDefaultCollection(context.Background(), "m1", 1024, "")
		if err != nil {
			t.Fatalf("EnsureDefaultCollection error: %v", err)
		}
		if got.ID != "col-default" {
			t.Errorf("ID = %q, want col-default", got.ID)
		}
	})

	t.Run("creates default collection when missing", func(t *testing.T) {
		mr := noOpMockRepo()
		mr.collListFn = func(_ context.Context, _ string, _, _ int) ([]Collection, int, error) {
			return []Collection{{ID: "col-a", Name: "产品文档"}}, 1, nil
		}
		mr.collCreateFn = func(_ context.Context, c Collection) (Collection, error) {
			if c.Name != DefaultCollectionName {
				t.Errorf("Name = %q, want %q", c.Name, DefaultCollectionName)
			}
			if c.EmbeddingModel != "text-embedding-3-small" {
				t.Errorf("EmbeddingModel = %q", c.EmbeddingModel)
			}
			if c.Dim != 768 {
				t.Errorf("Dim = %d, want 768", c.Dim)
			}
			if c.VaultBackend != VaultBackendTeam {
				t.Errorf("VaultBackend = %q, want team inbox", c.VaultBackend)
			}
			c.ID = "col-new-default"
			return c, nil
		}
		u := NewUsecaseFromRepo(mr)
		got, err := u.EnsureDefaultCollection(context.Background(), "text-embedding-3-small", 768, "")
		if err != nil {
			t.Fatalf("EnsureDefaultCollection error: %v", err)
		}
		if got.ID != "col-new-default" {
			t.Errorf("ID = %q, want col-new-default", got.ID)
		}
	})

	t.Run("empty embedding model creates lexical team inbox", func(t *testing.T) {
		mr := noOpMockRepo()
		mr.collCreateFn = func(_ context.Context, c Collection) (Collection, error) {
			if c.EmbeddingModel != "" {
				t.Errorf("EmbeddingModel = %q, want empty", c.EmbeddingModel)
			}
			if c.Dim != 0 {
				t.Errorf("Dim = %d, want 0 when model is empty", c.Dim)
			}
			if c.VaultBackend != VaultBackendTeam {
				t.Errorf("VaultBackend = %q, want team", c.VaultBackend)
			}
			if c.RootPath != "" {
				t.Errorf("RootPath = %q, want empty team inbox", c.RootPath)
			}
			c.ID = "col-lex-default"
			return c, nil
		}
		u := NewUsecaseFromRepo(mr)
		got, err := u.EnsureDefaultCollection(context.Background(), "", 1536, "")
		if err != nil {
			t.Fatalf("EnsureDefaultCollection error: %v", err)
		}
		if got.ID != "col-lex-default" {
			t.Errorf("ID = %q, want col-lex-default", got.ID)
		}
	})

	t.Run("nil usecase returns unavailable", func(t *testing.T) {
		var u *Usecase
		_, err := u.EnsureDefaultCollection(context.Background(), "m", 1536, "")
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("error = %v, want ErrUnavailable", err)
		}
	})

	t.Run("retries GetByName after unique conflict", func(t *testing.T) {
		mr := noOpMockRepo()
		lookups := 0
		mr.collGetByNameFn = func(_ context.Context, _, name string) (Collection, error) {
			lookups++
			if lookups == 1 {
				return Collection{}, apierror.NotFound(apierror.DomainKnowledge, "missing")
			}
			return Collection{ID: "col-won", Name: name}, nil
		}
		mr.collCreateFn = func(_ context.Context, _ Collection) (Collection, error) {
			return Collection{}, apierror.Conflict(apierror.DomainKnowledge, "duplicate name")
		}
		u := NewUsecaseFromRepo(mr)
		got, err := u.EnsureDefaultCollection(context.Background(), "m1", 1024, "ws-a")
		if err != nil {
			t.Fatalf("EnsureDefaultCollection error: %v", err)
		}
		if got.ID != "col-won" {
			t.Errorf("ID = %q, want col-won", got.ID)
		}
		if lookups != 2 {
			t.Errorf("GetCollectionByName calls = %d, want 2", lookups)
		}
	})
}

// US-14：文档跨库移动——dim 兼容校验 + 委托 Repo 事务。
func TestUsecase_MoveDocument(t *testing.T) {
	t.Run("empty id rejected", func(t *testing.T) {
		u := NewUsecaseFromRepo(noOpMockRepo())
		_, err := u.MoveDocument(context.Background(), "  ", "col-b")
		if !errors.Is(err, ErrIDRequired) {
			t.Errorf("error = %v, want ErrIDRequired", err)
		}
	})

	t.Run("empty target rejected", func(t *testing.T) {
		u := NewUsecaseFromRepo(noOpMockRepo())
		_, err := u.MoveDocument(context.Background(), "doc-1", " ")
		if !errors.Is(err, ErrCollectionIDRequired) {
			t.Errorf("error = %v, want ErrCollectionIDRequired", err)
		}
	})

	t.Run("same collection is no-op", func(t *testing.T) {
		mr := noOpMockRepo()
		mr.docGetFn = func(_ context.Context, id string) (Document, error) {
			return Document{ID: id, CollectionID: "col-a"}, nil
		}
		mr.docMoveFn = func(_ context.Context, _, _ string) (Document, error) {
			t.Error("repo MoveDocument should not be called for same-collection no-op")
			return Document{}, nil
		}
		u := NewUsecaseFromRepo(mr)
		got, err := u.MoveDocument(context.Background(), "doc-1", "col-a")
		if err != nil {
			t.Fatalf("MoveDocument error: %v", err)
		}
		if got.CollectionID != "col-a" {
			t.Errorf("CollectionID = %q, want col-a", got.CollectionID)
		}
	})

	t.Run("dim mismatch rejected with conflict", func(t *testing.T) {
		mr := noOpMockRepo()
		mr.docGetFn = func(_ context.Context, id string) (Document, error) {
			return Document{ID: id, CollectionID: "col-a"}, nil
		}
		mr.collGetFn = func(_ context.Context, id string) (Collection, error) {
			if id == "col-a" {
				return Collection{ID: "col-a", Dim: 1536}, nil
			}
			return Collection{ID: id, Dim: 768}, nil
		}
		mr.docMoveFn = func(_ context.Context, _, _ string) (Document, error) {
			t.Error("repo MoveDocument should not be called on dim mismatch")
			return Document{}, nil
		}
		u := NewUsecaseFromRepo(mr)
		_, err := u.MoveDocument(context.Background(), "doc-1", "col-b")
		if !errors.Is(err, ErrMoveDimensionMismatch) {
			t.Errorf("error = %v, want ErrMoveDimensionMismatch", err)
		}
	})

	t.Run("same dim delegates to repo", func(t *testing.T) {
		mr := noOpMockRepo()
		mr.docGetFn = func(_ context.Context, id string) (Document, error) {
			return Document{ID: id, CollectionID: "col-a"}, nil
		}
		mr.collGetFn = func(_ context.Context, id string) (Collection, error) {
			return Collection{ID: id, Dim: 1536}, nil
		}
		mr.docMoveFn = func(_ context.Context, id, target string) (Document, error) {
			if id != "doc-1" || target != "col-b" {
				t.Errorf("MoveDocument(%q, %q), want (doc-1, col-b)", id, target)
			}
			return Document{ID: id, CollectionID: target}, nil
		}
		u := NewUsecaseFromRepo(mr)
		got, err := u.MoveDocument(context.Background(), "doc-1", "col-b")
		if err != nil {
			t.Fatalf("MoveDocument error: %v", err)
		}
		if got.CollectionID != "col-b" {
			t.Errorf("CollectionID = %q, want col-b", got.CollectionID)
		}
	})

	t.Run("document lookup error propagated", func(t *testing.T) {
		mr := noOpMockRepo()
		mr.docGetFn = func(_ context.Context, _ string) (Document, error) {
			return Document{}, fmt.Errorf("not found")
		}
		u := NewUsecaseFromRepo(mr)
		if _, err := u.MoveDocument(context.Background(), "doc-x", "col-b"); err == nil {
			t.Fatal("expected error propagated")
		}
	})

	t.Run("nil usecase returns unavailable", func(t *testing.T) {
		var u *Usecase
		_, err := u.MoveDocument(context.Background(), "doc-1", "col-b")
		if !errors.Is(err, ErrUnavailable) {
			t.Errorf("error = %v, want ErrUnavailable", err)
		}
	})
}

func TestUsecase_DeleteDocument(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		repoFn    func(_ context.Context, id string) error
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
			"valid id passes",
			"doc-1",
			func(_ context.Context, id string) error {
				if id != "doc-1" {
					t.Errorf("id = %q, want %q", id, "doc-1")
				}
				return nil
			},
			false, nil,
		},
		{
			"repo error propagated",
			"doc-1",
			func(_ context.Context, _ string) error {
				return fmt.Errorf("knowledge: db error")
			},
			true, nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.docDeleteFn = tt.repoFn
			}
			u := NewUsecaseFromRepo(mr)
			err := u.DeleteDocument(context.Background(), tt.id)
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
