package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	agentv1 "aranea-agents/api/kratos/agent/v1"
)

// ListAgents calls GET /v1/agents and returns a paginated list of agents.
func (c *Client) ListAgents(ctx context.Context, keyword string, limit, offset int32) (*agentv1.ListAgentsResponse, error) {
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
	path := "/v1/agents"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &agentv1.ListAgentsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetAgent calls GET /v1/agents/{id}.
func (c *Client) GetAgent(ctx context.Context, id string) (*agentv1.Agent, error) {
	resp := &agentv1.Agent{}
	if err := c.Do(ctx, http.MethodGet, "/v1/agents/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateAgent calls POST /v1/agents.
func (c *Client) CreateAgent(ctx context.Context, req *agentv1.CreateAgentRequest) (*agentv1.Agent, error) {
	resp := &agentv1.Agent{}
	if err := c.Do(ctx, http.MethodPost, "/v1/agents", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateAgent calls PATCH /v1/agents/{id}.
// Note: the HTTP annotation has body: "agent", so we pass the agent fields.
func (c *Client) UpdateAgent(ctx context.Context, id string, agent *agentv1.Agent) (*agentv1.Agent, error) {
	resp := &agentv1.Agent{}
	// Build a raw PATCH since body is "agent" not "*".
	b, err := marshalOpts.Marshal(agent)
	if err != nil {
		return nil, fmt.Errorf("marshal agent: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+"/v1/agents/"+id,
		bytes.NewReader(b))
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

// DeleteAgent calls DELETE /v1/agents/{id}.
func (c *Client) DeleteAgent(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/agents/"+id, nil, nil)
}

// EnableAgent enables an agent by setting status="active" via UpdateAgent.
// Code archaeology A5: No dedicated enable/disable RPC in agent.proto.
// Uses PATCH /v1/agents/{id} with status field.
func (c *Client) EnableAgent(ctx context.Context, id string) (*agentv1.Agent, error) {
	return c.UpdateAgent(ctx, id, &agentv1.Agent{Status: "active"})
}

// DisableAgent disables an agent by setting status="inactive" via UpdateAgent.
func (c *Client) DisableAgent(ctx context.Context, id string) (*agentv1.Agent, error) {
	return c.UpdateAgent(ctx, id, &agentv1.Agent{Status: "inactive"})
}

// GetAgentEffectiveTools calls GET /v1/agents/{agent_id}/tools/effective.
func (c *Client) GetAgentEffectiveTools(ctx context.Context, agentID string) (*agentv1.AgentEffectiveToolsView, error) {
	resp := &agentv1.AgentEffectiveToolsView{}
	if err := c.Do(ctx, http.MethodGet, "/v1/agents/"+agentID+"/tools/effective", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateAgentToolPolicy calls PUT /v1/agents/{agent_id}/tools/policy.
func (c *Client) UpdateAgentToolPolicy(ctx context.Context, agentID string, req *agentv1.UpdateAgentToolPolicyRequest) (*agentv1.AgentEffectiveToolsView, error) {
	resp := &agentv1.AgentEffectiveToolsView{}
	// Build raw PUT since this has a specific path.
	b, err := marshalOpts.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPut,
		c.Base+"/v1/agents/"+agentID+"/tools/policy", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
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
