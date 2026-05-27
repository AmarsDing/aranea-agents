package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"

	skillv1 "aranea-agents/api/kratos/skill/v1"
)

// ListSkills calls GET /v1/skills.
func (c *Client) ListSkills(ctx context.Context, keyword string, limit, offset int32) (*skillv1.ListSkillsResponse, error) {
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
	path := "/v1/skills"
	if len(params) > 0 {
		path += "?" + params.Encode()
	}
	resp := &skillv1.ListSkillsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSkill calls GET /v1/skills/{id}.
func (c *Client) GetSkill(ctx context.Context, id string) (*skillv1.GetSkillResponse, error) {
	resp := &skillv1.GetSkillResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/skills/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateSkill calls POST /v1/skills.
func (c *Client) CreateSkill(ctx context.Context, req *skillv1.CreateSkillRequest) (*skillv1.Skill, error) {
	resp := &skillv1.Skill{}
	if err := c.Do(ctx, http.MethodPost, "/v1/skills", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateSkill calls PATCH /v1/skills/{id}.
func (c *Client) UpdateSkill(ctx context.Context, id string, req *skillv1.UpdateSkillRequest) (*skillv1.Skill, error) {
	resp := &skillv1.Skill{}
	b, err := marshalOpts.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+"/v1/skills/"+id, bytes.NewReader(b))
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

// DeleteSkill calls DELETE /v1/skills/{id}.
func (c *Client) DeleteSkill(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/skills/"+id, nil, nil)
}

// ToggleSkillEnabled calls PATCH /v1/skills/{id}/enabled.
func (c *Client) ToggleSkillEnabled(ctx context.Context, id string, enabled bool) (*skillv1.Skill, error) {
	req := &skillv1.ToggleSkillEnabledRequest{Id: id, Enabled: enabled}
	resp := &skillv1.Skill{}
	b, err := marshalOpts.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+fmt.Sprintf("/v1/skills/%s/enabled", id), bytes.NewReader(b))
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

// PublishSkill calls POST /v1/skills/{id}/publish.
func (c *Client) PublishSkill(ctx context.Context, id string) (*skillv1.Skill, error) {
	req := &skillv1.PublishSkillRequest{Id: id}
	resp := &skillv1.Skill{}
	b, err := marshalOpts.Marshal(req)
	if err != nil {
		return nil, err
	}
	httpReq, _ := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+fmt.Sprintf("/v1/skills/%s/publish", id), bytes.NewReader(b))
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
