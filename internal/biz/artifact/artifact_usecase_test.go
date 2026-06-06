package artifact_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz/artifact"
	"aranea-agents/pkg/loggateway"
)

type mockRepo struct {
	saveFn                 func(ctx context.Context, sessionID, name, mimeType string, data []byte) (artifact.Artifact, error)
	loadFn                 func(ctx context.Context, id string, version int) (artifact.Artifact, []byte, error)
	loadMetaFn             func(ctx context.Context, id string, version int) (artifact.Artifact, error)
	listFn                 func(ctx context.Context, sessionID string, limit, offset int) ([]artifact.Artifact, int, error)
	deleteFn               func(ctx context.Context, id string) error
	deleteVersionFn        func(ctx context.Context, sessionID, name string, version int) error
	listBySessionAndNameFn func(ctx context.Context, sessionID, name string) ([]artifact.Artifact, error)
}

func (m *mockRepo) Save(ctx context.Context, sessionID, name, mimeType string, data []byte) (artifact.Artifact, error) {
	if m.saveFn != nil {
		return m.saveFn(ctx, sessionID, name, mimeType, data)
	}
	return artifact.Artifact{}, nil
}

func (m *mockRepo) Load(ctx context.Context, id string, version int) (artifact.Artifact, []byte, error) {
	if m.loadFn != nil {
		return m.loadFn(ctx, id, version)
	}
	return artifact.Artifact{}, nil, nil
}

func (m *mockRepo) LoadMeta(ctx context.Context, id string, version int) (artifact.Artifact, error) {
	if m.loadMetaFn != nil {
		return m.loadMetaFn(ctx, id, version)
	}
	return artifact.Artifact{}, nil
}

