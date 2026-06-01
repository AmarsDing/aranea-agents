package knowledge

import (
	"context"
	"testing"

	"github.com/go-kratos/kratos/v2/errors"
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
		errReason  string
	}{
		{
			"delegates to repo on success",
			"doc-1", "ready", "", 5,
			func(_ context.Context, id, status, errMsg string, chunkCount int) error {
				if id != "doc-1" {
					t.Errorf("id = %q, want %q", id, "doc-1")
				}
				if status != "ready" {
					t.Errorf("status = %q, want %q", status, "ready")
				}
				if chunkCount != 5 {
					t.Errorf("chunkCount = %d, want 5", chunkCount)
				}
				return nil
			},
			false, "",
		},
		{
			"repo error propagated",
			"doc-1", "failed", "parse error", 0,
			func(_ context.Context, _, _, _ string, _ int) error {
				return errors.InternalServer("KNOWLEDGE", "db error")
			},
			true, "KNOWLEDGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.docUpdateFn = tt.repoFn
			}
			u := NewUsecase(mr)
			err := u.UpdateDocumentStatus(context.Background(), tt.id, tt.status, tt.errMsg, tt.chunkCount)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
				}
			}
		})
	}
}

func TestUsecase_UpdateDocumentStatus_NilUsecase(t *testing.T) {
	var u *Usecase
	err := u.UpdateDocumentStatus(context.Background(), "doc-1", "ready", "", 0)
	if err == nil {
		t.Fatal("nil usecase should return error")
	}
	if e := errors.FromError(err); e == nil || e.Reason != "KNOWLEDGE" {
		t.Errorf("expected KNOWLEDGE error, got %v", err)
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
		errReason  string
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
			false, "",
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
			false, "",
		},
		{
			"repo error propagated",
			"col-1", 1, 1,
			func(_ context.Context, _ string, _, _ int) error {
				return errors.InternalServer("KNOWLEDGE", "db error")
			},
			true, "KNOWLEDGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.collUpdateFn = tt.repoFn
			}
			u := NewUsecase(mr)
			err := u.UpdateCollectionCounts(context.Background(), tt.id, tt.docDelta, tt.chunkDelta)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
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
		errReason string
	}{
		{
			"nil slice short circuits",
			nil,
			func(_ context.Context, _ []Chunk) error {
				t.Error("repo should not be called for nil chunks")
				return nil
			},
			false, "",
		},
		{
			"empty slice short circuits",
			[]Chunk{},
			func(_ context.Context, _ []Chunk) error {
				t.Error("repo should not be called for empty chunks")
				return nil
			},
			false, "",
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
			false, "",
		},
		{
			"repo error propagated",
			[]Chunk{{ID: "c1"}},
			func(_ context.Context, _ []Chunk) error {
				return errors.InternalServer("KNOWLEDGE", "insert error")
			},
			true, "KNOWLEDGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.chunkInsertFn = tt.repoFn
			}
			u := NewUsecase(mr)
			err := u.InsertChunks(context.Background(), tt.chunks)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
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

func TestUsecase_DeleteDocument(t *testing.T) {
	tests := []struct {
		name      string
		id        string
		repoFn    func(_ context.Context, id string) error
		wantErr   bool
		errReason string
	}{
		{
			"empty id rejected",
			"",
			nil, true, "KNOWLEDGE",
		},
		{
			"whitespace id rejected",
			"  ",
			nil, true, "KNOWLEDGE",
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
			false, "",
		},
		{
			"repo error propagated",
			"doc-1",
			func(_ context.Context, _ string) error {
				return errors.InternalServer("KNOWLEDGE", "db error")
			},
			true, "KNOWLEDGE",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mr := noOpMockRepo()
			if tt.repoFn != nil {
				mr.docDeleteFn = tt.repoFn
			}
			u := NewUsecase(mr)
			err := u.DeleteDocument(context.Background(), tt.id)
			if (err != nil) != tt.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errReason != "" {
				if e := errors.FromError(err); e != nil && e.Reason != tt.errReason {
					t.Errorf("reason = %q, want %q", e.Reason, tt.errReason)
				}
			}
		})
	}
}
