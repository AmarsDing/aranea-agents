package service

import (
	"context"
	"strings"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/evaluation"
	"aranea-agents/internal/skill/manifest"
	"aranea-agents/internal/tools/skillruntime"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// Compile-time check: SkillTriggerGoldenRunner implements the
// biz.SkillTriggerGoldenRunner port consumed by biz.GateVerifier (P2 F4 触发率
// 黄金集回归进 Gate 第七维).
var _ biz.SkillTriggerGoldenRunner = (*SkillTriggerGoldenRunner)(nil)

// triggerGoldenDatasetSuffix 是黄金集数据集寻址后缀：数据集名 =
// {skill.Name|Slug}__trigger（回放约定 + 后缀）。
const triggerGoldenDatasetSuffix = "__trigger"

// SkillTriggerGoldenRunner replays a skill's trigger golden set (should-trigger
// / should-not-trigger queries) against the frontmatter triggers of BOTH the
// current live body (baseline) and the evolved draft. Judgment is fully
// deterministic — it reuses the runtime trigger matcher
// (skillruntime.MatchTrigger: CJK substring / ASCII word boundary), no LLM.
//
// 语义约定（best-effort，与回放一致）：无绑定数据集或数据集为空返回
// biz.ErrNoReplayDataset（Gate 跳过）；全部用例非法返回 Total=0 结果
// （Gate 跳过）；当前正文不可得时 HasBaseline=false（Gate 仅查绝对下限）。
type SkillTriggerGoldenRunner struct {
	eval   evalDatasetReader
	skills biz.SkillLookupReader
	lg     loggateway.Logger
}

// NewSkillTriggerGoldenRunner constructs a SkillTriggerGoldenRunner.
func NewSkillTriggerGoldenRunner(eval evalDatasetReader, skills biz.SkillLookupReader, lg loggateway.Logger) *SkillTriggerGoldenRunner {
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &SkillTriggerGoldenRunner{eval: eval, skills: skills, lg: lg}
}

// RunTriggerGolden implements biz.SkillTriggerGoldenRunner.
func (r *SkillTriggerGoldenRunner) RunTriggerGolden(ctx context.Context, skillID string, draftBody string, maxCases int) (*biz.SkillTriggerGoldenResult, error) {
	if strings.TrimSpace(skillID) == "" {
		return nil, apierror.BadRequest("SKILL_TRIGGER_GOLDEN", "skill_id is required")
	}
	if maxCases <= 0 {
		maxCases = biz.TriggerGoldenMaxCases
	}

	dataset, err := r.findGoldenDataset(ctx, skillID)
	if err != nil {
		return nil, err
	}
	cases, err := r.eval.ListCases(ctx, dataset.ID)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_TRIGGER_GOLDEN")
	}
	if len(cases) == 0 {
		return nil, biz.ErrNoReplayDataset
	}
	if len(cases) > maxCases {
		cases = cases[:maxCases]
	}

	// 过滤非法用例（ExpectedOutput ∉ {"trigger","no_trigger"}）：跳过并计
	// Warn，只在进入双端评分前过滤一次。
	valid := make([]evaluation.Case, 0, len(cases))
	for _, c := range cases {
		expected := strings.ToLower(strings.TrimSpace(c.ExpectedOutput))
		if expected != "trigger" && expected != "no_trigger" {
			r.lg.Warn("SkillTriggerGoldenRunner: invalid case ExpectedOutput, skipped",
				loggateway.StepID("skill_trigger_golden.case"),
				loggateway.Str("case_id", c.ID),
				loggateway.Str("expected_output", c.ExpectedOutput))
			continue
		}
		valid = append(valid, c)
	}

	res := &biz.SkillTriggerGoldenResult{DatasetName: dataset.Name, Total: len(valid)}
	draftTriggers := manifest.Parse(draftBody).Triggers
	res.Accuracy, res.FalseNeg, res.FalsePos = scoreTriggerSide(draftTriggers, valid)

	// Baseline：当前正文不可得时 HasBaseline=false（Gate 仅绝对下限），不阻断。
	if currentBody, lerr := r.skills.GetLatestSkillMarkdown(ctx, skillID); lerr == nil && strings.TrimSpace(currentBody) != "" {
		baseTriggers := manifest.Parse(currentBody).Triggers
		res.BaselineAccuracy, _, _ = scoreTriggerSide(baseTriggers, valid)
		res.HasBaseline = true
	} else if lerr != nil {
		r.lg.Warn("SkillTriggerGoldenRunner: baseline body unavailable, degrades to absolute-only",
			loggateway.StepID("skill_trigger_golden.baseline"),
			loggateway.Str("skill_id", skillID),
			loggateway.Err(lerr))
	}
	return res, nil
}

// scoreTriggerSide scores one trigger set against the valid case list,
// returning accuracy plus false-negative / false-positive counts.
func scoreTriggerSide(triggers []string, cases []evaluation.Case) (accuracy float64, falseNeg, falsePos int) {
	correct := 0
	for _, c := range cases {
		triggered := skillruntime.MatchTrigger(c.Input, triggers) != ""
		wantTrigger := strings.EqualFold(strings.TrimSpace(c.ExpectedOutput), "trigger")
		switch {
		case triggered == wantTrigger:
			correct++
		case wantTrigger:
			falseNeg++
		default:
			falsePos++
		}
	}
	if len(cases) > 0 {
		accuracy = float64(correct) / float64(len(cases))
	}
	return accuracy, falseNeg, falsePos
}

// findGoldenDataset resolves the trigger golden dataset bound to the skill by
// the name convention (dataset name == {skill Name or Slug} + "__trigger").
func (r *SkillTriggerGoldenRunner) findGoldenDataset(ctx context.Context, skillID string) (*evaluation.Dataset, error) {
	skill, err := r.skills.GetSkillByID(ctx, skillID)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_TRIGGER_GOLDEN")
	}
	datasets, _, err := r.eval.ListDatasets(ctx, "", 100, 0)
	if err != nil {
		return nil, apierror.Wrap(err, apierror.CodeInternal, "SKILL_TRIGGER_GOLDEN")
	}
	wantNames := []string{
		strings.ToLower(skill.Name + triggerGoldenDatasetSuffix),
		strings.ToLower(skill.Slug + triggerGoldenDatasetSuffix),
	}
	for i := range datasets {
		name := strings.ToLower(strings.TrimSpace(datasets[i].Name))
		if name == wantNames[0] || name == wantNames[1] {
			return &datasets[i], nil
		}
	}
	return nil, biz.ErrNoReplayDataset
}
