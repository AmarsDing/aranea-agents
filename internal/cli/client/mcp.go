package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	mcpv1 "aranea-agents/api/kratos/mcp_server/v1"

	"google.golang.org/protobuf/types/known/emptypb"
)

// ListMCPServers calls GET /v1/mcp-servers.
func (c *Client) ListMCPServers(ctx context.Context) (*mcpv1.ListMCPServersResponse, error) {
	resp := &mcpv1.ListMCPServersResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/mcp-servers", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetMCPServer calls GET /v1/mcp-servers/{id}.
func (c *Client) GetMCPServer(ctx context.Context, id string) (*mcpv1.MCPServer, error) {
	resp := &mcpv1.MCPServer{}
	if err := c.Do(ctx, http.MethodGet, "/v1/mcp-servers/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateMCPServer calls POST /v1/mcp-servers.
func (c *Client) CreateMCPServer(ctx context.Context, req *mcpv1.CreateMCPServerRequest) (*mcpv1.MCPServer, error) {
	resp := &mcpv1.MCPServer{}
	if err := c.Do(ctx, http.MethodPost, "/v1/mcp-servers", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateMCPServer calls PATCH /v1/mcp-servers/{id} with body: "mcp_server".
func (c *Client) UpdateMCPServer(ctx context.Context, id string, mcp *mcpv1.MCPServer) (*mcpv1.MCPServer, error) {
	resp := &mcpv1.MCPServer{}
	b, err := marshalOpts.Marshal(mcp)
	if err != nil {
		return nil, fmt.Errorf("marshal mcp_server: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+"/v1/mcp-servers/"+id, bytes.NewReader(b))
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

// DeleteMCPServer calls DELETE /v1/mcp-servers/{id}.
func (c *Client) DeleteMCPServer(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/mcp-servers/"+id, nil, nil)
}

// TestMCPServer calls POST /v1/mcp-servers/{id}/test.
func (c *Client) TestMCPServer(ctx context.Context, id string) (*mcpv1.MCPServerTestResponse, error) {
	resp := &mcpv1.MCPServerTestResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/mcp-servers/"+id+"/test", &emptypb.Empty{}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ValidateMCPServer calls POST /v1/mcp-servers/validate.
func (c *Client) ValidateMCPServer(ctx context.Context, req *mcpv1.ValidateMCPServerRequest) (*mcpv1.ValidateMCPServerResponse, error) {
	resp := &mcpv1.ValidateMCPServerResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/mcp-servers/validate", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
