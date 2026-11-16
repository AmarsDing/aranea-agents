package service

import (
	"context"

	v1 "aranea-agents/api/kratos/office/v1"
	"aranea-agents/internal/biz/office"

	"google.golang.org/protobuf/types/known/timestamppb"
)

// 个人任务 RPC（工作台「我的任务」）。只读写调用者本人的任务，属主来自
// 登录上下文，请求体不接受 user_id。

func officeTaskToProto(t *office.OfficeTask) *v1.OfficeTask {
	return &v1.OfficeTask{
		Id:        t.ID,
		Title:     t.Title,
		Body:      t.Body,
		Source:    t.Source,
		SourceRef: t.SourceRef,
		Status:    t.Status,
		CreatedAt: timestamppb.New(t.CreatedAt),
		UpdatedAt: timestamppb.New(t.UpdatedAt),
	}
}

func (s *OfficeMailService) ListOfficeTasks(ctx context.Context, req *v1.ListOfficeTasksRequest) (*v1.ListOfficeTasksResponse, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	rows, err := s.messages.ListTasks(ctx, userID, req.GetStatus(), int(req.GetLimit()))
	if err != nil {
		return nil, mailErr(err)
	}
	out := &v1.ListOfficeTasksResponse{Items: make([]*v1.OfficeTask, 0, len(rows))}
	for _, t := range rows {
		out.Items = append(out.Items, officeTaskToProto(t))
	}
	return out, nil
}

func (s *OfficeMailService) CreateOfficeTask(ctx context.Context, req *v1.CreateOfficeTaskRequest) (*v1.OfficeTask, error) {
	userID, workspaceID, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	task, err := s.messages.CreateTask(ctx, userID, workspaceID, req.GetTitle(), req.GetBody())
	if err != nil {
		return nil, mailErr(err)
	}
	return officeTaskToProto(task), nil
}

func (s *OfficeMailService) SetOfficeTaskStatus(ctx context.Context, req *v1.SetOfficeTaskStatusRequest) (*v1.OfficeTask, error) {
	userID, _, err := officeSubject(ctx)
	if err != nil {
		return nil, mailErr(err)
	}
	task, err := s.messages.SetTaskStatus(ctx, req.GetId(), userID, req.GetStatus())
	if err != nil {
		return nil, mailErr(err)
	}
	return officeTaskToProto(task), nil
}
