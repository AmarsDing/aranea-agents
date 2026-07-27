package service

import (
	"context"
	"strings"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// replayCaseTimeout 单条评测用例回放的硬超时（对齐 evolver 30s 先例）。
const replayCaseTimeout = 30 * time.Second

// RefineLLMReader resolves the platform DefaultRefineLLM setting.
// *biz.SystemSettingUsecase satisfies it in production; tests use a stub.
type RefineLLMReader interface {
	GetRefineLLM(ctx context.Context) (biz.RefineLLMSetting, error)
}

// evalDatasetReader is the narrow evaluation dependency (dataset/case
// listing only). *evaluation.Usecase satisfies it in production; tests use a
// fake.
type evalDatasetReader interface {
	ListDatasets(ctx context.Context, workspace string, limit, offset int) ([]evaluation.Dataset, int, error)
	ListCases(ctx context.Context, datasetID string) ([]evaluation.Case, error)
}

// Compile-time check: SkillReplayRunner implements the biz.SkillReplayRunner
// port consumed by biz.GateVerifier (P1 Solve 接线：数据集回放进 Gate 功能维).
var _ biz.SkillReplayRunner = (*SkillReplayRunner)(nil)

// Compile-time check: the production evaluation usecase satisfies the narrow
// reader port.
var _ evalDatasetReader = (*evaluation.Usecase)(nil)

// SkillReplayRunner replays a skill's bound evaluation dataset against an
// evolved draft body: each case is executed with the draft as the system
// prompt via the platform DefaultRefineLLM, and scored by case-insensitive
// contains-match against the expected output.
//
// 数据集寻址约定：ListDatasets(workspace="") 中名称等于该 skill 的 Name 或
// Slug 者即其评测集。无绑定数据集返回 biz.ErrNoReplayDataset（Gate 跳过）；
// LLM 未配置返回错误（Gate 同样跳过，不阻断）——与项目 best-effort 降级
// 风格一致。
type SkillReplayRunner struct {
	eval   evalDatasetReader
	caller biz.LLMCaller
	llm    RefineLLMReader
	skills biz.SkillLookupReader
	lg     loggateway.Logger
}

// NewSkillReplayRunner constructs a SkillReplayRunner.
func NewSkillReplayRunner(eval evalDatasetReader, caller biz.LLMCaller, llm RefineLLMReader, skills biz.SkillLookupReader, lg loggateway.Logger) *SkillReplayRunner {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillReplayRunner{eval: eval, caller: caller, llm: llm, skills: skills, lg: lg}
}

// Replay implements biz.SkillReplayRunner.
func (r *SkillReplayRunner) Replay(ctx context.Context, skillID string, draftBody string, maxCases int) (*biz.SkillReplayResult, error) {
	if strings.TrimSpace(skillID) == "" {
		return nil, apierror.BadRequest("SKILL_REPLAY", "skill_id is required")
	}
	if maxCases <= 0 {
		maxCases = biz.ReplayMaxCases
	}

	dataset, err := r.findBoundDataset(ctx, skillID)
	if err != nil {
		return nil, err
	}
	cases, err := r.eval.ListCases(ctx, dataset.ID)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_REPLAY")
	}
	if len(cases) == 0 {
		return nil, biz.ErrNoReplayDataset
	}
	if len(cases) > maxCases {
		cases = cases[:maxCases]
	}

	provider, model, err := r.resolveReplayLLM(ctx)
	if err != nil {
		return nil, err
	}

	result := &biz.SkillReplayResult{
		DatasetID:   dataset.ID,
		DatasetName: dataset.Name,
		Total:       len(cases),
	}
	for _, c := range cases {
		if r.replayCase(ctx, provider, model, draftBody, c) {
			result.Passed++
		}
	}
	result.PassRate = float64(result.Passed) / float64(result.Total)
	return result, nil
}

// findBoundDataset resolves the evaluation dataset bound to the skill by the
// name convention (dataset name == skill Name or Slug).
func (r *SkillReplayRunner) findBoundDataset(ctx context.Context, skillID string) (*evaluation.Dataset, error) {
	skill, err := r.skills.GetSkillByID(ctx, skillID)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_REPLAY")
	}
	datasets, _, err := r.eval.ListDatasets(ctx, "", 100, 0)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_REPLAY")
	}
	for i := range datasets {
		name := strings.TrimSpace(datasets[i].Name)
		if name != "" && (strings.EqualFold(name, skill.Name) || strings.EqualFold(name, skill.Slug)) {
			return &datasets[i], nil
		}
	}
	return nil, biz.ErrNoReplayDataset
}

// resolveReplayLLM resolves the platform DefaultRefineLLM. Unconfigured LLM
// is a hard error here — the Gate treats it as "replay unavailable" and skips.
func (r *SkillReplayRunner) resolveReplayLLM(ctx context.Context) (string, string, error) {
	rl, err := r.llm.GetRefineLLM(ctx)
	if err != nil || strings.TrimSpace(rl.Provider) == "" || strings.TrimSpace(rl.Model) == "" {
		return "", "", apierror.Unavailable("SKILL_REPLAY", "no DefaultRefineLLM configured, dataset replay unavailable")
	}
	return rl.Provider, rl.Model, nil
}

// replayCase executes one case: draft body as system, case input as user.
// Scoring is case-insensitive contains-match on the expected output. A failed
// LLM call counts as a failed case (best-effort, one flaky call must not
// abort the whole replay).
func (r *SkillReplayRunner) replayCase(ctx context.Context, provider, model, draftBody string, c evaluation.Case) bool {
	callCtx, cancel := context.WithTimeout(ctx, replayCaseTimeout)
	defer cancel()
	output, _, err := r.caller.Call(callCtx, biz.LLMCallRequest{
		Provider: provider,
		Model:    model,
		System:   draftBody,
		User:     c.Input,
	})
	if err != nil {
		r.lg.Warn("SkillReplayRunner: case LLM call failed",
			loggateway.StepID("skill_replay.case"),
			loggateway.Str("case_id", c.ID),
			loggateway.Err(err))
		return false
	}
	expected := strings.TrimSpace(c.ExpectedOutput)
	if expected == "" {
		return false
	}
	return strings.Contains(strings.ToLower(output), strings.ToLower(expected))
}
