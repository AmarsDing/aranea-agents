package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	bizknowledge "aranea-agents/internal/biz/knowledge"
	"aranea-agents/pkg/loggateway"
)

// G-37 实体消解裁决器（ADR-KN-20261021-F 生产实现）测试：严解析 fail-close
//（围栏/畸形 JSON/非 yes/解析失败）· usage 记账（成败/零 token 成功跳账）·
// 未接线门禁。

type stubEMLLMCaller struct {
	resp    string
	tokens  int
	err     error
	calls   int
	gotUser string
	gotSys  string
}

func (s *stubEMLLMCaller) Call(_ context.Context, req biz.LLMCallRequest) (string, int, error) {
	s.calls++
	s.gotUser, s.gotSys = req.User, req.System
	return s.resp, s.tokens, s.err
}

type stubEMSysGet struct{}

func (stubEMSysGet) Get(context.Context) (biz.SystemSetting, error) {
	return biz.SystemSetting{DefaultRefineLLM: biz.RefineLLMSetting{Provider: "openai", Model: "m1"}}, nil
}

type stubEMRec struct{ rows []EntityMergeUsageInput }

func (s *stubEMRec) RecordEntityMergeUsage(_ context.Context, in EntityMergeUsageInput) {
	s.rows = append(s.rows, in)
}

func emReq() bizknowledge.EntityMergeAdjudicateRequest {
	return bizknowledge.EntityMergeAdjudicateRequest{
		CollectionID: "c1", KeeperID: 1, KeeperName: "RAG", KeeperType: "tech",
		MergeeID: 2, MergeeName: "检索增强生成", MergeeType: "tech", Similarity: 0.86,
	}
}

func newEMAdjudicator(caller biz.LLMCaller) (*EntityMergeAdjudicator, *stubEMRec) {
	a := NewEntityMergeAdjudicator(caller, stubEMSysGet{}, nil, loggateway.NewNoop())
	rec := &stubEMRec{}
	a.SetUsageRecorder(rec)
	return a, rec
}

// yes 判定 + reason ≤120 runes 截断；usage 成功行记账。
func TestEntityMergeAdjudicator_Yes(t *testing.T) {
	t.Parallel()
	long := strings.Repeat("同", 130)
	caller := &stubEMLLMCaller{resp: `{"verdict":"yes","reason":"` + long + `"}`, tokens: 11}
	a, rec := newEMAdjudicator(caller)
	verdict, err := a.AdjudicateEntityMerge(context.Background(), emReq())
	if err != nil || !verdict.Verdict {
		t.Fatalf("adjudicate: %v %+v", err, verdict)
	}
	if got := len([]rune(verdict.Reason)); got != entityMergeReasonMaxRunes {
		t.Fatalf("reason 应截 120 runes: %d", got)
	}
	if len(rec.rows) != 1 || rec.rows[0].Tokens != 11 || rec.rows[0].KeeperID != 1 || rec.rows[0].MergeeID != 2 {
		t.Fatalf("usage 行: %+v", rec.rows)
	}
	if !strings.Contains(caller.gotUser, "RAG") || !strings.Contains(caller.gotUser, "检索增强生成") || !strings.Contains(caller.gotUser, "0.86") {
		t.Fatalf("裁决证据未注入 prompt: %q", caller.gotUser)
	}
}

// no 判定：静默成功零提案（Verdict=false，无 error）；usage 照账。
func TestEntityMergeAdjudicator_No(t *testing.T) {
	t.Parallel()
	caller := &stubEMLLMCaller{resp: `{"verdict":"no","reason":"名称不同义"}`, tokens: 9}
	a, rec := newEMAdjudicator(caller)
	v, err := a.AdjudicateEntityMerge(context.Background(), emReq())
	if err != nil || v.Verdict {
		t.Fatalf("no 判定: %v %+v", err, v)
	}
	if len(rec.rows) != 1 || rec.rows[0].Tokens != 9 || rec.rows[0].Err != nil {
		t.Fatalf("usage 行: %+v", rec.rows)
	}
}

