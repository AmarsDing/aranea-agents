package service

import (
	"context"

	v1 "aranea-agents/api/kratos/cron/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"

	emptypb "google.golang.org/protobuf/types/known/emptypb"
)

// CronService implements kratos cron.v1.
type CronService struct {
	v1.UnimplementedCronServiceServer

	uc *biz.CronUsecase
}

func NewCronService(uc *biz.CronUsecase) *CronService {
	return &CronService{uc: uc}
}

// assertCronTaskAccess 校验 caller workspace 是否拥有指定 cron task。
// P2-B: IDOR 防护 — 防止跨租户访问 cron task（私有实体）。
//
// 校验逻辑：
//  1. 系统 caller（workspace.IsSystem）→ 绕过（cron/admin 后台任务）
//  2. 查 task → task.WorkspaceID；task 不存在或查询失败 → NotFound（不泄露存在性）
//  3. workspace.AssertWorkspace 校验；不匹配 → NotFound（不泄露存在性）
//
// TECH-DEBT(P2-B): add Warn log once CronService gets lg field injected via wire.
func (s *CronService) assertCronTaskAccess(ctx context.Context, taskID string) error {
	if workspace.IsSystem(ctx) {
		return nil
	}
	task, err := s.uc.GetTask(ctx, taskID)
	if err != nil {
		// task 不存在或查询失败 → NotFound，不泄露存在性
		// TECH-DEBT(P2-B): add Warn log once CronService gets lg field injected via wire.
		return apierror.NotFound("CRON", "cron task not found")
	}
	if err := workspace.AssertWorkspace(workspace.IDFromContext(ctx), task.WorkspaceID); err != nil {
		// workspace 不匹配 → NotFound，不泄露存在性
		// TECH-DEBT(P2-B): add Warn log once CronService gets lg field injected via wire.
		return apierror.NotFound("CRON", "cron task not found")
	}
	return nil
}

func toProtoCronTask(t biz.CronTask) *v1.CronTask {
	return &v1.CronTask{
		Id:           t.ID,
		TaskKey:      t.TaskKey,
		Name:         t.Name,
		Description:  t.Description,
		Status:       t.Status,
		Enabled:      t.Enabled,
		SortOrder:    int32(t.SortOrder),
		AgentId:      t.AgentID,
		ConfigJson:   t.ConfigJSON,
		MetadataJson: t.MetadataJSON,
		CreatedAt:    t.CreatedAt,
		UpdatedAt:    t.UpdatedAt,
		DeletedAt:    t.DeletedAt,
	}
}

func patchFromProtoCronTask(pb *v1.CronTask) biz.CronTaskPatch {
	if pb == nil {
		return biz.CronTaskPatch{}
	}
	return biz.CronTaskPatch{
		TaskKey:      biz.StrPtr(pb.GetTaskKey()),
		Name:         biz.StrPtr(pb.GetName()),
		Description:  biz.StrPtr(pb.GetDescription()),
		Status:       biz.StrPtr(pb.GetStatus()),
		Enabled:      biz.BoolPtr(pb.GetEnabled()),
		SortOrder:    biz.IntPtr(int(pb.GetSortOrder())),
		AgentID:      biz.StrPtr(pb.GetAgentId()),
		ConfigJSON:   biz.StrPtr(pb.GetConfigJson()),
		MetadataJSON: biz.StrPtr(pb.GetMetadataJson()),
	}
}

func toProtoCronTaskRun(r biz.CronTaskRun) *v1.CronTaskRun {
	return &v1.CronTaskRun{
		Id:           r.ID,
		TaskId:       r.TaskID,
		TaskName:     r.TaskName,
		Status:       r.Status,
		StartedAt:    r.StartedAt,
		FinishedAt:   r.FinishedAt,
		Trigger:      r.Trigger,
		RunId:        r.RunID,
		OutputJson:   r.OutputJSON,
		ErrorMessage: r.ErrorMessage,
		CreatedAt:    r.CreatedAt,
	}
}

func (s *CronService) ListCronTasks(ctx context.Context, _ *emptypb.Empty) (*v1.ListCronTasksResponse, error) {
	items, err := s.uc.ListTasks(ctx)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListCronTasksResponse{Items: make([]*v1.CronTask, 0, len(items))}
	for i := range items {
		resp.Items = append(resp.Items, toProtoCronTask(items[i]))
	}
	return resp, nil
}

