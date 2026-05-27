package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	sessionv1 "aranea-agents/api/kratos/session/v1"
	chatv1 "aranea-agents/api/kratos/chat/v1"
)

// SearchSessions calls GET /v1/sessions with filter params.
func (c *Client) SearchSessions(ctx context.Context, agentID string, limit, offset int32) (*sessionv1.SearchSessionsResponse, error) {
	params := url.Values{}
	if agentID != "" {
		params.Set("agent_id", agentID)
	}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	path := "/v1/sessions"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &sessionv1.SearchSessionsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSession calls GET /v1/sessions/{id}.
func (c *Client) GetSession(ctx context.Context, id string) (*sessionv1.Session, error) {
	resp := &sessionv1.Session{}
	if err := c.Do(ctx, http.MethodGet, "/v1/sessions/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateSession calls POST /v1/sessions.
func (c *Client) CreateSession(ctx context.Context, req *sessionv1.CreateSessionRequest) (*sessionv1.Session, error) {
	resp := &sessionv1.Session{}
	if err := c.Do(ctx, http.MethodPost, "/v1/sessions", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteSession calls DELETE /v1/sessions/{id}.
func (c *Client) DeleteSession(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/sessions/"+id, nil, nil)
}

// SendChatMessage calls POST /v1/chat/messages.
func (c *Client) SendChatMessage(ctx context.Context, req *chatv1.SendChatMessageRequest) (*chatv1.SendChatMessageResponse, error) {
	resp := &chatv1.SendChatMessageResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/chat/messages", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListSessionMessages calls GET /v1/sessions/{id}/messages.
func (c *Client) ListSessionMessages(ctx context.Context, sessionID string, limit int32) (*sessionv1.ListSessionMessagesResponse, error) {
	path := fmt.Sprintf("/v1/sessions/%s/messages", sessionID)
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}
	resp := &sessionv1.ListSessionMessagesResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
