package client

import (
	"context"
	"net/http"

	graphv1 "aranea-agents/api/kratos/graph/v1"
)

// ListGraphs calls GET /v1/graphs.
func (c *Client) ListGraphs(ctx context.Context) (*graphv1.ListGraphsResponse, error) {
	resp := &graphv1.ListGraphsResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/graphs", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetGraph calls GET /v1/graphs/{id}.
func (c *Client) GetGraph(ctx context.Context, id string) (*graphv1.GetGraphResponse, error) {
	resp := &graphv1.GetGraphResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/graphs/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateGraph calls POST /v1/graphs.
func (c *Client) CreateGraph(ctx context.Context, req *graphv1.CreateGraphRequest) (*graphv1.CreateGraphResponse, error) {
	resp := &graphv1.CreateGraphResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/graphs", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateGraph calls PUT /v1/graphs/{id}.
func (c *Client) UpdateGraph(ctx context.Context, id string, req *graphv1.UpdateGraphRequest) (*graphv1.UpdateGraphResponse, error) {
	resp := &graphv1.UpdateGraphResponse{}
	if err := c.Do(ctx, http.MethodPut, "/v1/graphs/"+id, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteGraph calls DELETE /v1/graphs/{id}.
func (c *Client) DeleteGraph(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/graphs/"+id, nil, nil)
}

// ImportGraph calls POST /v1/graph/import.
func (c *Client) ImportGraph(ctx context.Context, req *graphv1.ImportGraphRequest) (*graphv1.ImportGraphResponse, error) {
	resp := &graphv1.ImportGraphResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/graph/import", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ExportGraph calls GET /v1/graphs/{graph_id}/export.
func (c *Client) ExportGraph(ctx context.Context, graphID string) (*graphv1.ExportGraphResponse, error) {
	resp := &graphv1.ExportGraphResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/graphs/"+graphID+"/export", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListGraphExecutions calls GET /v1/graphs/{graph_id}/executions.
func (c *Client) ListGraphExecutions(ctx context.Context, graphID string) (*graphv1.ListGraphExecutionsResponse, error) {
	resp := &graphv1.ListGraphExecutionsResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/graphs/"+graphID+"/executions", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
