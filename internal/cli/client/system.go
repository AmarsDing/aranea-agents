package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
)

// SystemInfo holds the response from GET /v1/system/info.
type SystemInfo struct {
	Version             string            `json:"version"`
	GitCommit           string            `json:"git_commit"`
	BuildTime           string            `json:"build_time"`
	DefaultProvider     string            `json:"default_provider"`
	DefaultModel        string            `json:"default_model"`
	SystemAdminAgentID  string            `json:"system_admin_agent_id"`
	SystemAdminAgentKey string            `json:"system_admin_agent_key"`
	SkillMaxZipMB       int               `json:"skill_max_zip_mb"`
	SkillStorageRoot    string            `json:"skill_storage_root"`
	Features            map[string]string `json:"features"`
}

// GetSystemInfo calls GET /v1/system/info (requires auth).
func (c *Client) GetSystemInfo(ctx context.Context) (*SystemInfo, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/v1/system/info", nil)
	if err != nil {
		return nil, wrapNetErr(err)
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", c.UA)
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	if c.Debug {
		c.logRequest(req)
	}
	resp, err := c.Doer.Do(req)
	if err != nil {
		return nil, wrapNetErr(err)
	}
	defer resp.Body.Close()
	if c.Debug {
		c.logResponse(resp)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1024*1024))
	if err != nil {
		return nil, wrapNetErr(err)
	}
	if resp.StatusCode >= 300 {
		return nil, decodeErrorBody(body, resp.StatusCode)
	}

	var info SystemInfo
	if err := json.Unmarshal(body, &info); err != nil {
		return &info, nil
	}
	return &info, nil
}

// CheckReachability checks if the backend is reachable by calling GET /healthz.
// Returns nil if reachable, a *cli.CLIError otherwise.
func (c *Client) CheckReachability(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.Base+"/healthz", nil)
	if err != nil {
		return wrapNetErr(err)
	}
	req.Header.Set("User-Agent", c.UA)
	resp, err := c.Doer.Do(req)
	if err != nil {
		return wrapNetErr(err)
	}
	resp.Body.Close()
	if resp.StatusCode >= 500 {
		return decodeErrorBody(nil, resp.StatusCode)
	}
	return nil
}