// ```json 围栏剥除；大小写容忍（"YES" → yes）。
func TestEntityMergeAdjudicator_FencedJSON(t *testing.T) {
	t.Parallel()
	caller := &stubEMLLMCaller{resp: "```json\n{\"verdict\":\"YES\",\"reason\":\"缩写同义\"}\n```", tokens: 5}
	a, _ := newEMAdjudicator(caller)
	v, err := a.AdjudicateEntityMerge(context.Background(), emReq())
	if err != nil || !v.Verdict || v.Reason != "缩写同义" {
		t.Fatalf("围栏/大小写: %v %+v", err, v)
	}
}

// 畸形 JSON / verdict 畸形值 / 空体：fail-close 为零提案零错误（ADR-F 零噪音）。
func TestEntityMergeAdjudicator_FailClose(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{
		"not json at all",
		`{"verdict":"maybe","reason":"x"}`,
		`{"foo":"bar"}`,
		"",
	} {
		caller := &stubEMLLMCaller{resp: bad, tokens: 4}
		a, rec := newEMAdjudicator(caller)
		v, err := a.AdjudicateEntityMerge(context.Background(), emReq())
		if err != nil || v.Verdict {
			t.Fatalf("bad=%q 应 fail-close no: %v %+v", bad, err, v)
		}
		if len(rec.rows) != 1 {
			t.Fatalf("bad=%q 成功调用仍记账: %d", bad, len(rec.rows))
		}
	}
}

// LLM 调用失败：error 上抛（biz 侧连败熔断记账），失败行 usage 照记。
func TestEntityMergeAdjudicator_LLMError(t *testing.T) {
	t.Parallel()
	caller := &stubEMLLMCaller{err: errors.New("llm down"), tokens: 0}
	a, rec := newEMAdjudicator(caller)
	_, err := a.AdjudicateEntityMerge(context.Background(), emReq())
	if err == nil {
		t.Fatal("调用失败应上抛")
	}
	if len(rec.rows) != 1 || rec.rows[0].Err == nil || rec.rows[0].Tokens != 0 {
		t.Fatalf("失败 usage 行: %+v", rec.rows)
	}
}

// 零 token 成功行跳账（F12 落盘截断防幻影计数——G-34 先例）。
func TestEntityMergeAdjudicator_ZeroTokenSuccessSkipped(t *testing.T) {
	t.Parallel()
	caller := &stubEMLLMCaller{resp: `{"verdict":"yes","reason":"ok"}`, tokens: 0}
	a, rec := newEMAdjudicator(caller)
	v, err := a.AdjudicateEntityMerge(context.Background(), emReq())
	if err != nil || !v.Verdict {
		t.Fatalf("adjudicate: %v %+v", err, v)
	}
	if len(rec.rows) != 0 {
		t.Fatalf("零 token 成功行应跳账: %d", len(rec.rows))
	}
}

// 未接线：nil caller → 错；ENV 开关函数语义（t.Setenv 禁并行——不设 t.Parallel）。
func TestEntityMergeAdjudicator_Unwired(t *testing.T) {
	a := NewEntityMergeAdjudicator(nil, stubEMSysGet{}, nil, loggateway.NewNoop())
	if _, err := a.AdjudicateEntityMerge(context.Background(), emReq()); err == nil {
		t.Fatal("nil caller 应错")
	}
	t.Setenv("KNOWLEDGE_ENTITY_MERGE_ADJUDICATOR_DISABLED", "1")
	if !EntityMergeAdjudicatorDisabled() {
		t.Fatal("ENV=1 应禁用")
	}
	t.Setenv("KNOWLEDGE_ENTITY_MERGE_ADJUDICATOR_DISABLED", "")
	if EntityMergeAdjudicatorDisabled() {
		t.Fatal("ENV 空应启用")
	}
}