func (s *CronService) CreateCronTask(ctx context.Context, req *v1.CreateCronTaskRequest) (*v1.CronTask, error) {
	in := biz.CronTask{
		TaskKey:      req.GetTaskKey(),
		Name:         req.GetName(),
		Description:  req.GetDescription(),
		Status:       req.GetStatus(),
		Enabled:      req.GetEnabled(),
		SortOrder:    int(req.GetSortOrder()),
		AgentID:      req.GetAgentId(),
		ConfigJSON:   req.GetConfigJson(),
		MetadataJSON: req.GetMetadataJson(),
	}
	// P2-B: workspace isolation. System caller leaves WorkspaceID empty (system-owned).
	if !workspace.IsSystem(ctx) {
		in.WorkspaceID = workspace.IDFromContext(ctx)
	}
	out, err := s.uc.CreateTask(ctx, in)
	if err != nil {
		return nil, err
	}
	return toProtoCronTask(out), nil
}

func (s *CronService) GetCronTask(ctx context.Context, req *v1.GetCronTaskRequest) (*v1.CronTask, error) {
	t, err := s.uc.GetTask(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoCronTask(t), nil
}

func (s *CronService) UpdateCronTask(ctx context.Context, req *v1.UpdateCronTaskRequest) (*v1.CronTask, error) {
	if req.GetTask() == nil {
		return nil, apierror.BadRequest("CRON", "task body is required")
	}
	if err := s.assertCronTaskAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	out, err := s.uc.UpdateTask(ctx, req.GetId(), patchFromProtoCronTask(req.GetTask()))
	if err != nil {
		return nil, err
	}
	return toProtoCronTask(out), nil
}

func (s *CronService) DeleteCronTask(ctx context.Context, req *v1.DeleteCronTaskRequest) (*emptypb.Empty, error) {
	if err := s.assertCronTaskAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	if err := s.uc.DeleteTask(ctx, req.GetId()); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (s *CronService) ListCronTaskRuns(ctx context.Context, req *v1.ListCronTaskRunsRequest) (*v1.ListCronTaskRunsResponse, error) {
	q := biz.CronTaskRunQuery{
		TaskID: req.GetCronTaskId(),
		Status: req.GetStatus(),
	}
	if ps := int(req.GetPageSize()); ps > 0 {
		page := int(req.GetPage())
		if page <= 0 {
			page = 1
		}
		q.Limit = ps
		q.Offset = (page - 1) * ps
	} else {
		q.Limit = int(req.GetLimit())
	}
	items, total, err := s.uc.ListTaskRuns(ctx, q)
	if err != nil {
		return nil, err
	}
	resp := &v1.ListCronTaskRunsResponse{Items: make([]*v1.CronTaskRun, 0, len(items)), Total: int64(total)}
	for i := range items {
		resp.Items = append(resp.Items, toProtoCronTaskRun(items[i]))
	}
	return resp, nil
}

func (s *CronService) TriggerCronTask(ctx context.Context, req *v1.TriggerCronTaskRequest) (*v1.CronTaskRun, error) {
	if err := s.assertCronTaskAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	run, err := s.uc.TriggerTask(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoCronTaskRun(run), nil
}

// GetTaskRun is internal-only (not a proto RPC); used by cronTriggerGatewayAdapter for async completion watchers.
func (s *CronService) GetTaskRun(ctx context.Context, id string) (biz.CronTaskRun, error) {
	if s == nil || s.uc == nil {
		return biz.CronTaskRun{}, apierror.Internal("CRON", "cron service not configured")
	}
	return s.uc.GetTaskRun(ctx, id)
}

func (s *CronService) ResetCronTaskFailures(ctx context.Context, req *v1.ResetCronTaskFailuresRequest) (*v1.CronTask, error) {
	if err := s.assertCronTaskAccess(ctx, req.GetId()); err != nil {
		return nil, err
	}
	out, err := s.uc.ResetTaskFailures(ctx, req.GetId())
	if err != nil {
		return nil, err
	}
	return toProtoCronTask(out), nil
}
