package service

import (
	v1 "aranea-agents/api/kratos/evaluation/v1"
	"aranea-agents/internal/biz"
	"aranea-agents/internal/workspace"
	"aranea-agents/pkg/apierror"
	"context"
	"fmt"
	"strings"
	"time"
)

func (s *EvaluationService) ListDatasetVersions(ctx context.Context, req *v1.ListDatasetVersionsRequest) (*v1.ListDatasetVersionsResponse, error) {
	if _, err := s.assertEvalDatasetAccess(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	items, err := s.uc.ListDatasetVersions(ctx, req.GetDatasetId(), int(req.GetLimit()))
	if err != nil {
		return nil, err
	}
	out := make([]*v1.EvalDatasetVersion, 0, len(items))
	for _, v := range items {
		out = append(out, &v1.EvalDatasetVersion{
			Id:        v.ID,
			DatasetId: v.DatasetID,
			Version:   int32(v.Version),
			Hash:      v.Hash,
			CaseCount: int32(v.CaseCount),
			CreatedAt: v.CreatedAt,
		})
	}
	return &v1.ListDatasetVersionsResponse{Items: out}, nil
}

func (s *EvaluationService) RunExperiment(ctx context.Context, req *v1.RunExperimentRequest) (*v1.RunExperimentResponse, error) {
	if _, err := s.assertEvalDatasetMutate(ctx, req.GetDatasetId()); err != nil {
		return nil, err
	}
	if s.runner == nil {
		return nil, apierror.Unavailable("EVAL", "evaluation runner is not available")
	}
	if len(req.GetVariants()) < 2 {
		return nil, apierror.BadRequest("EVAL", "at least two variants are required")
	}
	cases, err := s.uc.ListCases(ctx, req.GetDatasetId())
	if err != nil {
		return nil, err
	}
	if len(cases) == 0 {
		return nil, apierror.BadRequest("EVAL", "dataset has no cases")
	}
	numRuns := int(req.GetNumRuns())
	if numRuns <= 0 {
		numRuns = 1
	}
	expID := "exp-" + time.Now().UTC().Format("20060102150405")
	items := make([]*v1.EvalRun, 0, len(req.GetVariants()))
	for _, v := range req.GetVariants() {
		agentID := strings.TrimSpace(v.GetAgentId())
		if agentID == "" {
			continue
		}
		model := strings.TrimSpace(v.GetModel())
		prompt := strings.TrimSpace(v.GetPrompt())
		tools := strings.TrimSpace(v.GetTools())
		label := strings.TrimSpace(v.GetLabel())
		if label == "" {
			label = biz.DefaultEvalVariantLabel(agentID, model, prompt, tools)
		}
		if existing, ok, err := s.uc.FindInFlightExperimentCell(ctx, req.GetDatasetId(), agentID, label); err != nil {
			return nil, err
		} else if ok {
			return nil, apierror.Conflict("EVAL",
				fmt.Sprintf("an evaluation run is already in flight for this experiment cell (run_id=%s)", existing.ID))
		}
		in := biz.EvalRun{
			DatasetID:        req.GetDatasetId(),
			AgentID:          agentID,
			NumRuns:          numRuns,
			ExperimentID:     expID,
			VariantLabel:     label,
			Model:            model,
			Prompt:           prompt,
			Tools:            tools,
			DatasetVersionID: strings.TrimSpace(req.GetDatasetVersionId()),
		}
		if !workspace.IsSystem(ctx) {
			in.WorkspaceID = workspace.IDFromContext(ctx)
		}
		run, err := s.uc.CreateRun(ctx, in)
		if err != nil {
			return nil, err
		}
		if err := s.runner.Start(ctx, run, req.GetMetrics(), numRuns, req.GetUseUserSimulation()); err != nil {
			_ = s.uc.DeleteRun(ctx, run.ID)
			return nil, err
		}
		items = append(items, toProtoRun(run))
	}
	if len(items) == 0 {
		return nil, apierror.BadRequest("EVAL", "no valid experiment variants")
	}
	return &v1.RunExperimentResponse{ExperimentId: expID, Items: items}, nil
}

func toProtoRunPreference(p biz.EvalRunPreference) *v1.EvalRunPreference {
	return &v1.EvalRunPreference{
		Id:          p.ID,
		DatasetId:   p.DatasetID,
		RunIdA:      p.RunIDA,
		RunIdB:      p.RunIDB,
		WinnerRunId: p.WinnerRunID,
		Comment:     p.Comment,
		CreatedBy:   p.CreatedBy,
		CreatedAt:   p.CreatedAt,
	}
}

func toProtoGateConfig(c biz.EvalGateConfig) *v1.EvalGateConfig {
	return &v1.EvalGateConfig{
		Enabled:   c.Enabled,
		AgentId:   c.AgentID,
		DatasetId: c.DatasetID,
		Metric:    c.Metric,
		MinScore:  c.MinScore,
		MaxDrop:   c.MaxDrop,
		Mode:      c.Mode,
		UpdatedAt: c.UpdatedAt,
	}
}

func toProtoCase(c biz.EvalCase) *v1.EvalCase {
	return &v1.EvalCase{
		Id:             c.ID,
		DatasetId:      c.DatasetID,
		Input:          c.Input,
		ExpectedOutput: c.ExpectedOutput,
		MetadataJson:   c.MetadataJSON,
	}
}

func toProtoDataset(d biz.EvalDataset) *v1.EvalDataset {
	return &v1.EvalDataset{
		Id:          d.ID,
		Name:        d.Name,
		Description: d.Description,
		CaseCount:   int32(d.CaseCount),
		Workspace:   d.Workspace,
		CreatedAt:   d.CreatedAt,
		UpdatedAt:   d.UpdatedAt,
	}
}

func toProtoRun(r biz.EvalRun) *v1.EvalRun {
	return &v1.EvalRun{
		Id:                 r.ID,
		DatasetId:          r.DatasetID,
		AgentId:            r.AgentID,
		Status:             r.Status,
		TotalCases:         int32(r.TotalCases),
		CompletedCases:     int32(r.CompletedCases),
		ExactMatchScore:    r.ExactMatchScore,
		ContainsMatchScore: r.ContainsMatchScore,
		LlmJudgeScore:      r.LLMJudgeScore,
		ToolCallAccuracy:   r.ToolCallAccuracy,
		PassAtK:            r.PassAtK,
		PassHatK:           r.PassHatK,
		TriggerSource:      r.TriggerSource,
		NumRuns:            int32(r.NumRuns),
		ScoresJson:         r.ScoresJSON,
		ErrorMessage:       r.ErrorMessage,
		StartedAt:          r.StartedAt,
		FinishedAt:         r.FinishedAt,
		DatasetHash:        r.DatasetHash,
		DatasetVersionId:   r.DatasetVersionID,
		DatasetVersion:     int32(r.DatasetVersion),
		ExperimentId:       r.ExperimentID,
		VariantLabel:       r.VariantLabel,
		Model:              r.Model,
		Prompt:             r.Prompt,
		Tools:              r.Tools,
		JudgeCalls:         int32(r.JudgeCalls),
		JudgeTokens:        int32(r.JudgeTokens),
		CreatedAt:          r.CreatedAt,
	}
}

func toProtoCaseResult(r biz.EvalCaseResult) *v1.EvalCaseResult {
	out := &v1.EvalCaseResult{
		Id:               r.ID,
		RunId:            r.RunID,
		CaseId:           r.CaseID,
		Input:            r.Input,
		ActualOutput:     r.ActualOutput,
		ExactMatch:       r.ExactMatch,
		ContainsMatch:    r.ContainsMatch,
		LlmJudgeScore:    r.LLMJudgeScore,
		ToolCallAccuracy: r.ToolCallAccuracy,
		ErrorMessage:     r.ErrorMessage,
		CreatedAt:        r.CreatedAt,
		HumanComment:     r.HumanComment,
		AnnotatedAt:      r.AnnotatedAt,
		AnnotatedBy:      r.AnnotatedBy,
		ScoresJson:       r.ScoresJSON,
		SessionId:        r.SessionID,
		TraceRunId:       r.TraceRunID,
	}
	if r.HumanPass != nil {
		v := *r.HumanPass
		out.HumanPass = &v
	}
	if r.HumanScore != nil {
		v := *r.HumanScore
		out.HumanScore = &v
	}
	return out
}
