package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	evaluationv1 "aranea-agents/api/kratos/evaluation/v1"
)

// ListDatasets calls GET /v1/evaluation/datasets.
func (c *Client) ListDatasets(ctx context.Context, limit, offset int32) (*evaluationv1.ListDatasetsResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/evaluation/datasets"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &evaluationv1.ListDatasetsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetDataset calls GET /v1/evaluation/datasets/{id}.
func (c *Client) GetDataset(ctx context.Context, id string) (*evaluationv1.EvalDataset, error) {
	resp := &evaluationv1.EvalDataset{}
	if err := c.Do(ctx, http.MethodGet, "/v1/evaluation/datasets/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateDataset calls POST /v1/evaluation/datasets.
func (c *Client) CreateDataset(ctx context.Context, req *evaluationv1.CreateDatasetRequest) (*evaluationv1.EvalDataset, error) {
	resp := &evaluationv1.EvalDataset{}
	if err := c.Do(ctx, http.MethodPost, "/v1/evaluation/datasets", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListEvalRuns calls GET /v1/evaluation/runs.
func (c *Client) ListEvalRuns(ctx context.Context, datasetID, agentID string, limit, offset int32) (*evaluationv1.ListRunsResponse, error) {
	params := url.Values{}
	if datasetID != "" {
		params.Set("dataset_id", datasetID)
	}
	if agentID != "" {
		params.Set("agent_id", agentID)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/evaluation/runs"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &evaluationv1.ListRunsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetEvalRun calls GET /v1/evaluation/runs/{id}.
func (c *Client) GetEvalRun(ctx context.Context, id string) (*evaluationv1.EvalRun, error) {
	resp := &evaluationv1.EvalRun{}
	if err := c.Do(ctx, http.MethodGet, "/v1/evaluation/runs/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RunEvaluation calls POST /v1/evaluation/runs.
func (c *Client) RunEvaluation(ctx context.Context, req *evaluationv1.RunEvaluationRequest) (*evaluationv1.EvalRun, error) {
	resp := &evaluationv1.EvalRun{}
	if err := c.Do(ctx, http.MethodPost, "/v1/evaluation/runs", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetRunResults calls GET /v1/evaluation/runs/{run_id}/results.
func (c *Client) GetRunResults(ctx context.Context, runID string, limit, offset int32) (*evaluationv1.GetRunResultsResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/evaluation/runs/" + runID + "/results"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &evaluationv1.GetRunResultsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
