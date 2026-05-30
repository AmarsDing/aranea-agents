package service

import (
	"testing"

	v1 "aranea-agents/api/kratos/session/v1"
	"aranea-agents/internal/biz"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func TestValidateBatchHTTPRequest(t *testing.T) {
	cases := []struct {
		name    string
		ids     []string
		days    int32
		wantErr bool
		wantMsg string
	}{
		{
			name:    "ids_provided_no_days",
			ids:     []string{"s1", "s2"},
			days:    0,
			wantErr: false,
		},
		{
			name:    "no_ids_valid_days",
			ids:     nil,
			days:    7,
			wantErr: false,
		},
		{
			name:    "no_ids_no_days",
			ids:     nil,
			days:    0,
			wantErr: true,
			wantMsg: "ids or older_than_days >= 1 is required",
		},
		{
			name:    "empty_ids_no_days",
			ids:     []string{},
			days:    0,
			wantErr: true,
			wantMsg: "ids or older_than_days >= 1 is required",
		},
		{
			name:    "negative_days",
			ids:     []string{"s1"},
			days:    -1,
			wantErr: true,
			wantMsg: "older_than_days must be >= 0",
		},
		{
			name:    "no_ids_negative_days",
			ids:     nil,
			days:    -5,
			wantErr: true,
			wantMsg: "ids or older_than_days >= 1 is required",
		},
		{
			name:    "ids_and_days_both_set",
			ids:     []string{"s1"},
			days:    30,
			wantErr: false,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateBatchHTTPRequest(tc.ids, tc.days)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				ke := kerrors.FromError(err)
				if ke == nil || ke.Message != tc.wantMsg {
					t.Fatalf("message = %q, want %q", ke.Message, tc.wantMsg)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
			}
		})
	}
}

func TestBatchScopeFromProto(t *testing.T) {
	cases := []struct {
		name string
		in   *v1.SessionBatchScope
		want biz.SessionBatchScope
	}{
		{
			name: "nil_input",
			in:   nil,
			want: biz.SessionBatchScope{},
		},
		{
			name: "full_scope",
			in: &v1.SessionBatchScope{
				OwnerType:     "agent",
				AgentId:       "a1",
				TeamId:        "t1",
				Status:        "completed",
				ContextStatus: "normal",
				Keyword:       "hello",
				UserId:        "u1",
			},
			want: biz.SessionBatchScope{
				OwnerType:     "agent",
				AgentID:       "a1",
				TeamID:        "t1",
				Status:        "completed",
				ContextStatus: "normal",
				Keyword:       "hello",
				UserID:        "u1",
			},
		},
		{
			name: "partial_scope",
			in: &v1.SessionBatchScope{
				AgentId: "a2",
			},
			want: biz.SessionBatchScope{
				AgentID: "a2",
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := batchScopeFromProto(tc.in)
			if got != tc.want {
				t.Fatalf("got=%+v want=%+v", got, tc.want)
			}
		})
	}
}

func TestToProtoBatchPreview(t *testing.T) {
	cases := []struct {
		name string
		in   biz.SessionBatchPreview
		want *v1.BatchPreviewSessionsResponse
	}{
		{
			name: "zero_values",
			in:   biz.SessionBatchPreview{},
			want: &v1.BatchPreviewSessionsResponse{},
		},
		{
			name: "full_preview",
			in: biz.SessionBatchPreview{
				Matched:         10,
				SkippedRunning:  2,
				SkippedNotFound: 1,
				Truncated:       true,
				SampleIDs:       []string{"s1", "s2"},
			},
			want: &v1.BatchPreviewSessionsResponse{
				Matched:         10,
				SkippedRunning:  2,
				SkippedNotFound: 1,
				Truncated:       true,
				SampleIds:       []string{"s1", "s2"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoBatchPreview(tc.in)
			if got.Matched != tc.want.Matched || got.SkippedRunning != tc.want.SkippedRunning || got.SkippedNotFound != tc.want.SkippedNotFound || got.Truncated != tc.want.Truncated {
				t.Fatalf("got=%+v want=%+v", got, tc.want)
			}
			if len(got.SampleIds) != len(tc.want.SampleIds) {
				t.Fatalf("SampleIds len = %d, want %d", len(got.SampleIds), len(tc.want.SampleIds))
			}
			for i, id := range got.SampleIds {
				if id != tc.want.SampleIds[i] {
					t.Fatalf("SampleIds[%d] = %q, want %q", i, id, tc.want.SampleIds[i])
				}
			}
		})
	}
}

func TestToProtoBatchResult(t *testing.T) {
	cases := []struct {
		name string
		in   biz.SessionBatchResult
		want *v1.BatchSessionsResponse
	}{
		{
			name: "zero_values",
			in:   biz.SessionBatchResult{},
			want: &v1.BatchSessionsResponse{},
		},
		{
			name: "full_result",
			in: biz.SessionBatchResult{
				Matched:         5,
				Processed:       4,
				SkippedRunning:  1,
				SkippedNotFound: 2,
				Truncated:       false,
				FailedIDs:       []string{"f1"},
			},
			want: &v1.BatchSessionsResponse{
				Matched:         5,
				Processed:       4,
				SkippedRunning:  1,
				SkippedNotFound: 2,
				Truncated:       false,
				FailedIds:       []string{"f1"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := toProtoBatchResult(tc.in)
			if got.Matched != tc.want.Matched || got.Processed != tc.want.Processed || got.SkippedRunning != tc.want.SkippedRunning || got.SkippedNotFound != tc.want.SkippedNotFound || got.Truncated != tc.want.Truncated {
				t.Fatalf("got=%+v want=%+v", got, tc.want)
			}
			if len(got.FailedIds) != len(tc.want.FailedIds) {
				t.Fatalf("FailedIds len = %d, want %d", len(got.FailedIds), len(tc.want.FailedIds))
			}
			for i, id := range got.FailedIds {
				if id != tc.want.FailedIds[i] {
					t.Fatalf("FailedIds[%d] = %q, want %q", i, id, tc.want.FailedIds[i])
				}
			}
		})
	}
}
