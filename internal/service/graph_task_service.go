package service

import (
	"context"

	graphv1 "aranea-agents/api/kratos/graph/v1"
	"aranea-agents/internal/biz"

	"google.golang.org/protobuf/types/known/timestamppb"
)

func (s *GraphService) ListTasks(ctx context.Context, req *graphv1.ListTasksRequest) (*graphv1.ListTasksResponse, error) {
	tasks, nextToken, err := s.taskUC.ListTasks(ctx, req.ExecutionId, protoTaskStatusToBiz(req.StatusFilter), int(req.PageSize), req.PageToken)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.Task, len(tasks))
	for i, t := range tasks {
		items[i] = toProtoTask(t)
	}
	return &graphv1.ListTasksResponse{Items: items, NextPageToken: nextToken}, nil
}

func (s *GraphService) GetTask(ctx context.Context, req *graphv1.GetTaskRequest) (*graphv1.GetTaskResponse, error) {
	task, err := s.taskUC.GetTask(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	return &graphv1.GetTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) ClaimTask(ctx context.Context, req *graphv1.ClaimTaskRequest) (*graphv1.ClaimTaskResponse, error) {
	task, err := s.taskUC.ClaimTask(ctx, req.TaskId, req.AgentKey)
	if err != nil {
		return nil, err
	}
	return &graphv1.ClaimTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) SubmitTaskResult(ctx context.Context, req *graphv1.SubmitTaskResultRequest) (*graphv1.SubmitTaskResultResponse, error) {
	task, err := s.taskUC.SubmitTaskResult(ctx, req.TaskId, req.Output, req.Summary, req.Metadata)
	if err != nil {
		return nil, err
	}
	return &graphv1.SubmitTaskResultResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) Heartbeat(ctx context.Context, req *graphv1.HeartbeatRequest) (*graphv1.HeartbeatResponse, error) {
	ack, ext, err := s.taskUC.Heartbeat(ctx, req.TaskId, req.AgentKey, req.Metadata)
	if err != nil {
		return nil, err
	}
	return &graphv1.HeartbeatResponse{Acknowledged: ack, LeaseExtensionSeconds: ext}, nil
}

func (s *GraphService) ReportBlocked(ctx context.Context, req *graphv1.ReportBlockedRequest) (*graphv1.ReportBlockedResponse, error) {
	task, err := s.taskUC.ReportBlocked(ctx, req.TaskId, req.Reason, req.Metadata)
	if err != nil {
		return nil, err
	}
	return &graphv1.ReportBlockedResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) UnblockTask(ctx context.Context, req *graphv1.UnblockTaskRequest) (*graphv1.UnblockTaskResponse, error) {
	task, err := s.taskUC.UnblockTask(ctx, req.TaskId, req.Comment)
	if err != nil {
		return nil, err
	}
	return &graphv1.UnblockTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) CreateTask(ctx context.Context, req *graphv1.CreateTaskRequest) (*graphv1.CreateTaskResponse, error) {
	task, err := s.taskUC.CreateTaskWithParents(ctx, biz.CreateTaskParams{
		ExecutionID:        req.ExecutionId,
		NodeID:             req.NodeId,
		RequiredRole:       req.RequiredRole,
		AssignmentMode:     req.AssignmentMode,
		AssignmentStrategy: req.AssignmentStrategy,
		Input:              req.Input,
		Context:            req.Context,
		ParentIDs:          req.ParentTaskIds,
	})
	if err != nil {
		return nil, err
	}
	return &graphv1.CreateTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) LinkTasks(ctx context.Context, req *graphv1.LinkTasksRequest) (*graphv1.LinkTasksResponse, error) {
	if err := s.taskUC.LinkTasks(ctx, req.ParentTaskId, req.ChildTaskId); err != nil {
		return nil, err
	}
	return &graphv1.LinkTasksResponse{}, nil
}

func (s *GraphService) UnlinkTasks(ctx context.Context, req *graphv1.UnlinkTasksRequest) (*graphv1.UnlinkTasksResponse, error) {
	if err := s.taskUC.UnlinkTasks(ctx, req.ParentTaskId, req.ChildTaskId); err != nil {
		return nil, err
	}
	return &graphv1.UnlinkTasksResponse{}, nil
}

func (s *GraphService) ReviewTask(ctx context.Context, req *graphv1.ReviewTaskRequest) (*graphv1.ReviewTaskResponse, error) {
	task, err := s.taskUC.ReviewTask(ctx, req.TaskId, req.ReviewerAgent, req.Approved, req.Comment)
	if err != nil {
		return nil, err
	}
	return &graphv1.ReviewTaskResponse{Task: toProtoTask(task)}, nil
}

func (s *GraphService) ListTaskComments(ctx context.Context, req *graphv1.ListTaskCommentsRequest) (*graphv1.ListTaskCommentsResponse, error) {
	comments, err := s.taskUC.ListTaskComments(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskComment, len(comments))
	for i, c := range comments {
		items[i] = &graphv1.TaskComment{
			CommentId: c.CommentID,
			TaskId:    c.TaskID,
			Author:    c.Author,
			Content:   c.Content,
			Type:      c.Type,
			CreatedAt: timestamppb.New(c.CreatedAt),
		}
	}
	return &graphv1.ListTaskCommentsResponse{Comments: items}, nil
}

func (s *GraphService) AddTaskComment(ctx context.Context, req *graphv1.AddTaskCommentRequest) (*graphv1.AddTaskCommentResponse, error) {
	comment, err := s.taskUC.AddTaskComment(ctx, req.TaskId, req.Author, req.Content, req.Type)
	if err != nil {
		return nil, err
	}
	return &graphv1.AddTaskCommentResponse{Comment: &graphv1.TaskComment{
		CommentId: comment.CommentID,
		TaskId:    comment.TaskID,
		Author:    comment.Author,
		Content:   comment.Content,
		Type:      comment.Type,
		CreatedAt: timestamppb.New(comment.CreatedAt),
	}}, nil
}

func (s *GraphService) ListTaskLogs(ctx context.Context, req *graphv1.ListTaskLogsRequest) (*graphv1.ListTaskLogsResponse, error) {
	logs, err := s.taskUC.ListTaskLogs(ctx, req.TaskId, req.Stream, req.Level, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskLog, len(logs))
	for i, l := range logs {
		items[i] = &graphv1.TaskLog{
			LogId:     l.LogID,
			TaskId:    l.TaskID,
			Stream:    l.Stream,
			Content:   l.Content,
			Level:     l.Level,
			Timestamp: timestamppb.New(l.Timestamp),
		}
	}
	return &graphv1.ListTaskLogsResponse{Logs: items}, nil
}

func (s *GraphService) ListTaskRuns(ctx context.Context, req *graphv1.ListTaskRunsRequest) (*graphv1.ListTaskRunsResponse, error) {
	runs, err := s.taskUC.ListTaskRuns(ctx, req.TaskId)
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskRun, len(runs))
	for i, r := range runs {
		items[i] = &graphv1.TaskRun{
			RunId:     r.RunID,
			TaskId:    r.TaskID,
			StartedAt: timestamppb.New(r.StartedAt),
			ExitCode:  int32(r.ExitCode),
			LogRef:    r.LogRef,
		}
		if r.FinishedAt != nil {
			items[i].FinishedAt = timestamppb.New(*r.FinishedAt)
		}
	}
	return &graphv1.ListTaskRunsResponse{Runs: items}, nil
}

func (s *GraphService) ListTaskEvents(ctx context.Context, req *graphv1.ListTaskEventsRequest) (*graphv1.ListTaskEventsResponse, error) {
	events, err := s.taskUC.ListTaskEvents(ctx, req.ExecutionId, req.TaskId, req.EventType, int(req.PageSize))
	if err != nil {
		return nil, err
	}
	items := make([]*graphv1.TaskEvent, len(events))
	for i, e := range events {
		items[i] = &graphv1.TaskEvent{
			EventId:     e.EventID,
			TaskId:      e.TaskID,
			EventType:   e.EventType,
			SourceNode:  e.SourceNode,
			Description: e.Description,
			Timestamp:   timestamppb.New(e.Timestamp),
		}
	}
	return &graphv1.ListTaskEventsResponse{Events: items}, nil
}
