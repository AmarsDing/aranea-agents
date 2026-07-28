package client

import (
	"bytes"
	"context"
	"fmt"
	"mime/multipart"
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

// ListSkillFiles calls GET /v1/skills/{id}/files.
func (c *Client) ListSkillFiles(ctx context.Context, id string) (*skillv1.ListSkillFilesResponse, error) {
	resp := &skillv1.ListSkillFilesResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/skills/"+id+"/files", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSkillFile calls GET /v1/skills/{id}/file?path=<path>.
func (c *Client) GetSkillFile(ctx context.Context, id, path string) (*skillv1.SkillFileContent, error) {
	resp := &skillv1.SkillFileContent{}
	urlPath := "/v1/skills/" + id + "/file?path=" + url.QueryEscape(path)
	if err := c.Do(ctx, http.MethodGet, urlPath, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateSkillFile calls PUT /v1/skills/{id}/file.
func (c *Client) UpdateSkillFile(ctx context.Context, id, path, content string) (*skillv1.SkillFileContent, error) {
	req := &skillv1.UpdateSkillFileRequest{Id: id, Path: path, Content: content}
	resp := &skillv1.SkillFileContent{}
	if err := c.Do(ctx, http.MethodPut, "/v1/skills/"+id+"/file", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// DeleteSkillFile calls POST /v1/skills/{id}/files:delete.
func (c *Client) DeleteSkillFile(ctx context.Context, id, path string) error {
	req := &skillv1.DeleteSkillFileRequest{Id: id, Path: path}
	return c.Do(ctx, http.MethodPost, "/v1/skills/"+id+"/files:delete", req, nil)
}

// ImportSkillZip calls POST /v1/skills/import with a multipart/form-data ZIP
// upload (form field "file"). The endpoint is registered manually on the
// server (not proto codegen) and responds with plain JSON {"job_id": "..."}.
func (c *Client) ImportSkillZip(ctx context.Context, filename string, data []byte) (*skillv1.ImportSkillZipResponse, error) {
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		return nil, fmt.Errorf("build multipart: %w", err)
	}
	if _, err := part.Write(data); err != nil {
		return nil, fmt.Errorf("write multipart: %w", err)
	}
	if err := mw.Close(); err != nil {
		return nil, fmt.Errorf("close multipart: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.Base+"/v1/skills/import", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
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
	resp := &skillv1.ImportSkillZipResponse{}
	if err := decode(httpResp, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetSkillImportJob calls GET /v1/skills/import/{job_id}.
func (c *Client) GetSkillImportJob(ctx context.Context, jobID string) (*skillv1.SkillImportJob, error) {
	resp := &skillv1.SkillImportJob{}
	if err := c.Do(ctx, http.MethodGet, "/v1/skills/import/"+jobID, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ApplySkillImport calls POST /v1/skills/import/{job_id}/apply.
func (c *Client) ApplySkillImport(ctx context.Context, jobID string, decisions []*skillv1.SkillImportDecision) (*skillv1.SkillImportApplyResult, error) {
	req := &skillv1.ApplySkillImportRequest{JobId: jobID, Decisions: decisions}
	resp := &skillv1.SkillImportApplyResult{}
	if err := c.Do(ctx, http.MethodPost, "/v1/skills/import/"+jobID+"/apply", req, resp); err != nil {
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
