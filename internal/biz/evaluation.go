package biz

import "aranea-agents/internal/biz/evaluation"

// Re-export evaluation types from sub-package for backward compatibility.
type (
	EvalDataset              = evaluation.Dataset
	EvalCase                 = evaluation.Case
	EvalCaseUpload           = evaluation.CaseUpload
	EvalRun                  = evaluation.Run
	EvalCaseResult           = evaluation.CaseResult
	EvalCaseResultAnnotation = evaluation.CaseResultAnnotation
	EvalTrendPoint           = evaluation.TrendPoint
	EvalRunComparison        = evaluation.RunComparison
	EvalJudgeDivergence      = evaluation.JudgeDivergence
	EvalJudgeDivergenceCase  = evaluation.JudgeDivergenceCase
	EvalJudgeAnnotatedResult = evaluation.JudgeAnnotatedResult
	EvalFailureGroup         = evaluation.FailureGroup
	EvalFailureGroupReport   = evaluation.FailureGroupReport
	EvalRunPreference        = evaluation.RunPreference
	EvalGateConfig           = evaluation.GateConfig
	EvalRepo                 = evaluation.Repo
	EvalUsecase              = evaluation.Usecase
	EvalScores               = evaluation.Scores
	EvalLLMSetting           = evaluation.LLMSetting
)

// Re-export evaluation gate trigger constants.
const (
	EvalGateTriggerSkillPublish = evaluation.GateTriggerSkillPublish
	EvalGateTriggerPackInstall  = evaluation.GateTriggerPackInstall
)

// Re-export evaluation constructors and helpers for backward compatibility.
var (
	NewEvalUsecase     = evaluation.NewUsecase
	ParseEvalScores    = evaluation.ParseScores
	MarshalEvalScores  = evaluation.MarshalScores
	ApplyEvalLLMPatch  = evaluation.ApplyLLMPatch
	EvalRunMetricScore = evaluation.RunMetricScore
)
