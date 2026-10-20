package session

import (
	"context"
	"errors"
	"testing"
)

func TestSearchMessages(t *testing.T) {

	tests := []struct {
		name        string
		query       MessageSearchQuery
		listMsgFn   func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error)
		wantErr     bool
		wantMsg     string
		checkResult func(t *testing.T, got MessageSearchResult)
	}{
		{
			name:    "empty session_id returns error",
			query:   MessageSearchQuery{SessionID: "", Keyword: "test"},
			wantErr: true,
			wantMsg: "session_id is required",
		},
		{
			name:    "whitespace session_id returns error",
			query:   MessageSearchQuery{SessionID: "  ", Keyword: "test"},
			wantErr: true,
			wantMsg: "session_id is required",
		},
		{
			name:    "empty keyword returns error",
			query:   MessageSearchQuery{SessionID: "s1", Keyword: ""},
			wantErr: true,
			wantMsg: "keyword is required",
		},
		{
			name:    "whitespace keyword returns error",
			query:   MessageSearchQuery{SessionID: "s1", Keyword: "   "},
			wantErr: true,
			wantMsg: "keyword is required",
		},
		{
			name:  "valid search returns results",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "hello", Limit: 10},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{
					{ID: "m1", Role: "user", ContentMarkdown: "hello world"},
					{ID: "m2", Role: "user", ContentMarkdown: "goodbye"},
				}, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got MessageSearchResult) {
				if got.Total != 1 {
					t.Fatalf("expected total 1, got %d", got.Total)
				}
				if len(got.Items) != 1 {
					t.Fatalf("expected 1 item, got %d", len(got.Items))
				}
				if got.Items[0].ID != "m1" {
					t.Fatalf("expected item id m1, got %s", got.Items[0].ID)
				}
			},
		},
		{
			name:  "limit applied to results",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "test", Limit: 2},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return []ChatMessage{
					{ID: "m1", Role: "user", ContentMarkdown: "test 1"},
					{ID: "m2", Role: "user", ContentMarkdown: "test 2"},
					{ID: "m3", Role: "user", ContentMarkdown: "test 3"},
				}, nil
			},
			wantErr: false,
			checkResult: func(t *testing.T, got MessageSearchResult) {
				if len(got.Items) != 2 {
					t.Fatalf("expected 2 items after limit, got %d", len(got.Items))
				}
			},
		},
		{
			name:  "repo error propagated",
			query: MessageSearchQuery{SessionID: "s1", Keyword: "test"},
			listMsgFn: func(_ context.Context, _ string, _, _ int) ([]ChatMessage, error) {
				return nil, errors.New("db error")
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &testRepo{}
			if tt.listMsgFn != nil {
				repo.listMessagesBySessionFn = tt.listMsgFn
			}
			uc := newTestUc(repo)

			got, err := uc.SearchMessages(context.Background(), tt.query)

			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tt.wantMsg != "" {
					assertBadRequest(t, err, tt.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkResult != nil {
				tt.checkResult(t, got)
			}
		})
	}
}
