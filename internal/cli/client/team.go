package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	teamv1 "aranea-agents/api/kratos/team/v1"
)

// ListTeams calls GET /v1/teams.
func (c *Client) ListTeams(ctx context.Context) (*teamv1.ListTeamsResponse, error) {
	resp := &teamv1.ListTeamsResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/teams", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetTeam calls GET /v1/teams/{id}.
func (c *Client) GetTeam(ctx context.Context, id string) (*teamv1.Team, error) {
	resp := &teamv1.Team{}
	if err := c.Do(ctx, http.MethodGet, "/v1/teams/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateTeam calls POST /v1/teams.
func (c *Client) CreateTeam(ctx context.Context, req *teamv1.CreateTeamRequest) (*teamv1.Team, error) {
	resp := &teamv1.Team{}
	if err := c.Do(ctx, http.MethodPost, "/v1/teams", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateTeam calls PATCH /v1/teams/{id} with body: "team".
func (c *Client) UpdateTeam(ctx context.Context, id string, team *teamv1.Team) (*teamv1.Team, error) {
	resp := &teamv1.Team{}
	b, err := marshalOpts.Marshal(team)
	if err != nil {
		return nil, fmt.Errorf("marshal team: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+"/v1/teams/"+id, bytes.NewReader(b))
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

// DeleteTeam calls DELETE /v1/teams/{id}.
func (c *Client) DeleteTeam(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/teams/"+id, nil, nil)
}

// RunTeamTest calls POST /v1/teams/{id}/run-test.
func (c *Client) RunTeamTest(ctx context.Context, id, content string) (*teamv1.RunTeamTestResponse, error) {
	resp := &teamv1.RunTeamTestResponse{}
	if err := c.Do(ctx, http.MethodPost, "/v1/teams/"+id+"/run-test",
		&teamv1.RunTeamTestRequest{Id: id, Content: content}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListTeamRuns calls GET /v1/team-runs.
func (c *Client) ListTeamRuns(ctx context.Context, teamID string, limit int32) (*teamv1.ListTeamRunsResponse, error) {
	resp := &teamv1.ListTeamRunsResponse{}
	path := "/v1/team-runs"
	if teamID != "" {
		path += "?team_id=" + teamID
	}
	if limit > 0 {
		if teamID != "" {
			path += fmt.Sprintf("&limit=%d", limit)
		} else {
			path += fmt.Sprintf("?limit=%d", limit)
		}
	}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetTeamRun calls GET /v1/team-runs/{id}.
func (c *Client) GetTeamRun(ctx context.Context, id string) (*teamv1.TeamRun, error) {
	resp := &teamv1.TeamRun{}
	if err := c.Do(ctx, http.MethodGet, "/v1/team-runs/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