func (m *mockRepo) List(ctx context.Context, sessionID string, limit, offset int) ([]artifact.Artifact, int, error) {
	if m.listFn != nil {
		return m.listFn(ctx, sessionID, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockRepo) Delete(ctx context.Context, id string) error {
	if m.deleteFn != nil {
		return m.deleteFn(ctx, id)
	}
	return nil
}

func (m *mockRepo) DeleteVersion(ctx context.Context, sessionID, name string, version int) error {
	if m.deleteVersionFn != nil {
		return m.deleteVersionFn(ctx, sessionID, name, version)
	}
	return nil
}

func (m *mockRepo) ListBySessionAndName(ctx context.Context, sessionID, name string) ([]artifact.Artifact, error) {
	if m.listBySessionAndNameFn != nil {
		return m.listBySessionAndNameFn(ctx, sessionID, name)
	}
	return nil, nil
}

func TestUsecase_Save(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := artifact.Artifact{ID: "a1", SessionID: "s1", Name: "f.csv", MimeType: "text/csv", Size: 4}
		repo := &mockRepo{
			saveFn: func(_ context.Context, sessionID, name, mimeType string, data []byte) (artifact.Artifact, error) {
				if sessionID != "s1" || name != "f.csv" || mimeType != "text/csv" || string(data) != "data" {
					t.Errorf("unexpected args: sid=%q name=%q mime=%q data=%q", sessionID, name, mimeType, string(data))
				}
				return want, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		got, err := uc.Save(context.Background(), "s1", "f.csv", "text/csv", []byte("data"))
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != want.ID {
			t.Fatalf("got %q, want %q", got.ID, want.ID)
		}
	})

	t.Run("exceeds_size_limit", func(t *testing.T) {
		repo := &mockRepo{}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		big := make([]byte, artifact.MaxUploadBytes+1)
		_, err := uc.Save(context.Background(), "s1", "big.bin", "application/octet-stream", big)
		if err == nil {
			t.Fatal("expected error for oversized upload")
		}
		if !errors.Is(err, artifact.ErrSizeExceeded) {
			t.Fatalf("expected ErrSizeExceeded, got %v", err)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockRepo{
			saveFn: func(context.Context, string, string, string, []byte) (artifact.Artifact, error) {
				return artifact.Artifact{}, fmt.Errorf("db fail")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.Save(context.Background(), "s1", "f.csv", "text/csv", []byte("x"))
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("collector_integration", func(t *testing.T) {
		want := artifact.Artifact{ID: "a1", Name: "f.csv", MimeType: "text/csv", Size: 4}
		repo := &mockRepo{
			saveFn: func(context.Context, string, string, string, []byte) (artifact.Artifact, error) {
				return want, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		ctx, c := artifact.WithTurnCollector(context.Background())
		_, err := uc.Save(ctx, "s1", "f.csv", "text/csv", []byte("data"))
		if err != nil {
			t.Fatal(err)
		}
		refs := c.Refs()
		if len(refs) != 1 {
			t.Fatalf("expected 1 ref, got %d", len(refs))
		}
		if refs[0].ID != "a1" {
			t.Fatalf("ref ID=%q, want a1", refs[0].ID)
		}
	})

	t.Run("no_collector_in_context", func(t *testing.T) {
		want := artifact.Artifact{ID: "a2", Name: "f.csv", MimeType: "text/csv", Size: 1}
		repo := &mockRepo{
			saveFn: func(context.Context, string, string, string, []byte) (artifact.Artifact, error) {
				return want, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.Save(context.Background(), "s1", "f.csv", "text/csv", []byte("x"))
		if err != nil {
			t.Fatal(err)
		}
	})
}

func TestUsecase_Load(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		wantArt := artifact.Artifact{ID: "a1", Name: "f.csv"}
		wantData := []byte("hello")
		repo := &mockRepo{
			loadFn: func(_ context.Context, id string, version int) (artifact.Artifact, []byte, error) {
				if id != "a1" || version != 2 {
					t.Errorf("unexpected args: id=%q version=%d", id, version)
				}
				return wantArt, wantData, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		a, d, err := uc.Load(context.Background(), "a1", 2)
		if err != nil {
			t.Fatal(err)
		}
		if a.ID != "a1" || string(d) != "hello" {
			t.Fatalf("got a.ID=%q d=%q", a.ID, string(d))
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockRepo{
			loadFn: func(context.Context, string, int) (artifact.Artifact, []byte, error) {
				return artifact.Artifact{}, nil, fmt.Errorf("not found")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		_, _, err := uc.Load(context.Background(), "missing", 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestUsecase_LoadMeta(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := artifact.Artifact{ID: "a1", Name: "f.csv", MimeType: "text/csv"}
		repo := &mockRepo{
			loadMetaFn: func(_ context.Context, id string, version int) (artifact.Artifact, error) {
				return want, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		got, err := uc.LoadMeta(context.Background(), "a1", 0)
		if err != nil {
			t.Fatal(err)
		}
		if got.ID != "a1" {
			t.Fatalf("got %q", got.ID)
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockRepo{
			loadMetaFn: func(context.Context, string, int) (artifact.Artifact, error) {
				return artifact.Artifact{}, fmt.Errorf("not found")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.LoadMeta(context.Background(), "missing", 0)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestUsecase_List(t *testing.T) {
	allItems := []artifact.Artifact{
		{ID: "1", Name: "report.csv", MimeType: "text/csv", SessionID: "s1"},
		{ID: "2", Name: "photo.png", MimeType: "image/png", SessionID: "s1"},
		{ID: "3", Name: "data.json", MimeType: "application/json", SessionID: "s1"},
	}

	t.Run("no_filter_passes_limit_offset", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(_ context.Context, sessionID string, limit, offset int) ([]artifact.Artifact, int, error) {
				if sessionID != "s1" {
					t.Errorf("sessionID=%q", sessionID)
				}
				if limit != 10 || offset != 5 {
					t.Errorf("limit=%d offset=%d", limit, offset)
				}
				return allItems[:2], 2, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		items, total, err := uc.List(context.Background(), "s1", 10, 5, "", "")
		if err != nil {
			t.Fatal(err)
		}
		if total != 2 || len(items) != 2 {
			t.Fatalf("total=%d len=%d", total, len(items))
		}
	})

	t.Run("with_query_fetches_all_then_filters", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(_ context.Context, sessionID string, limit, offset int) ([]artifact.Artifact, int, error) {
				if limit != 0 || offset != 0 {
					t.Errorf("expected limit=0 offset=0 for filtered query, got limit=%d offset=%d", limit, offset)
				}
				return allItems, 3, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		items, total, err := uc.List(context.Background(), "s1", 10, 0, "csv", "")
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 {
			t.Fatalf("total=%d, want 1", total)
		}
		if len(items) != 1 || items[0].ID != "1" {
			t.Fatalf("items=%v", items)
		}
	})

	t.Run("with_mime_prefix", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(_ context.Context, sessionID string, limit, offset int) ([]artifact.Artifact, int, error) {
				return allItems, 3, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		items, total, err := uc.List(context.Background(), "s1", 10, 0, "", "image/")
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || items[0].ID != "2" {
			t.Fatalf("total=%d items=%v", total, items)
		}
	})

	t.Run("with_both_filters", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(context.Context, string, int, int) ([]artifact.Artifact, int, error) {
				return allItems, 3, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		items, total, err := uc.List(context.Background(), "s1", 10, 0, "photo", "image/")
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 || items[0].ID != "2" {
			t.Fatalf("total=%d items=%v", total, items)
		}
	})

	t.Run("filtered_offset_beyond_total", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(context.Context, string, int, int) ([]artifact.Artifact, int, error) {
				return allItems, 3, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		items, total, err := uc.List(context.Background(), "s1", 10, 100, "csv", "")
		if err != nil {
			t.Fatal(err)
		}
		if total != 1 {
			t.Fatalf("total=%d, want 1", total)
		}
		if len(items) != 0 {
			t.Fatalf("expected no items, got %d", len(items))
		}
	})

	t.Run("filtered_with_limit_truncation", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(context.Context, string, int, int) ([]artifact.Artifact, int, error) {
				return allItems, 3, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		items, total, err := uc.List(context.Background(), "s1", 1, 0, "s1", "")
		if err != nil {
			t.Fatal(err)
		}
		if total != 3 {
			t.Fatalf("total=%d, want 3", total)
		}
		if len(items) != 1 {
			t.Fatalf("expected 1 item, got %d", len(items))
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(context.Context, string, int, int) ([]artifact.Artifact, int, error) {
				return nil, 0, fmt.Errorf("db fail")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		_, _, err := uc.List(context.Background(), "s1", 10, 0, "", "")
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("whitespace_query_no_filter", func(t *testing.T) {
		repo := &mockRepo{
			listFn: func(_ context.Context, sessionID string, limit, offset int) ([]artifact.Artifact, int, error) {
				if limit != 10 || offset != 0 {
					t.Errorf("limit=%d offset=%d, expected no filter mode", limit, offset)
				}
				return allItems, 3, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		items, _, err := uc.List(context.Background(), "s1", 10, 0, "  ", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if len(items) != 3 {
			t.Fatalf("expected 3 items, got %d", len(items))
		}
	})
}

func TestUsecase_Delete(t *testing.T) {
	t.Run("empty_id_returns_bad_request", func(t *testing.T) {
		repo := &mockRepo{}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		err := uc.Delete(context.Background(), "")
		if err == nil {
			t.Fatal("expected error for empty id")
		}
		if !strings.Contains(err.Error(), "id is required") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("whitespace_id_returns_bad_request", func(t *testing.T) {
		repo := &mockRepo{}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		err := uc.Delete(context.Background(), "   ")
		if err == nil {
			t.Fatal("expected error for whitespace id")
		}
		if !strings.Contains(err.Error(), "id is required") {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("success_deletes_all_versions", func(t *testing.T) {
		meta := artifact.Artifact{ID: "a1", SessionID: "s1", Name: "f.csv"}
		versions := []artifact.Artifact{
			{ID: "a1", SessionID: "s1", Name: "f.csv", Version: 1},
			{ID: "a2", SessionID: "s1", Name: "f.csv", Version: 2},
			{ID: "a3", SessionID: "s1", Name: "f.csv", Version: 3},
		}
		deletedIDs := []string{}
		repo := &mockRepo{
			loadFn: func(_ context.Context, id string, version int) (artifact.Artifact, []byte, error) {
				return meta, nil, nil
			},
			listBySessionAndNameFn: func(_ context.Context, sessionID, name string) ([]artifact.Artifact, error) {
				return versions, nil
			},
			deleteFn: func(_ context.Context, id string) error {
				deletedIDs = append(deletedIDs, id)
				return nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		if err := uc.Delete(context.Background(), "a1"); err != nil {
			t.Fatal(err)
		}
		if len(deletedIDs) != 3 {
			t.Fatalf("deleted %d, want 3", len(deletedIDs))
		}
	})

	t.Run("load_error_best_effort_delete", func(t *testing.T) {
		deleteCalled := false
		repo := &mockRepo{
			loadFn: func(context.Context, string, int) (artifact.Artifact, []byte, error) {
				return artifact.Artifact{}, nil, fmt.Errorf("not found")
			},
			deleteFn: func(_ context.Context, id string) error {
				deleteCalled = true
				return nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		err := uc.Delete(context.Background(), "a1")
		if err == nil {
			t.Fatal("expected error from Load")
		}
		if !deleteCalled {
			t.Fatal("expected best-effort repo.Delete")
		}
	})

	t.Run("list_error_falls_back_to_single_delete", func(t *testing.T) {
		meta := artifact.Artifact{ID: "a1", SessionID: "s1", Name: "f.csv"}
		deleteCalled := false
		repo := &mockRepo{
			loadFn: func(_ context.Context, id string, version int) (artifact.Artifact, []byte, error) {
				return meta, nil, nil
			},
			listBySessionAndNameFn: func(context.Context, string, string) ([]artifact.Artifact, error) {
				return nil, fmt.Errorf("list error")
			},
			deleteFn: func(_ context.Context, id string) error {
				deleteCalled = true
				if id != "a1" {
					t.Errorf("deleted id=%q, want a1", id)
				}
				return nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		if err := uc.Delete(context.Background(), "a1"); err != nil {
			t.Fatal(err)
		}
		if !deleteCalled {
			t.Fatal("expected fallback repo.Delete")
		}
	})

	t.Run("list_empty_falls_back_to_single_delete", func(t *testing.T) {
		meta := artifact.Artifact{ID: "a1", SessionID: "s1", Name: "f.csv"}
		deleteCalled := false
		repo := &mockRepo{
			loadFn: func(_ context.Context, id string, version int) (artifact.Artifact, []byte, error) {
				return meta, nil, nil
			},
			listBySessionAndNameFn: func(context.Context, string, string) ([]artifact.Artifact, error) {
				return nil, nil
			},
			deleteFn: func(_ context.Context, id string) error {
				deleteCalled = true
				return nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		if err := uc.Delete(context.Background(), "a1"); err != nil {
			t.Fatal(err)
		}
		if !deleteCalled {
			t.Fatal("expected fallback repo.Delete")
		}
	})

	t.Run("partial_delete_error_returns_first", func(t *testing.T) {
		meta := artifact.Artifact{ID: "a1", SessionID: "s1", Name: "f.csv"}
		versions := []artifact.Artifact{
			{ID: "a1", SessionID: "s1", Name: "f.csv"},
			{ID: "a2", SessionID: "s1", Name: "f.csv"},
		}
		repo := &mockRepo{
			loadFn: func(context.Context, string, int) (artifact.Artifact, []byte, error) {
				return meta, nil, nil
			},
			listBySessionAndNameFn: func(context.Context, string, string) ([]artifact.Artifact, error) {
				return versions, nil
			},
			deleteFn: func(_ context.Context, id string) error {
				if id == "a2" {
					return fmt.Errorf("delete failed")
				}
				return nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		err := uc.Delete(context.Background(), "a1")
		if err == nil {
			t.Fatal("expected first delete error")
		}
	})
}

func TestUsecase_DeleteVersion(t *testing.T) {
	t.Run("empty_id", func(t *testing.T) {
		uc := artifact.NewUsecase(&mockRepo{}, loggateway.NewNoop())
		err := uc.DeleteVersion(context.Background(), "", 1)
		if err == nil {
			t.Fatal("expected error for empty id")
		}
	})

	t.Run("whitespace_id", func(t *testing.T) {
		uc := artifact.NewUsecase(&mockRepo{}, loggateway.NewNoop())
		err := uc.DeleteVersion(context.Background(), "   ", 1)
		if err == nil {
			t.Fatal("expected error for whitespace id")
		}
	})

	t.Run("version_zero", func(t *testing.T) {
		uc := artifact.NewUsecase(&mockRepo{}, loggateway.NewNoop())
		err := uc.DeleteVersion(context.Background(), "a1", 0)
		if err == nil {
			t.Fatal("expected error for version 0")
		}
	})

	t.Run("negative_version", func(t *testing.T) {
		uc := artifact.NewUsecase(&mockRepo{}, loggateway.NewNoop())
		err := uc.DeleteVersion(context.Background(), "a1", -1)
		if err == nil {
			t.Fatal("expected error for negative version")
		}
	})

	t.Run("success", func(t *testing.T) {
		meta := artifact.Artifact{ID: "a1", SessionID: "s1", Name: "f.csv"}
		repo := &mockRepo{
			loadFn: func(_ context.Context, id string, version int) (artifact.Artifact, []byte, error) {
				return meta, nil, nil
			},
			deleteVersionFn: func(_ context.Context, sessionID, name string, version int) error {
				if sessionID != "s1" || name != "f.csv" || version != 2 {
					t.Errorf("unexpected args: sid=%q name=%q version=%d", sessionID, name, version)
				}
				return nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		if err := uc.DeleteVersion(context.Background(), "a1", 2); err != nil {
			t.Fatal(err)
		}
	})

	t.Run("load_error", func(t *testing.T) {
		repo := &mockRepo{
			loadFn: func(context.Context, string, int) (artifact.Artifact, []byte, error) {
				return artifact.Artifact{}, nil, fmt.Errorf("not found")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		err := uc.DeleteVersion(context.Background(), "a1", 1)
		if err == nil {
			t.Fatal("expected error")
		}
	})

	t.Run("delete_version_repo_error", func(t *testing.T) {
		meta := artifact.Artifact{ID: "a1", SessionID: "s1", Name: "f.csv"}
		repo := &mockRepo{
			loadFn: func(context.Context, string, int) (artifact.Artifact, []byte, error) {
				return meta, nil, nil
			},
			deleteVersionFn: func(context.Context, string, string, int) error {
				return fmt.Errorf("db fail")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		err := uc.DeleteVersion(context.Background(), "a1", 1)
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

func TestUsecase_ListVersions(t *testing.T) {
	t.Run("success", func(t *testing.T) {
		want := []artifact.Artifact{
			{ID: "a1", SessionID: "s1", Name: "f.csv", Version: 1},
			{ID: "a2", SessionID: "s1", Name: "f.csv", Version: 2},
		}
		repo := &mockRepo{
			listBySessionAndNameFn: func(_ context.Context, sessionID, name string) ([]artifact.Artifact, error) {
				if sessionID != "s1" || name != "f.csv" {
					t.Errorf("sid=%q name=%q", sessionID, name)
				}
				return want, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		got, err := uc.ListVersions(context.Background(), "s1", "f.csv")
		if err != nil {
			t.Fatal(err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d items", len(got))
		}
	})

	t.Run("repo_error", func(t *testing.T) {
		repo := &mockRepo{
			listBySessionAndNameFn: func(context.Context, string, string) ([]artifact.Artifact, error) {
				return nil, fmt.Errorf("db fail")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.ListVersions(context.Background(), "s1", "f.csv")
		if err == nil {
			t.Fatal("expected error")
		}
	})
}

type mockRepoWithStorage struct {
	*mockRepo
	storageBytesFn func(ctx context.Context) (int64, error)
}

func (m *mockRepoWithStorage) StorageBytes(ctx context.Context) (int64, error) {
	if m.storageBytesFn != nil {
		return m.storageBytesFn(ctx)
	}
	return 0, nil
}

func TestUsecase_StorageBytes(t *testing.T) {
	t.Run("repo_without_storage_reporter", func(t *testing.T) {
		repo := &mockRepo{}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		n, err := uc.StorageBytes(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("expected 0, got %d", n)
		}
	})

	t.Run("repo_with_storage_reporter", func(t *testing.T) {
		repo := &mockRepoWithStorage{
			mockRepo: &mockRepo{},
			storageBytesFn: func(context.Context) (int64, error) {
				return 4096, nil
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		n, err := uc.StorageBytes(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if n != 4096 {
			t.Fatalf("expected 4096, got %d", n)
		}
	})

	t.Run("repo_with_storage_reporter_error", func(t *testing.T) {
		repo := &mockRepoWithStorage{
			mockRepo: &mockRepo{},
			storageBytesFn: func(context.Context) (int64, error) {
				return 0, fmt.Errorf("storage error")
			},
		}
		uc := artifact.NewUsecase(repo, loggateway.NewNoop())
		_, err := uc.StorageBytes(context.Background())
		if err == nil {
			t.Fatal("expected error")
		}
	})
}
