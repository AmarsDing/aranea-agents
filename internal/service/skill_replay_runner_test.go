package service

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/biz/evaluation"
	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// ── fakes ────────────────────────────────────────────────────────────────────

type fakeEvalDatasetReader struct {
	datasets []evaluation.Dataset
	cases    []evaluation.Case
	err      error
}

func (f *fakeEvalDatasetReader) ListDatasets(context.Context, string, int, int) ([]evaluation.Dataset, int, error) {
	if f.err != nil {
		return nil, 0, f.err
	}
	return f.datasets, len(f.datasets), nil
}

func (f *fakeEvalDatasetReader) ListCases(context.Context, string) ([]evaluation.Case, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.cases, nil
}

type fakeReplayLLMCaller struct {
	outputs []string // per-case outputs, in order
	err     error
	calls   int
}

func (f *fakeReplayLLMCaller) Call(_ context.Context, _ biz.LLMCallRequest) (string, int, error) {
	if f.err != nil {
		return "", 0, f.err
	}
	out := ""
	if f.calls < len(f.outputs) {
		out = f.outputs[f.calls]
	}
	f.calls++
	return out, len(out) / 4, nil
}

type fakeRefineLLMReader struct {
	setting biz.RefineLLMSetting
	err     error
}

func (f *fakeRefineLLMReader) GetRefineLLM(context.Context) (biz.RefineLLMSetting, error) {
	return f.setting, f.err
}

type fakeReplaySkillLookup struct {
	skill biz.Skill
	err   error
}

func (f *fakeReplaySkillLookup) GetSkillByID(context.Context, string) (biz.Skill, error) {
	return f.skill, f.err
}
func (f *fakeReplaySkillLookup) GetSkillBySkillKey(context.Context, string) (biz.Skill, error) {
	return biz.Skill{}, errors.New("not implemented")
}
func (f *fakeReplaySkillLookup) GetSkillStorageDir(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}
func (f *fakeReplaySkillLookup) GetLatestSkillMarkdown(context.Context, string) (string, error) {
	return "", errors.New("not implemented")
}

func newTestReplayRunner(eval evalDatasetReader, caller biz.LLMCaller, llm RefineLLMReader, skills biz.SkillLookupReader) *SkillReplayRunner {
	return NewSkillReplayRunner(eval, caller, llm, skills, loggateway.NewNoop())
}

// ── tests ────────────────────────────────────────────────────────────────────

// 数据集寻址约定：名称匹配 skill 的 Name 或 Slug；未命中 → ErrNoReplayDataset。
func TestSkillReplayRunner_NoBoundDataset(t *testing.T) {
	eval := &fakeEvalDatasetReader{datasets: []evaluation.Dataset{{ID: "ds1", Name: "other-dataset"}}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research", Slug: "web-research"}}
	r := newTestReplayRunner(eval, &fakeReplayLLMCaller{}, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	_, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if !errors.Is(err, biz.ErrNoReplayDataset) && err != biz.ErrNoReplayDataset {
		t.Fatalf("expected ErrNoReplayDataset, got %v", err)
	}
}

// 名称（或 Slug）大小写不敏感命中；含期望输出（大小写不敏感）计通过。
func TestSkillReplayRunner_PassRateScoring(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "Web-Research"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "q1", ExpectedOutput: "Paris"},
			{ID: "c2", Input: "q2", ExpectedOutput: "Tokyo"},
		},
	}
	caller := &fakeReplayLLMCaller{outputs: []string{
		"The capital is paris, France.", // case-insensitive contains → pass
		"The capital is London.",        // miss → fail
	}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research", Slug: "web-research"}}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	res, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if res.Total != 2 || res.Passed != 1 {
		t.Fatalf("expected 1/2 passed, got %d/%d", res.Passed, res.Total)
	}
	if res.PassRate != 0.5 {
		t.Fatalf("expected pass rate 0.5, got %v", res.PassRate)
	}
	if res.DatasetID != "ds1" || res.DatasetName != "Web-Research" {
		t.Fatalf("unexpected dataset identity: %+v", res)
	}
	if caller.calls != 2 {
		t.Fatalf("expected 2 LLM calls, got %d", caller.calls)
	}
}

// maxCases 上限截断；默认值为 biz.ReplayMaxCases。
func TestSkillReplayRunner_MaxCasesCaps(t *testing.T) {
	cases := make([]evaluation.Case, 4)
	for i := range cases {
		cases[i] = evaluation.Case{ID: "c", Input: "q", ExpectedOutput: "x"}
	}
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases:    cases,
	}
	caller := &fakeReplayLLMCaller{outputs: []string{"x", "x", "x", "x"}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	res, err := r.Replay(context.Background(), "s1", "# draft", 2)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if res.Total != 2 || caller.calls != 2 {
		t.Fatalf("expected 2 cases executed, got total=%d calls=%d", res.Total, caller.calls)
	}
}

// LLM 未配置 → 硬错误（Gate 视为回放不可用并跳过）。
func TestSkillReplayRunner_NoRefineLLM_Unavailable(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases:    []evaluation.Case{{ID: "c1", Input: "q", ExpectedOutput: "x"}},
	}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, &fakeReplayLLMCaller{}, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{}}, skills)

	_, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err == nil {
		t.Fatal("expected unavailable error when no DefaultRefineLLM configured")
	}
	if ae, ok := err.(*apierror.Error); !ok || ae.Code != apierror.CodeUnavailable {
		t.Fatalf("expected CodeUnavailable, got %v", err)
	}
}

// 单 case LLM 调用失败计为失败 case，不中断整体回放（best-effort）。
func TestSkillReplayRunner_CaseLLMFailureCountsAsFailure(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "q1", ExpectedOutput: "x"},
		},
	}
	caller := &fakeReplayLLMCaller{err: errors.New("llm timeout")}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	res, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if res.Passed != 0 || res.Total != 1 || res.PassRate != 0 {
		t.Fatalf("expected 0/1 passed, got %+v", res)
	}
}

// 空用例集 → ErrNoReplayDataset（无回放素材视为未绑定）。
func TestSkillReplayRunner_EmptyCases(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
	}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, &fakeReplayLLMCaller{}, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	_, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err != biz.ErrNoReplayDataset {
		t.Fatalf("expected ErrNoReplayDataset, got %v", err)
	}
}

// 空 expected output 的 case 恒为失败（无评分依据）。
func TestSkillReplayRunner_EmptyExpected_Fails(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases:    []evaluation.Case{{ID: "c1", Input: "q", ExpectedOutput: "  "}},
	}
	caller := &fakeReplayLLMCaller{outputs: []string{"anything"}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	res, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if res.Passed != 0 {
		t.Fatalf("expected 0 passed for empty expected output, got %+v", res)
	}
	if !strings.Contains(res.DatasetName, "web-research") {
		t.Fatalf("unexpected dataset name: %+v", res)
	}
}
