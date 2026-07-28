package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	modelcatalogv1 "aranea-agents/api/kratos/model_catalog/v1"
)

// ListCatalogProviders calls GET /v1/model-catalog/providers.
func (c *Client) ListCatalogProviders(ctx context.Context, q string, limit, offset int32) (*modelcatalogv1.ListCatalogProvidersResponse, error) {
	params := url.Values{}
	if q != "" {
		params.Set("q", q)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/model-catalog/providers"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &modelcatalogv1.ListCatalogProvidersResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListCatalogModels calls GET /v1/model-catalog/providers/{provider_id}/models.
func (c *Client) ListCatalogModels(ctx context.Context, providerID, q string, includeDeprecated bool, limit, offset int32) (*modelcatalogv1.ListCatalogModelsResponse, error) {
	params := url.Values{}
	if q != "" {
		params.Set("q", q)
	}
	if includeDeprecated {
		params.Set("include_deprecated", "true")
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/model-catalog/providers/" + providerID + "/models"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &modelcatalogv1.ListCatalogModelsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetModelCatalogPolicy calls GET /v1/model-catalog/policy.
func (c *Client) GetModelCatalogPolicy(ctx context.Context) (*modelcatalogv1.ModelCatalogPolicy, error) {
	resp := &modelcatalogv1.ModelCatalogPolicy{}
	if err := c.Do(ctx, http.MethodGet, "/v1/model-catalog/policy", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateModelCatalogPolicy calls PUT /v1/model-catalog/policy.
func (c *Client) UpdateModelCatalogPolicy(ctx context.Context, req *modelcatalogv1.UpdateModelCatalogPolicyRequest) (*modelcatalogv1.ModelCatalogPolicy, error) {
	resp := &modelcatalogv1.ModelCatalogPolicy{}
	if err := c.Do(ctx, http.MethodPut, "/v1/model-catalog/policy", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// SyncModelCatalog calls POST /v1/model-catalog/sync.
func (c *Client) SyncModelCatalog(ctx context.Context, dryRun bool) (*modelcatalogv1.SyncModelCatalogResponse, error) {
	req := &modelcatalogv1.SyncModelCatalogRequest{DryRun: dryRun}
	resp := &modelcatalogv1.SyncModelCatalogResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/model-catalog/sync", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
