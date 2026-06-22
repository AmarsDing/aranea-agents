package flowlog

import (
	"context"
	"errors"
	"testing"
	"time"
)

type mockRepo struct {
	insert          func(ctx context.Context, rec Record) error
	list            func(ctx context.Context, q Query) (ListResult, error)
	deleteOlderThan func(ctx context.Context, cutoff time.Time) (int64, error)
}

func (m *mockRepo) Insert(ctx context.Context, rec Record) error {
	if m.insert != nil {
		return m.insert(ctx, rec)
	}
	return nil
}

func (m *mockRepo) List(ctx context.Context, q Query) (ListResult, error) {
	if m.list != nil {
		return m.list(ctx, q)
	}
	return ListResult{}, nil
}

func (m *mockRepo) DeleteOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	if m.deleteOlderThan != nil {
		return m.deleteOlderThan(ctx, cutoff)
	}
	return 0, nil
}

func TestNewUsecase_NilRepo(t *testing.T) {
	uc := NewUsecase(nil)
	if uc != nil {
		t.Fatalf("expected nil, got %v", uc)
	}
}

func TestNewUsecase_ValidRepo(t *testing.T) {
	uc := NewUsecase(&mockRepo{})
	if uc == nil {
		t.Fatal("expected non-nil Usecase")
	}
}

