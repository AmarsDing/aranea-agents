package evaluation

import (
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
	"context"
	"errors"
	"testing"
)

func TestGetAgentEvalTrend(t *testing.T) {
	tests := []struct {
		name      string
		agentID   string
		datasetID string
		limit     int
		setup     func(*fnMockRepo)
		wantErr   bool
		check     func(t *testing.T, points []TrendPoint)
	}{
		{
			name:    "empty_agentID_returns_error",
			agentID: "",
			wantErr: true,
			check:   func(t *testing.T, _ []TrendPoint) {},
		},
		{
			name:    "whitespace_agentID_returns_error",
			agentID: "   ",
			wantErr: true,
			check:   func(t *testing.T, _ []TrendPoint) {},
		},
		{
			name:      "default_limit_when_zero",
			agentID:   "agent-1",
			datasetID: "ds-1",
			limit:     0,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, _, _ string, limit int) ([]TrendPoint, error) {
					if limit != 30 {
						return nil, errors.New("limit not defaulted to 30")
					}
					return []TrendPoint{{RunID: "r1"}}, nil
				}
			},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 1 {
					t.Fatalf("expected 1 point, got %d", len(points))
				}
			},
		},
		{
			name:      "default_limit_when_negative",
			agentID:   "agent-1",
			datasetID: "",
			limit:     -5,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, _, _ string, limit int) ([]TrendPoint, error) {
					if limit != 30 {
						return nil, errors.New("limit not defaulted to 30")
					}
					return []TrendPoint{{RunID: "r1"}}, nil
				}
			},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 1 {
					t.Fatalf("expected 1 point, got %d", len(points))
				}
			},
		},
		{
			name:      "positive_limit_passed_through",
			agentID:   "agent-1",
			datasetID: "ds-1",
			limit:     10,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, agentID, datasetID string, limit int) ([]TrendPoint, error) {
					if agentID != "agent-1" {
						return nil, errors.New("agentID not passed")
					}
					if datasetID != "ds-1" {
						return nil, errors.New("datasetID not passed")
					}
					if limit != 10 {
						return nil, errors.New("limit not passed")
					}
					return []TrendPoint{
						{RunID: "r1", ExactMatchScore: 0.8},
						{RunID: "r2", ExactMatchScore: 0.9},
					}, nil
				}
			},
			check: func(t *testing.T, points []TrendPoint) {
				if len(points) != 2 {
					t.Fatalf("expected 2 points, got %d", len(points))
				}
			},
		},
		{
			name:    "repo_error",
			agentID: "agent-1",
			limit:   10,
			setup: func(r *fnMockRepo) {
				r.listTrendPointsFn = func(_ context.Context, _, _ string, _ int) ([]TrendPoint, error) {
					return nil, apierror.Internal("EVAL", "db error")
				}
			},
			wantErr: true,
			check:   func(t *testing.T, _ []TrendPoint) {},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			repo := &fnMockRepo{}
			if tt.setup != nil {
				tt.setup(repo)
			}
			uc := NewUsecase(StoresFrom(repo), loggateway.NewNoop())
			points, err := uc.GetAgentEvalTrend(context.Background(), tt.agentID, tt.datasetID, tt.limit)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				se, ok := apierror.From(err)
				if !ok {
					t.Fatalf("expected apierror, got %T", err)
				}
				if tt.agentID == "" || tt.agentID == "   " {
					if se.Domain != "EVAL" {
						t.Fatalf("expected domain 'EVAL', got %q", se.Domain)
					}
					if se.Code != apierror.CodeBadRequest {
						t.Fatalf("expected code %s, got %s", apierror.CodeBadRequest, se.Code)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, points)
			}
		})
	}
}

func boolPtr(b bool) *bool {
	return &b
}
