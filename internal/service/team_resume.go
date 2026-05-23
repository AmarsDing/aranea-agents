package service

import (
	"context"

	v1 "aranea-agents/api/kratos/team/v1"

	kerrors "github.com/go-kratos/kratos/v2/errors"
)

func (s *TeamService) ResumeTeamRunExecution(ctx context.Context, req *v1.ResumeTeamRunExecutionRequest) (*v1.ResumeTeamRunExecutionResponse, error) {
	run, err := s.uc.GetRun(ctx, req.GetRunId())
	if err != nil {
		return nil, mapTeamErr(err)
	}
	execID := run.GraphExecutionID
	if execID == "" {
		return nil, kerrors.BadRequest("TEAM", "team run has no graph_execution_id; resume requires Graph runtime")
	}
	if s.graphUC == nil {
		return nil, kerrors.InternalServer("TEAM", "graph runtime unavailable")
	}
	var resume map[string]any
	if req.GetResumeValue() != nil {
		resume = req.GetResumeValue().AsMap()
	}
	exec, err := s.graphUC.ResumeExecution(ctx, execID, resume)
	if err != nil {
		return nil, mapTeamErr(err)
	}
	return &v1.ResumeTeamRunExecutionResponse{
		RunId:             run.ID,
		GraphExecutionId: execID,
		Status:            exec.Status,
	}, nil
}
