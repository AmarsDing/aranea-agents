package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	chatv1 "aranea-agents/api/kratos/chat/v1"
	sessionv1 "aranea-agents/api/kratos/session/v1"
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

// ArchiveSession calls POST /v1/sessions/{id}/archive.
func (c *Client) ArchiveSession(ctx context.Context, id string) error {
	req := &sessionv1.ArchiveSessionRequest{Id: id}
	return c.Do(ctx, http.MethodPost, "/v1/sessions/"+id+"/archive", req, nil)
}

// RestoreSession calls POST /v1/sessions/{id}/restore.
func (c *Client) RestoreSession(ctx context.Context, id string) (*sessionv1.Session, error) {
	req := &sessionv1.RestoreSessionRequest{Id: id}
	resp := &sessionv1.Session{}
	if err := c.Do(ctx, http.MethodPost, "/v1/sessions/"+id+"/restore", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// PinSession calls POST /v1/sessions/{id}/pin.
func (c *Client) PinSession(ctx context.Context, id string) (*sessionv1.Session, error) {
	req := &sessionv1.PinSessionRequest{Id: id}
	resp := &sessionv1.Session{}
	if err := c.Do(ctx, http.MethodPost, "/v1/sessions/"+id+"/pin", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UnpinSession calls POST /v1/sessions/{id}/unpin.
func (c *Client) UnpinSession(ctx context.Context, id string) (*sessionv1.Session, error) {
	req := &sessionv1.UnpinSessionRequest{Id: id}
	resp := &sessionv1.Session{}
	if err := c.Do(ctx, http.MethodPost, "/v1/sessions/"+id+"/unpin", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CompactSession calls POST /v1/sessions:compact.
func (c *Client) CompactSession(ctx context.Context, sessionID, preserveInstruction string) (*sessionv1.CompactSessionResponse, error) {
	req := &sessionv1.CompactSessionRequest{SessionId: sessionID, PreserveInstruction: preserveInstruction}
	resp := &sessionv1.CompactSessionResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/sessions:compact", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ExportSession calls GET /v1/sessions/{id}/export?format=markdown|json.
func (c *Client) ExportSession(ctx context.Context, id, format string) (*sessionv1.ExportSessionResponse, error) {
	path := "/v1/sessions/" + id + "/export"
	if format != "" {
		path += "?format=" + url.QueryEscape(format)
	}
	resp := &sessionv1.ExportSessionResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
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