func TestUsecase_Save(t *testing.T) {
	tests := []struct {
		name      string
		repo      Repo
		rec       Record
		insertErr error
		wantErr   bool
	}{
		{
			name:    "success",
			repo:    &mockRepo{},
			rec:     Record{TraceID: "t1", Message: "hello"},
			wantErr: false,
		},
		{
			name:    "repo error",
			repo:    &mockRepo{insert: func(_ context.Context, _ Record) error { return errors.New("db fail") }},
			rec:     Record{TraceID: "t1"},
			wantErr: true,
		},
		{
			name:    "nil usecase",
			repo:    nil,
			rec:     Record{TraceID: "t1"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var uc *Usecase
			if tt.repo != nil {
				uc = NewUsecase(tt.repo)
			}
			err := uc.Save(context.Background(), tt.rec)
			if (err != nil) != tt.wantErr {
				t.Fatalf("Save() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestUsecase_List(t *testing.T) {
	tests := []struct {
		name      string
		query     Query
		setupRepo func() *mockRepo
		wantEmpty bool
		wantLimit int
		wantErr   bool
	}{
		{
			name:      "missing all 3 IDs returns empty",
			query:     Query{Limit: 10},
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantEmpty: true,
		},
		{
			name:  "has TraceID delegates to repo",
			query: Query{TraceID: "trace-1", Limit: 10},
			setupRepo: func() *mockRepo {
				return &mockRepo{list: func(_ context.Context, q Query) (ListResult, error) {
					return ListResult{Items: []Record{{TraceID: "trace-1"}}, Total: 1}, nil
				}}
			},
			wantEmpty: false,
			wantLimit: 10,
		},
		{
			name:  "has SessionID delegates to repo",
			query: Query{SessionID: "sess-1", Limit: 10},
			setupRepo: func() *mockRepo {
				return &mockRepo{list: func(_ context.Context, q Query) (ListResult, error) {
					return ListResult{Items: []Record{{SessionID: "sess-1"}}, Total: 1}, nil
				}}
			},
			wantEmpty: false,
		},
		{
			name:  "has RunID delegates to repo",
			query: Query{RunID: "run-1", Limit: 10},
			setupRepo: func() *mockRepo {
				return &mockRepo{list: func(_ context.Context, q Query) (ListResult, error) {
					return ListResult{Items: []Record{{RunID: "run-1"}}, Total: 1}, nil
				}}
			},
			wantEmpty: false,
		},
		{
			name:  "default limit when zero",
			query: Query{TraceID: "t1"},
			setupRepo: func() *mockRepo {
				return &mockRepo{list: func(_ context.Context, q Query) (ListResult, error) {
					if q.Limit != 200 {
						t.Fatalf("expected default limit 200, got %d", q.Limit)
					}
					return ListResult{Total: 0}, nil
				}}
			},
		},
		{
			name:  "limit capped at 1000",
			query: Query{TraceID: "t1", Limit: 5000},
			setupRepo: func() *mockRepo {
				return &mockRepo{list: func(_ context.Context, q Query) (ListResult, error) {
					if q.Limit != 1000 {
						t.Fatalf("expected capped limit 1000, got %d", q.Limit)
					}
					return ListResult{Total: 0}, nil
				}}
			},
		},
		{
			name:  "repo error",
			query: Query{TraceID: "t1", Limit: 10},
			setupRepo: func() *mockRepo {
				return &mockRepo{list: func(_ context.Context, _ Query) (ListResult, error) {
					return ListResult{}, errors.New("db fail")
				}}
			},
			wantErr: true,
		},
		{
			name:      "whitespace-only IDs treated as empty",
			query:     Query{TraceID: "  ", SessionID: " ", RunID: "  ", Limit: 10},
			setupRepo: func() *mockRepo { return &mockRepo{} },
			wantEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			uc := NewUsecase(repo)
			result, err := uc.List(context.Background(), tt.query)
			if (err != nil) != tt.wantErr {
				t.Fatalf("List() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantEmpty && (len(result.Items) > 0 || result.Total > 0) {
				t.Fatalf("expected empty result, got %+v", result)
			}
		})
	}
}

func TestUsecase_List_NilUsecase(t *testing.T) {
	var uc *Usecase
	result, err := uc.List(context.Background(), Query{TraceID: "t1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Items) > 0 || result.Total > 0 {
		t.Fatalf("expected empty result for nil usecase, got %+v", result)
	}
}

func TestUsecase_PurgeExpired(t *testing.T) {
	tests := []struct {
		name      string
		setupRepo func() *mockRepo
		wantErr   bool
		wantDel   int64
	}{
		{
			name: "success",
			setupRepo: func() *mockRepo {
				return &mockRepo{deleteOlderThan: func(_ context.Context, _ time.Time) (int64, error) {
					return 42, nil
				}}
			},
			wantDel: 42,
		},
		{
			name: "repo error",
			setupRepo: func() *mockRepo {
				return &mockRepo{deleteOlderThan: func(_ context.Context, _ time.Time) (int64, error) {
					return 0, errors.New("db fail")
				}}
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := tt.setupRepo()
			uc := NewUsecase(repo)
			deleted, err := uc.PurgeExpired(context.Background())
			if (err != nil) != tt.wantErr {
				t.Fatalf("PurgeExpired() error = %v, wantErr %v", err, tt.wantErr)
			}
			if !tt.wantErr && deleted != tt.wantDel {
				t.Fatalf("PurgeExpired() deleted = %d, want %d", deleted, tt.wantDel)
			}
		})
	}
}

func TestUsecase_PurgeExpired_TTLCalculation(t *testing.T) {
	var capturedCutoff time.Time
	repo := &mockRepo{deleteOlderThan: func(_ context.Context, cutoff time.Time) (int64, error) {
		capturedCutoff = cutoff
		return 0, nil
	}}
	uc := NewUsecase(repo)
	_, err := uc.PurgeExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	now := time.Now().UTC()
	expectedCutoff := now.Add(-30 * 24 * time.Hour)
	diff := capturedCutoff.Sub(expectedCutoff)
	if diff < 0 {
		diff = -diff
	}
	if diff > 2*time.Second {
		t.Fatalf("cutoff diff too large: got %v, expected near %v (diff %v)", capturedCutoff, expectedCutoff, diff)
	}
}

func TestUsecase_PurgeExpired_NilUsecase(t *testing.T) {
	var uc *Usecase
	deleted, err := uc.PurgeExpired(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if deleted != 0 {
		t.Fatalf("expected 0 deleted for nil usecase, got %d", deleted)
	}
}

func TestFlowLogTTL_Default(t *testing.T) {
	ttl := flowLogTTL()
	expected := 30 * 24 * time.Hour
	if ttl != expected {
		t.Fatalf("expected default TTL %v, got %v", expected, ttl)
	}
}
