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
	skill    biz.Skill
	err      error
	markdown string
	mdErr    error
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
	if f.mdErr != nil {
		return "", f.mdErr
	}
	if f.markdown == "" {
		return "", errors.New("not implemented")
	}
	return f.markdown, nil
}

// fakeABLLMCaller returns programmed outputs keyed by the system body, so the
// baseline and draft sides of an AB replay can be scored independently.
type fakeABLLMCaller struct {
	outputsBySystem map[string][]string // per-body outputs, in order
	callsBySystem   map[string]int
}

func (f *fakeABLLMCaller) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	if f.callsBySystem == nil {
		f.callsBySystem = map[string]int{}
	}
	idx := f.callsBySystem[req.System]
	outs := f.outputsBySystem[req.System]
	out := ""
	if idx < len(outs) {
		out = outs[idx]
	}
	f.callsBySystem[req.System] = idx + 1
	return out, len(out) / 4, nil
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

// ── P2 F1：AB 对照回放 ───────────────────────────────────────────────────────

// 双端对照：baseline 与 draft 各自独立计分，共用同一数据集与同一批 case。
func TestSkillReplayRunner_ReplayAB_BothSides(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "q1", ExpectedOutput: "Paris"},
			{ID: "c2", Input: "q2", ExpectedOutput: "Tokyo"},
		},
	}
	caller := &fakeABLLMCaller{outputsBySystem: map[string][]string{
		"current body": {"Paris", "Tokyo"},  // baseline 2/2
		"draft body":   {"Paris", "London"}, // draft 1/2
	}}
	skills := &fakeReplaySkillLookup{
		skill:    biz.Skill{ID: "s1", Name: "web-research"},
		markdown: "current body",
	}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	ab, err := r.ReplayAB(context.Background(), "s1", "draft body", 0)
	if err != nil {
		t.Fatalf("ReplayAB: %v", err)
	}
	if ab.Baseline == nil {
		t.Fatal("expected baseline result")
	}
	if ab.Baseline.PassRate != 1.0 || ab.Baseline.Total != 2 {
		t.Fatalf("unexpected baseline: %+v", ab.Baseline)
	}
	if ab.Draft == nil || ab.Draft.PassRate != 0.5 || ab.Draft.Total != 2 {
		t.Fatalf("unexpected draft: %+v", ab.Draft)
	}
	// 同一 case 集：双端各执行 2 次。
	if caller.callsBySystem["current body"] != 2 || caller.callsBySystem["draft body"] != 2 {
		t.Fatalf("expected 2 calls per side, got %+v", caller.callsBySystem)
	}
	if ab.Baseline.DatasetID != ab.Draft.DatasetID {
		t.Fatalf("dataset mismatch: %+v vs %+v", ab.Baseline, ab.Draft)
	}
}

// 基线不可得（当前正文查询失败）→ Baseline=nil，Draft 正常，不报错。
func TestSkillReplayRunner_ReplayAB_BaselineUnavailable(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases:    []evaluation.Case{{ID: "c1", Input: "q1", ExpectedOutput: "Paris"}},
	}
	caller := &fakeABLLMCaller{outputsBySystem: map[string][]string{
		"draft body": {"Paris"},
	}}
	skills := &fakeReplaySkillLookup{
		skill: biz.Skill{ID: "s1", Name: "web-research"},
		mdErr: errors.New("no current version"),
	}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	ab, err := r.ReplayAB(context.Background(), "s1", "draft body", 0)
	if err != nil {
		t.Fatalf("ReplayAB: %v", err)
	}
	if ab.Baseline != nil {
		t.Fatalf("expected nil baseline, got %+v", ab.Baseline)
	}
	if ab.Draft == nil || ab.Draft.PassRate != 1.0 {
		t.Fatalf("unexpected draft: %+v", ab.Draft)
	}
}

