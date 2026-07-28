package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	taxonomyv1 "aranea-agents/api/kratos/taxonomy/v1"
)

// ListTaxonomy calls GET /v1/taxonomy.
func (c *Client) ListTaxonomy(ctx context.Context) (*taxonomyv1.ListTaxonomyResponse, error) {
	resp := &taxonomyv1.ListTaxonomyResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/taxonomy", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListTaxonomyTree calls GET /v1/taxonomy/tree.
func (c *Client) ListTaxonomyTree(ctx context.Context) (*taxonomyv1.ListTaxonomyTreeResponse, error) {
	resp := &taxonomyv1.ListTaxonomyTreeResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/taxonomy/tree", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetTaxonomy calls GET /v1/taxonomy/{id}.
func (c *Client) GetTaxonomy(ctx context.Context, id string) (*taxonomyv1.TaxonomyNode, error) {
	resp := &taxonomyv1.TaxonomyNode{}
	if err := c.Do(ctx, http.MethodGet, "/v1/taxonomy/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateTaxonomy calls POST /v1/taxonomy.
func (c *Client) CreateTaxonomy(ctx context.Context, req *taxonomyv1.CreateTaxonomyRequest) (*taxonomyv1.TaxonomyNode, error) {
	resp := &taxonomyv1.TaxonomyNode{}
	if err := c.Do(ctx, http.MethodPost, "/v1/taxonomy", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateTaxonomy calls PATCH /v1/taxonomy/{id} with body: "node".
func (c *Client) UpdateTaxonomy(ctx context.Context, id string, node *taxonomyv1.TaxonomyNode) (*taxonomyv1.TaxonomyNode, error) {
	resp := &taxonomyv1.TaxonomyNode{}
	b, err := marshalOpts.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("marshal taxonomy node: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+"/v1/taxonomy/"+id, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	httpResp, err := c.Doer.Do(req)
	if err != nil {
		return nil, wrapNetErr(err)
	}
	defer httpResp.Body.Close()
	if err := decode(httpResp, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteTaxonomy calls DELETE /v1/taxonomy/{id}.
func (c *Client) DeleteTaxonomy(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/taxonomy/"+id, nil, nil)
}

// ReorderTaxonomy calls PUT /v1/taxonomy/reorder.
func (c *Client) ReorderTaxonomy(ctx context.Context, ids []string) error {
	req := &taxonomyv1.ReorderTaxonomyRequest{Ids: ids}
	resp := &taxonomyv1.ReorderTaxonomyResponse{}
	if err := c.Do(ctx, http.MethodPut, "/v1/taxonomy/reorder", req, resp); err != nil {
		return err
	}
	return nil
}
