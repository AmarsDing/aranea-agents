package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	organizationv1 "aranea-agents/api/kratos/organization/v1"
)

// ListOrganization calls GET /v1/organization.
func (c *Client) ListOrganization(ctx context.Context) (*organizationv1.ListOrganizationResponse, error) {
	resp := &organizationv1.ListOrganizationResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/organization", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListOrganizationTree calls GET /v1/organization/tree.
func (c *Client) ListOrganizationTree(ctx context.Context) (*organizationv1.ListOrganizationTreeResponse, error) {
	resp := &organizationv1.ListOrganizationTreeResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/organization/tree", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetOrganization calls GET /v1/organization/{id}.
func (c *Client) GetOrganization(ctx context.Context, id string) (*organizationv1.OrganizationNode, error) {
	resp := &organizationv1.OrganizationNode{}
	if err := c.Do(ctx, http.MethodGet, "/v1/organization/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateOrganization calls POST /v1/organization.
func (c *Client) CreateOrganization(ctx context.Context, req *organizationv1.CreateOrganizationRequest) (*organizationv1.OrganizationNode, error) {
	resp := &organizationv1.OrganizationNode{}
	if err := c.Do(ctx, http.MethodPost, "/v1/organization", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateOrganization calls PATCH /v1/organization/{id} with body: "node".
func (c *Client) UpdateOrganization(ctx context.Context, id string, node *organizationv1.OrganizationNode) (*organizationv1.OrganizationNode, error) {
	resp := &organizationv1.OrganizationNode{}
	b, err := marshalOpts.Marshal(node)
	if err != nil {
		return nil, fmt.Errorf("marshal organization node: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+"/v1/organization/"+id, bytes.NewReader(b))
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

// DeleteOrganization calls DELETE /v1/organization/{id}.
func (c *Client) DeleteOrganization(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/organization/"+id, nil, nil)
}

// ReorderOrganization calls PUT /v1/organization/reorder.
func (c *Client) ReorderOrganization(ctx context.Context, ids []string) error {
	req := &organizationv1.ReorderOrganizationRequest{Ids: ids}
	resp := &organizationv1.ReorderOrganizationResponse{}
	if err := c.Do(ctx, http.MethodPut, "/v1/organization/reorder", req, resp); err != nil {
		return err
	}
	return nil
}