// 无绑定数据集 → ErrNoReplayDataset（与单跑语义一致）。
func TestSkillReplayRunner_ReplayAB_NoDataset(t *testing.T) {
	eval := &fakeEvalDatasetReader{datasets: []evaluation.Dataset{{ID: "ds1", Name: "other"}}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, &fakeABLLMCaller{}, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	_, err := r.ReplayAB(context.Background(), "s1", "draft body", 0)
	if err != biz.ErrNoReplayDataset {
		t.Fatalf("expected ErrNoReplayDataset, got %v", err)
	}
}

// ── P3 M1：per-case verdict 收集（配对判定/等价检测的数据基础）───────────────

// 单跑回放逐 case 记录 CaseID/Passed/OutputHash，与 case 集顺序对齐。
func TestSkillReplayRunner_CaseResultsCollected(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "q1", ExpectedOutput: "Paris"},
			{ID: "c2", Input: "q2", ExpectedOutput: "Tokyo"},
		},
	}
	caller := &fakeReplayLLMCaller{outputs: []string{"Paris, France.", "London."}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	res, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(res.CaseResults) != 2 {
		t.Fatalf("expected 2 case verdicts, got %+v", res.CaseResults)
	}
	c1, c2 := res.CaseResults[0], res.CaseResults[1]
	if c1.CaseID != "c1" || !c1.Passed || c1.OutputHash == "" {
		t.Fatalf("unexpected c1 verdict: %+v", c1)
	}
	if c2.CaseID != "c2" || c2.Passed || c2.OutputHash == "" {
		t.Fatalf("unexpected c2 verdict: %+v", c2)
	}
	// 不同输出必须产生不同 hash（等价检测的分辨力）。
	if c1.OutputHash == c2.OutputHash {
		t.Fatal("different outputs must hash differently")
	}
}

// 相同输出（含首尾空白差异）必须产生相同 hash——等价检测判"无变化"的依据。
func TestSkillReplayRunner_OutputHashStable(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "q1", ExpectedOutput: "x"},
			{ID: "c2", Input: "q2", ExpectedOutput: "y"},
		},
	}
	caller := &fakeReplayLLMCaller{outputs: []string{"same output", "  same output  "}}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	res, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if res.CaseResults[0].OutputHash != res.CaseResults[1].OutputHash {
		t.Fatalf("trim-equivalent outputs must hash equal: %+v", res.CaseResults)
	}
}

// LLM 调用失败的 case：Passed=false 且 OutputHash 为空（不参与等价比较）。
func TestSkillReplayRunner_CaseLLMFailure_EmptyHash(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases:    []evaluation.Case{{ID: "c1", Input: "q1", ExpectedOutput: "x"}},
	}
	caller := &fakeReplayLLMCaller{err: errors.New("llm timeout")}
	skills := &fakeReplaySkillLookup{skill: biz.Skill{ID: "s1", Name: "web-research"}}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	res, err := r.Replay(context.Background(), "s1", "# draft", 0)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(res.CaseResults) != 1 || res.CaseResults[0].Passed || res.CaseResults[0].OutputHash != "" {
		t.Fatalf("failed call must yield passed=false hash='', got %+v", res.CaseResults)
	}
}

// AB 双端各自携带 per-case verdict，按 case ID 对齐可配对。
func TestSkillReplayRunner_ReplayAB_CaseResultsPairable(t *testing.T) {
	eval := &fakeEvalDatasetReader{
		datasets: []evaluation.Dataset{{ID: "ds1", Name: "web-research"}},
		cases: []evaluation.Case{
			{ID: "c1", Input: "q1", ExpectedOutput: "Paris"},
			{ID: "c2", Input: "q2", ExpectedOutput: "Tokyo"},
		},
	}
	caller := &fakeABLLMCaller{outputsBySystem: map[string][]string{
		"current body": {"Paris", "Tokyo"},
		"draft body":   {"Paris", "London"},
	}}
	skills := &fakeReplaySkillLookup{
		skill:    biz.Skill{ID: "s1", Name: "web-research"},
		markdown: "current body",
	}
	r := newTestReplayRunner(eval, caller, &fakeRefineLLMReader{
		setting: biz.RefineLLMSetting{Provider: "openai", Model: "gpt-4o"}}, skills)

	ab, err := r.ReplayAB(context.Background(), "s1", "draft body", 0)
	if err != nil {
		t.Fatalf("ReplayAB: %v", err)
	}
	if len(ab.Baseline.CaseResults) != 2 || len(ab.Draft.CaseResults) != 2 {
		t.Fatalf("both sides must carry per-case verdicts: %+v", ab)
	}
	for i := range ab.Baseline.CaseResults {
		if ab.Baseline.CaseResults[i].CaseID != ab.Draft.CaseResults[i].CaseID {
			t.Fatalf("case sets must align by ID at %d: %+v vs %+v",
				i, ab.Baseline.CaseResults[i], ab.Draft.CaseResults[i])
		}
	}
	// c1 双端同输出 → hash 相同；c2 输出不同 → hash 不同。
	if ab.Baseline.CaseResults[0].OutputHash != ab.Draft.CaseResults[0].OutputHash {
		t.Fatal("c1 identical outputs must hash equal across sides")
	}
	if ab.Baseline.CaseResults[1].OutputHash == ab.Draft.CaseResults[1].OutputHash {
		t.Fatal("c2 differing outputs must hash differently across sides")
	}
}
