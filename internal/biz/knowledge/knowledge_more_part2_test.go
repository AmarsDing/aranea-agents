package knowledge

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

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
