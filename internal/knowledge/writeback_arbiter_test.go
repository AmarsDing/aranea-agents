package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	bizknowledge "aranea-agents/internal/biz/knowledge"
)

// 自治理图谱 G-40 写回冲突仲裁器契约（四操作词表，Mem0 对齐）：
//   - LLM 返回 update/delete/noop 结论 → 解析为 Arbitration（add 直出同样透传；
//     未知操作丢弃，钉⑩⑫⑬）；
//   - 旧三值键/词兼容映射：supersedes→update、contradicts→delete、unrelated→noop
//     （G-40 D2，钉⑪）；
//   - noop 透出由消费面零旁效应跳过（不在解析层吃掉，钉⑬）；
//   - 空新事实或空既有段 → 不调用 LLM 直接返回 nil；
//   - LLM/解析失败 → 上抛 error（调用方降级直接追加）；
//   - 未接线（nil llm）→ 报错；未配置 LLM → 报错；
//   - 超长候选截断（existing>20 / news>10）控制 prompt 规模。

func arbiterTestBlocks() []bizknowledge.WriteBackFactBlock {
	return []bizknowledge.WriteBackFactBlock{
		{Heading: "constraint", Body: "## constraint\n\n灰度比例 5% 起步。\n\n- fact_id: `fid-old`", FactID: "fid-old"},
	}
}

func arbiterTestNews() []bizknowledge.WriteBackFact {
	return []bizknowledge.WriteBackFact{
		{FactID: "fid-new", Statement: "灰度比例提升至 20% 起步", FactKind: "constraint", Confidence: 0.95},
	}
}

// 钉⑩⑫⑬：四操作直出消费——update/noop 透出、未知操作丢弃；noop 不在解析层吃掉
// （透传给消费面零旁效应跳过）。
func TestWriteBackArbiter_ParsesFourOps(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{
		`[{"fact_index":0,"operation":"update","target_fact_id":"fid-old","confidence":0.92,"reason":"同一事实的更新"},
		  {"fact_index":1,"operation":"noop","target_fact_id":"","confidence":0.9,"reason":"已被覆盖"},
		  {"fact_index":2,"operation":"bogus","target_fact_id":"fid-old","confidence":0.9,"reason":"非法操作"}]`,
	}}
	a := NewWriteBackArbiter(llm, stubRelationSys{}, nil, nil)
	news := append(arbiterTestNews(),
		bizknowledge.WriteBackFact{FactID: "fid-x", Statement: "重复事实", FactKind: "decision"},
		bizknowledge.WriteBackFact{FactID: "fid-y", Statement: "另一条", FactKind: "decision"},
	)
	got, err := a.ArbitrateWriteBack(context.Background(), "灰度发布", arbiterTestBlocks(), news)
	if err != nil {
		t.Fatal(err)
	}
	if llm.calls != 1 {
		t.Fatalf("llm calls = %d, want 1 (batch)", llm.calls)
	}
	// update + noop 透出；未知操作丢弃。
	if len(got) != 2 {
		t.Fatalf("arbitrations = %+v, want 2 (update+noop)", got)
	}
	v := got[0]
	if v.FactIndex != 0 || v.Operation != bizknowledge.WriteBackOpUpdate || v.TargetFactID != "fid-old" || v.Confidence != 0.92 {
		t.Fatalf("update arbitration = %+v", v)
	}
	if got[1].FactIndex != 1 || got[1].Operation != bizknowledge.WriteBackOpNoop {
		t.Fatalf("noop must pass through: %+v", got[1])
	}
	// prompt 必须含既有段 fact_id 与新事实陈述（仲裁依据）+ 四操作词表。
	if !strings.Contains(llm.lastUser, "fid-old") || !strings.Contains(llm.lastUser, "20% 起步") {
		t.Fatalf("prompt missing evidence: %q", llm.lastUser)
	}
}

