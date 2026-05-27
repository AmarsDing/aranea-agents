package client

import (
	"context"
	"fmt"
	"net/http"

	channelv1 "aranea-agents/api/kratos/channel/v1"

	"google.golang.org/protobuf/types/known/emptypb"
)

// ListChannels calls GET /v1/channels.
func (c *Client) ListChannels(ctx context.Context) (*channelv1.ListChannelsResponse, error) {
	resp := &channelv1.ListChannelsResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/channels", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetChannel calls GET /v1/channels/{id}.
func (c *Client) GetChannel(ctx context.Context, id string) (*channelv1.Channel, error) {
	resp := &channelv1.Channel{}
	if err := c.Do(ctx, http.MethodGet, "/v1/channels/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateChannel calls POST /v1/channels.
func (c *Client) CreateChannel(ctx context.Context, req *channelv1.CreateChannelRequest) (*channelv1.Channel, error) {
	resp := &channelv1.Channel{}
	if err := c.Do(ctx, http.MethodPost, "/v1/channels", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateChannel calls PATCH /v1/channels/{id}.
func (c *Client) UpdateChannel(ctx context.Context, id string, req *channelv1.UpdateChannelRequest) (*channelv1.Channel, error) {
	resp := &channelv1.Channel{}
	if err := c.Do(ctx, http.MethodPatch, "/v1/channels/"+id, req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteChannel calls DELETE /v1/channels/{id}.
func (c *Client) DeleteChannel(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/channels/"+id, nil, nil)
}

// TestChannel calls POST /v1/channels/{id}/test.
func (c *Client) TestChannel(ctx context.Context, id string) (*channelv1.ChannelTestResult, error) {
	resp := &channelv1.ChannelTestResult{}
	if err := c.Do(ctx, http.MethodPost, "/v1/channels/"+id+"/test", &emptypb.Empty{}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ToggleChannel calls POST /v1/channels/{id}/toggle.
func (c *Client) ToggleChannel(ctx context.Context, id string, enabled bool) (*channelv1.Channel, error) {
	resp := &channelv1.Channel{}
	if err := c.Do(ctx, http.MethodPost, "/v1/channels/"+id+"/toggle",
		&channelv1.ToggleChannelRequest{Id: id, Enabled: enabled}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListChannelDeliveries calls GET /v1/channels/{id}/deliveries.
func (c *Client) ListChannelDeliveries(ctx context.Context, id string, limit int32) (*channelv1.ListChannelDeliveriesResponse, error) {
	path := fmt.Sprintf("/v1/channels/%s/deliveries", id)
	if limit > 0 {
		path += fmt.Sprintf("?limit=%d", limit)
	}
	resp := &channelv1.ListChannelDeliveriesResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
