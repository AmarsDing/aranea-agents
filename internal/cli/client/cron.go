package client

import (
	"bytes"
	"context"
	"fmt"
	"net/http"

	cronv1 "aranea-agents/api/kratos/cron/v1"

	"google.golang.org/protobuf/types/known/emptypb"
)

// ListCronTasks calls GET /v1/cron-tasks.
func (c *Client) ListCronTasks(ctx context.Context) (*cronv1.ListCronTasksResponse, error) {
	resp := &cronv1.ListCronTasksResponse{}
	if err := c.Do(ctx, http.MethodGet, "/v1/cron-tasks", nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// GetCronTask calls GET /v1/cron-tasks/{id}.
func (c *Client) GetCronTask(ctx context.Context, id string) (*cronv1.CronTask, error) {
	resp := &cronv1.CronTask{}
	if err := c.Do(ctx, http.MethodGet, "/v1/cron-tasks/"+id, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// CreateCronTask calls POST /v1/cron-tasks.
func (c *Client) CreateCronTask(ctx context.Context, req *cronv1.CreateCronTaskRequest) (*cronv1.CronTask, error) {
	resp := &cronv1.CronTask{}
	if err := c.Do(ctx, http.MethodPost, "/v1/cron-tasks", req, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// UpdateCronTask calls PATCH /v1/cron-tasks/{id} with body: "task".
func (c *Client) UpdateCronTask(ctx context.Context, id string, task *cronv1.CronTask) (*cronv1.CronTask, error) {
	resp := &cronv1.CronTask{}
	b, err := marshalOpts.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("marshal cron task: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPatch, c.Base+"/v1/cron-tasks/"+id, bytes.NewReader(b))
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

// DeleteCronTask calls DELETE /v1/cron-tasks/{id}.
func (c *Client) DeleteCronTask(ctx context.Context, id string) error {
	return c.Do(ctx, http.MethodDelete, "/v1/cron-tasks/"+id, nil, nil)
}

// TriggerCronTask calls POST /v1/cron-tasks/{id}/trigger.
func (c *Client) TriggerCronTask(ctx context.Context, id string) (*cronv1.CronTaskRun, error) {
	resp := &cronv1.CronTaskRun{}
	if err := c.Do(ctx, http.MethodPost, "/v1/cron-tasks/"+id+"/trigger", &emptypb.Empty{}, resp); err != nil {
		return nil, err
	}
	return resp, nil
}

// ListCronTaskRuns calls GET /v1/cron-task-runs.
func (c *Client) ListCronTaskRuns(ctx context.Context, taskID string, limit int32) (*cronv1.ListCronTaskRunsResponse, error) {
	path := "/v1/cron-task-runs"
	sep := "?"
	if taskID != "" {
		path += sep + "cron_task_id=" + taskID
		sep = "&"
	}
	if limit > 0 {
		path += fmt.Sprintf("%slimit=%d", sep, limit)
	}
	resp := &cronv1.ListCronTaskRunsResponse{}
	if err := c.Do(ctx, http.MethodGet, path, nil, resp); err != nil {
		return nil, err
	}
	return resp, nil
}
