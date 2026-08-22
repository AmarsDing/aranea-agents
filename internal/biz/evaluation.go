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
	EvalDatasetVersion       = evaluation.DatasetVersion
	EvalRunOverride          = evaluation.RunOverride
	// EvalRepo is the deprecated composed evaluation persistence port.
	// Production uses EvalStores (see evaluation.Stores).
	EvalRepo       = evaluation.Repo
	EvalStores     = evaluation.Stores
	EvalUsecase    = evaluation.Usecase
	EvalScores     = evaluation.Scores
	EvalLLMSetting = evaluation.LLMSetting
)

// Re-export evaluation gate trigger constants.
const (
	EvalGateTriggerSkillPublish = evaluation.GateTriggerSkillPublish
	EvalGateTriggerPackInstall  = evaluation.GateTriggerPackInstall
	EvalMaxNumRuns              = evaluation.MaxNumRuns
)

// Re-export evaluation constructors and helpers for backward compatibility.
var (
	NewEvalUsecase          = evaluation.NewUsecase
	EvalStoresFrom          = evaluation.StoresFrom
	ParseEvalScores         = evaluation.ParseScores
	MarshalEvalScores       = evaluation.MarshalScores
	ApplyEvalLLMPatch       = evaluation.ApplyLLMPatch
	EvalRunMetricScore      = evaluation.RunMetricScore
	WithEvalRunOverride     = evaluation.WithRunOverride
	EvalRunOverrideFrom     = evaluation.RunOverrideFrom
	OverlayEvalPrompt       = evaluation.OverlayPrompt
	DefaultEvalVariantLabel = evaluation.DefaultVariantLabel
)