// 钉⑪：旧三值键/词映射（D2）——supersedes→update、contradicts→delete、
// unrelated→noop；旧键名 verdict 同样兼容。
func TestWriteBackArbiter_LegacyVerdictMapping(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{
		`[{"fact_index":0,"verdict":"supersedes","target_fact_id":"fid-old","confidence":0.9,"reason":"旧词-顶替"},
		  {"fact_index":1,"verdict":"contradicts","target_fact_id":"fid-old","confidence":0.85,"reason":"旧词-矛盾"},
		  {"fact_index":2,"verdict":"unrelated","target_fact_id":"","confidence":0.9,"reason":"旧词-无关"}]`,
	}}
	a := NewWriteBackArbiter(llm, stubRelationSys{}, nil, nil)
	news := append(arbiterTestNews(),
		bizknowledge.WriteBackFact{FactID: "fid-x", Statement: "另一条", FactKind: "decision"},
		bizknowledge.WriteBackFact{FactID: "fid-y", Statement: "无关事实", FactKind: "decision"},
	)
	got, err := a.ArbitrateWriteBack(context.Background(), "灰度发布", arbiterTestBlocks(), news)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("legacy vocabulary must all map: %+v", got)
	}
	want := []bizknowledge.WriteBackOp{bizknowledge.WriteBackOpUpdate, bizknowledge.WriteBackOpDelete, bizknowledge.WriteBackOpNoop}
	for i, w := range want {
		if got[i].Operation != w {
			t.Fatalf("legacy[%d] = %q, want %q (all %+v)", i, got[i].Operation, w, got)
		}
	}
}

func TestWriteBackArbiter_CodeFenceJSONTolerated(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{
		"```json\n[{\"fact_index\":0,\"operation\":\"delete\",\"target_fact_id\":\"fid-old\",\"confidence\":0.85,\"reason\":\"直接矛盾\"}]\n```",
	}}
	a := NewWriteBackArbiter(llm, stubRelationSys{}, nil, nil)
	got, err := a.ArbitrateWriteBack(context.Background(), "灰度发布", arbiterTestBlocks(), arbiterTestNews())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].Operation != bizknowledge.WriteBackOpDelete {
		t.Fatalf("code-fenced JSON must parse: %+v", got)
	}
}

func TestWriteBackArbiter_EmptyInputSkipsLLM(t *testing.T) {
	llm := &stubRelationLLM{}
	a := NewWriteBackArbiter(llm, stubRelationSys{}, nil, nil)
	got, err := a.ArbitrateWriteBack(context.Background(), "t", nil, arbiterTestNews())
	if err != nil || got != nil {
		t.Fatalf("empty existing: got=%v err=%v", got, err)
	}
	got, err = a.ArbitrateWriteBack(context.Background(), "t", arbiterTestBlocks(), nil)
	if err != nil || got != nil {
		t.Fatalf("empty news: got=%v err=%v", got, err)
	}
	if llm.calls != 0 {
		t.Fatalf("empty input must not call LLM, got %d", llm.calls)
	}
}

func TestWriteBackArbiter_LLMErrorPropagates(t *testing.T) {
	llm := &stubRelationLLM{err: errors.New("upstream 500")}
	a := NewWriteBackArbiter(llm, stubRelationSys{}, nil, nil)
	if _, err := a.ArbitrateWriteBack(context.Background(), "t", arbiterTestBlocks(), arbiterTestNews()); err == nil {
		t.Fatal("LLM error must propagate (caller degrades to append)")
	}
}

func TestWriteBackArbiter_BadJSONPropagates(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{"这不是 JSON"}}
	a := NewWriteBackArbiter(llm, stubRelationSys{}, nil, nil)
	if _, err := a.ArbitrateWriteBack(context.Background(), "t", arbiterTestBlocks(), arbiterTestNews()); err == nil {
		t.Fatal("unparseable response must return error")
	}
}

func TestWriteBackArbiter_NotWired(t *testing.T) {
	a := NewWriteBackArbiter(nil, stubRelationSys{}, nil, nil)
	if _, err := a.ArbitrateWriteBack(context.Background(), "t", arbiterTestBlocks(), arbiterTestNews()); err == nil {
		t.Fatal("nil llm must error")
	}
}

func TestWriteBackArbiter_TruncatesOversizedCandidates(t *testing.T) {
	llm := &stubRelationLLM{responses: []string{`[]`}}
	a := NewWriteBackArbiter(llm, stubRelationSys{}, nil, nil)
	existing := make([]bizknowledge.WriteBackFactBlock, 0, 25)
	for i := 0; i < 25; i++ {
		existing = append(existing, bizknowledge.WriteBackFactBlock{Heading: "constraint", Body: "段", FactID: "fid"})
	}
	news := make([]bizknowledge.WriteBackFact, 0, 15)
	for i := 0; i < 15; i++ {
		news = append(news, bizknowledge.WriteBackFact{FactID: "n", Statement: "s", FactKind: "decision"})
	}
	if _, err := a.ArbitrateWriteBack(context.Background(), "t", existing, news); err != nil {
		t.Fatal(err)
	}
	// existing 截 20、news 截 10：news 最大 index 为 9。
	if strings.Contains(llm.lastUser, `"index":10`) {
		t.Fatal("news must truncate to 10")
	}
	if llm.calls != 1 {
		t.Fatalf("llm calls = %d", llm.calls)
	}
}
