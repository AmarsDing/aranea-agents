package biz

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

// ── P3 M5-A: 进化元数据维度（dims）────────────────────────────────────────
//
// EverMind GSME 防搜索塌缩的务实落地：trigger 产出建议时在 metadata 写入
// 确定性可得的维度标签（dims.tools），供平台级多样性聚合观测——某维度长期
// 无新建议即塌缩信号。维度不做 LLM 推断（贵且不稳定），只取确定信号。

func TestNormalizeToolNames(t *testing.T) {
	got := normalizeToolNames([]string{" shell_exec ", "query_db", "shell_exec", "", "query_db", "api_call"})
	want := []string{"api_call", "query_db", "shell_exec"} // trim + 去重 + 排序
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v want %v", got, want)
		}
	}
	if got := normalizeToolNames(nil); got != nil {
		t.Fatalf("nil in must be nil out, got %v", got)
	}
	if got := normalizeToolNames([]string{" ", ""}); got != nil {
		t.Fatalf("all-empty must be nil, got %v", got)
	}
}

func TestWithDimsTools(t *testing.T) {
	// 空 tools 不写 dims 键（避免 `{}` 噪声，聚合端靠 IS NOT NULL 过滤）。
	kv := withDimsTools(map[string]any{"k": "v"}, nil)
	if _, ok := kv[EvoMetaDims]; ok {
		t.Fatalf("empty tools must not write dims, got %v", kv)
	}
	kv = withDimsTools(map[string]any{"k": "v"}, []string{"b", "a", "a"})
	raw, ok := kv[EvoMetaDims]
	if !ok {
		t.Fatal("dims key missing")
	}
	dims, ok := raw.(EvolutionDims)
	if !ok {
		t.Fatalf("dims must be EvolutionDims, got %T", raw)
	}
	if len(dims.Tools) != 2 || dims.Tools[0] != "a" || dims.Tools[1] != "b" {
		t.Fatalf("tools=%v", dims.Tools)
	}
	// JSON 形态断言：聚合 SQL 依赖 dims.tools 为 string array。
	buf, _ := json.Marshal(kv)
	var decoded struct {
		Dims struct {
			Tools []string `json:"tools"`
		} `json:"dims"`
	}
	if err := json.Unmarshal(buf, &decoded); err != nil || len(decoded.Dims.Tools) != 2 {
		t.Fatalf("json roundtrip: %v %s", err, buf)
	}
}

// ── PatternTrigger dims 写入 ──

type fakeDimsPatternReader struct {
	patterns []Pattern
}

func (f *fakeDimsPatternReader) ListByAgent(_ context.Context, _ string, _ string) ([]Pattern, error) {
	return f.patterns, nil
}

func (f *fakeDimsPatternReader) GetByID(_ context.Context, id string) (Pattern, error) {
	for _, p := range f.patterns {
		if p.ID == id {
			return p, nil
		}
	}
	return Pattern{}, errors.New("not found") // 本测试路径不调用
}

type fakeDimsCreator struct{}

func (f *fakeDimsCreator) GenerateSKILLMD(_ context.Context, _ string, _ []ToolCallRecord) (string, string, error) {
	return "auto-skill", "# body", nil
}

type fakeDimsRegistrar struct{}

func (f *fakeDimsRegistrar) SkillExists(_ context.Context, _ string, _ string) (bool, error) {
	return false, nil
}

func (f *fakeDimsRegistrar) RegisterSkill(_ context.Context, _ string, _ string, _ string) error {
	return nil
}

func TestPatternTrigger_WritesDimsTools(t *testing.T) {
	tr := NewPatternTrigger(
		&fakeEvoSettings{evolve: true},
		&fakeDimsPatternReader{patterns: []Pattern{{
			ID: "p1", AgentID: "ag-1", Kind: string(ObservationKindToolCall),
			Description: "shell_exec(10), query_db(3), shell_exec(2)",
			Confidence:  0.9, Status: PatternStatusDetected, DetectedAt: time.Now().UTC(),
		}}},
		&fakeDimsCreator{},
		&fakeDimsRegistrar{},
		nil, // patternReader=nil：跳过 hash 去重
		loggateway.NewNoop(),
	)
	got, err := tr.Check(context.Background(), "ag-1")
	if err != nil || len(got) != 1 {
		t.Fatalf("want 1 suggestion, got %v %v", got, err)
	}
	raw := got[0].MetaRaw(EvoMetaDims)
	if raw == nil {
		t.Fatal("metadata must carry dims")
	}
	var dims EvolutionDims
	if err := json.Unmarshal(raw, &dims); err != nil {
		t.Fatalf("dims unmarshal: %v", err)
	}
	// 去重 + 排序。
	if len(dims.Tools) != 2 || dims.Tools[0] != "query_db" || dims.Tools[1] != "shell_exec" {
		t.Fatalf("tools=%v", dims.Tools)
	}
}

// ── CaseDistillTrigger dims 写入 ──

func TestCaseDistillTrigger_WritesDimsToolsUnion(t *testing.T) {
	cases := makeCases(caseDistillMinCases + 1)
	cases[0].ToolsUsed = []string{"query_db", "shell_exec"}
	cases[1].ToolsUsed = []string{"shell_exec", "api_call"}
	tr := NewCaseDistillTrigger(
		&fakeEvoSettings{evolve: true},
		&fakeCaseRecallSource{cases: cases},
		&fakeCaseDistiller{name: "x", body: "y"},
		loggateway.NewNoop(),
	)
	got, err := tr.Check(context.Background(), "ag-1")
	if err != nil || len(got) != 1 {
		t.Fatalf("want 1 suggestion, got %v %v", got, err)
	}
	raw := got[0].MetaRaw(EvoMetaDims)
	if raw == nil {
		t.Fatal("metadata must carry dims")
	}
	var dims EvolutionDims
	if err := json.Unmarshal(raw, &dims); err != nil {
		t.Fatalf("dims unmarshal: %v", err)
	}
	// 跨 Case 并集，去重排序。
	want := []string{"api_call", "query_db", "shell_exec"}
	if len(dims.Tools) != len(want) {
		t.Fatalf("tools=%v want %v", dims.Tools, want)
	}
	for i := range want {
		if dims.Tools[i] != want[i] {
			t.Fatalf("tools=%v want %v", dims.Tools, want)
		}
	}
}
