package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	toolv1 "aranea-agents/api/kratos/tool/v1"
)

// ListTools calls GET /v1/tools.
func (c *Client) ListTools(ctx context.Context, keyword string, limit, offset int32) (*toolv1.ListToolsResponse, error) {
	params := url.Values{}
	if keyword != "" {
		params.Set("keyword", keyword)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/tools"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &toolv1.ListToolsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetTool calls GET /v1/tools/{id}.
func (c *Client) GetTool(ctx context.Context, id string) (*toolv1.Tool, error) {
	resp := &toolv1.Tool{}
	if err := c.Do(ctx, http.MethodGet, "/v1/tools/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ToggleToolEnabled calls PATCH /v1/tools/{id}/enabled.
func (c *Client) ToggleToolEnabled(ctx context.Context, id string, enabled bool, confirmKey string) (*toolv1.Tool, error) {
	req := &toolv1.ToggleToolEnabledRequest{Id: id, Enabled: enabled, ConfirmKey: confirmKey}
	resp := &toolv1.Tool{}
	b, err := marshalOpts.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+fmt.Sprintf("/v1/tools/%s/enabled", id), bytes.NewReader(b))
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.Token)
	}
	httpResp, err := c.Doer.Do(httpReq)
	if err != nil {
		return nil, wrapNetErr(err)
	}
	defer httpResp.Body.Close()
	if err := decode(httpResp, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
