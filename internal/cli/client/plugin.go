package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"

	pluginv1 "aranea-agents/api/kratos/plugin/v1"
)

// ListPlugins calls GET /v1/plugins.
func (c *Client) ListPlugins(ctx context.Context, search, category string, page, pageSize int32) (*pluginv1.ListPluginsResponse, error) {
	params := url.Values{}
	if search != "" {
		params.Set("search", search)
	}
	if category != "" {
		params.Set("category", category)
	}
	if page > 0 {
		params.Set("page", fmt.Sprintf("%d", page))
	}
	if pageSize > 0 {
		params.Set("page_size", fmt.Sprintf("%d", pageSize))
	}
	path := "/v1/plugins"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &pluginv1.ListPluginsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// TogglePluginEnabled calls PATCH /v1/plugins/{id}/enabled.
func (c *Client) TogglePluginEnabled(ctx context.Context, id string, enabled bool) (*pluginv1.Plugin, error) {
	resp := &pluginv1.Plugin{}
	if err := c.Do(ctx, http.MethodPatch, "/v1/plugins/"+id+"/enabled",
		&pluginv1.TogglePluginEnabledRequest{Id: id, Enabled: enabled}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdatePluginConfig calls PUT /v1/plugins/{id}/config.
func (c *Client) UpdatePluginConfig(ctx context.Context, id, configJSON string) (*pluginv1.Plugin, error) {
	resp := &pluginv1.Plugin{}
	if err := c.Do(ctx, http.MethodPut, "/v1/plugins/"+id+"/config",
		&pluginv1.UpdatePluginConfigRequest{Id: id, ConfigJson: configJSON}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdatePluginSortOrder calls PATCH /v1/plugins/{id}/sort-order.
func (c *Client) UpdatePluginSortOrder(ctx context.Context, id string, sortOrder int32) (*pluginv1.Plugin, error) {
	resp := &pluginv1.Plugin{}
	if err := c.Do(ctx, http.MethodPatch, "/v1/plugins/"+id+"/sort-order",
		&pluginv1.UpdatePluginSortOrderRequest{Id: id, SortOrder: sortOrder}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListPluginRuns calls GET /v1/plugins/runs.
func (c *Client) ListPluginRuns(ctx context.Context, pluginKey string, page, pageSize int32) (*pluginv1.ListPluginRunsResponse, error) {
	params := url.Values{}
	if pluginKey != "" {
		params.Set("plugin_key", pluginKey)
	}
	if page > 0 {
		params.Set("page", fmt.Sprintf("%d", page))
	}
	if pageSize > 0 {
		params.Set("page_size", fmt.Sprintf("%d", pageSize))
	}
	path := "/v1/plugins/runs"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &pluginv1.ListPluginRunsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
