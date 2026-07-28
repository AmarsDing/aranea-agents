package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	a2av1 "aranea-agents/api/kratos/a2a/v1"
)

// DiscoverA2ARemoteAgent calls POST /v1/a2a/remote-discover.
func (c *Client) DiscoverA2ARemoteAgent(ctx context.Context, remoteURL, authType, authConfigJSON string) (*a2av1.A2AAgentCard, error) {
	req := &a2av1.DiscoverRemoteAgentRequest{
		RemoteUrl:      remoteURL,
		AuthType:       authType,
		AuthConfigJson: authConfigJSON,
	}
	resp := &a2av1.A2AAgentCard{}
	if err := c.Do(ctx, http.MethodPost, "/v1/a2a/remote-discover", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListA2ARemoteAgents calls GET /v1/a2a/remote-agents.
func (c *Client) ListA2ARemoteAgents(ctx context.Context, workspace string) (*a2av1.ListRemoteAgentsResponse, error) {
	path := "/v1/a2a/remote-agents"
	if workspace != "" {
		path += "?workspace=" + url.QueryEscape(workspace)
	}
	resp := &a2av1.ListRemoteAgentsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// RegisterA2ARemoteAgent calls POST /v1/a2a/remote-agents.
func (c *Client) RegisterA2ARemoteAgent(ctx context.Context, req *a2av1.RegisterRemoteAgentRequest) (*a2av1.A2ARemoteAgent, error) {
	resp := &a2av1.A2ARemoteAgent{}
	if err := c.Do(ctx, http.MethodPost, "/v1/a2a/remote-agents", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteA2ARemoteAgent calls DELETE /v1/a2a/remote-agents/{id}.
func (c *Client) DeleteA2ARemoteAgent(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/a2a/remote-agents/"+id, nil, nil)
}

// ListA2AAudit calls GET /v1/a2a/audit.
func (c *Client) ListA2AAudit(ctx context.Context, callerAgentID, calleeAgentID string, limit, offset int32) (*a2av1.ListAuditResponse, error) {
	params := url.Values{}
	if callerAgentID != "" {
		params.Set("caller_agent_id", callerAgentID)
	}
	if calleeAgentID != "" {
		params.Set("callee_agent_id", calleeAgentID)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/a2a/audit"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &a2av1.ListAuditResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetA2AConfig calls GET /v1/a2a/config.
func (c *Client) GetA2AConfig(ctx context.Context) (*a2av1.A2ARuntimeConfig, error) {
	resp := &a2av1.A2ARuntimeConfig{}
	if err := c.Do(ctx, http.MethodGet, "/v1/a2a/config", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
