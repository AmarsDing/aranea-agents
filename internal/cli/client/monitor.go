package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	monitorv1 "aranea-agents/api/kratos/monitor/v1"
)

// ListAuditLogs calls GET /v1/monitor/audit.
func (c *Client) ListAuditLogs(ctx context.Context, limit, offset int32, action, resource, actor, keyword string) (*monitorv1.ListAuditLogsResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	if action != "" {
		params.Set("action", action)
	}
	if resource != "" {
		params.Set("resource", resource)
	}
	if actor != "" {
		params.Set("actor", actor)
	}
	if keyword != "" {
		params.Set("keyword", keyword)
	}
	path := "/v1/monitor/audit"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &monitorv1.ListAuditLogsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListMonitorEvents calls GET /v1/monitor/events.
func (c *Client) ListMonitorEvents(ctx context.Context, limit, offset int32, eventType, agentID, status string) (*monitorv1.ListMonitorEventsResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	if eventType != "" {
		params.Set("event_type", eventType)
	}
	if agentID != "" {
		params.Set("agent_id", agentID)
	}
	if status != "" {
		params.Set("status", status)
	}
	path := "/v1/monitor/events"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &monitorv1.ListMonitorEventsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListMonitorTraces calls GET /v1/monitor/traces.
func (c *Client) ListMonitorTraces(ctx context.Context, limit, offset int32, agentID, provider, model, status string) (*monitorv1.ListMonitorTracesResponse, error) {
	params := url.Values{}
	if limit > 0 {
		params.Set("limit", fmt.Sprintf("%d", limit))
	}
	if offset > 0 {
		params.Set("offset", fmt.Sprintf("%d", offset))
	}
	if agentID != "" {
		params.Set("agent_id", agentID)
	}
	if provider != "" {
		params.Set("provider", provider)
	}
	if model != "" {
		params.Set("model", model)
	}
	if status != "" {
		params.Set("status", status)
	}
	path := "/v1/monitor/traces"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &monitorv1.ListMonitorTracesResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
