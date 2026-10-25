package data

import (
	"context"
	"strings"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestEvalMerge_RouterPreservesOfflineModes(t *testing.T) {
	s := newIntentTestStore(newEvalIntentTestEmbedder(nil))
	qa := biz.AnalyzeEvalQuery("List all fields in the workflow", time.Now())
	s.applyEvalQueryModes(context.Background(), &qa, evalIntentTestAxis(7, 1))
	if !qa.Enumerative || !qa.Aggregate || !qa.Procedural || !qa.Stepwise {
		t.Fatalf("neutral semantic routing cleared explicit modes: %+v", qa)
	}
}

func TestEvalMerge_DerivedRowsPreserveSourceCalendar(t *testing.T) {
	s := setupEvalStore(t).(*evalMemoryStore)
	ctx := context.Background()
	stamp := "2024-03-20T00:30:00+08:00"
	if _, err := s.AddMessages(ctx, "derived-anchor", "s", []biz.EvalMessage{
		{Role: "user", Content: "今天步数3200步。我叫张三", MessageID: "known", Timestamp: stamp},
	}); err != nil {
		t.Fatal(err)
	}
	anchor, known, err := s.evalSearchAnchor(ctx, "derived-anchor", time.Now())
	if err != nil || !known || anchor.Format(time.RFC3339Nano) != stamp {
		t.Fatalf("derived rows changed source calendar: %s %v %v", anchor, known, err)
	}
	if _, err := s.AddMessages(ctx, "unknown-derived-anchor", "s", []biz.EvalMessage{
		{Role: "user", Content: "步数3200步。我叫张三", MessageID: "unknown"},
	}); err != nil {
		t.Fatal(err)
	}
	_, known, err = s.evalSearchAnchor(ctx, "unknown-derived-anchor", time.Now())
	if err != nil || known {
		t.Fatalf("derived profile invented a source timestamp: known=%v %v", known, err)
	}
}

func TestEvalMerge_ForgetRebuildsAllDerivedGenerations(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	ctx := context.Background()
	for i, text := range []string{"我最喜欢的旅行城市是杭州", "我把最喜欢的旅行城市改成上海了", "今天步数3200步", "今天步数4500步", "今天热量1800卡", "今天热量1900卡"} {
		if _, err := s.AddMessages(ctx, "derived-forget", "s", []biz.EvalMessage{{Role: "user", Content: text, MessageID: text, Timestamp: time.Date(2024, 3, i+1, 9, 0, 0, 0, time.UTC).Format(time.RFC3339Nano)}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, text := range []string{`忘记"步数"`, `忘记"旅行城市"`} {
		if _, err := s.AddMessages(ctx, "derived-forget", "s", []biz.EvalMessage{{Role: "user", Content: text, MessageID: text, Timestamp: "2024-04-01T09:00:00Z"}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{"Summarize all conversations", "我最喜欢的旅行城市以前是什么", "我的步数变化趋势"} {
		items, err := s.SearchMemories(ctx, "derived-forget", query, 100)
		text := evalContents(items)
		if err != nil || strings.Contains(text, "杭州") || strings.Contains(text, "上海") || strings.Contains(text, "3200") || strings.Contains(text, "4500") {
			t.Fatalf("stale derived content leaked for %q: %s %v", query, text, err)
		}
	}
	retained := false
	for _, fact := range evalC3AllFacts(t, r, "derived-forget") {
		statement, _ := fact["statement"].(string)
		if strings.HasPrefix(statement, "[series-digest: 热量]") && fact["valid_until"] == "" {
			retained = strings.Contains(statement, "count=2") && strings.Contains(statement, "mean=1850.0")
		}
	}
	if !retained {
		t.Fatal("forgetting one series lost unrelated numeric observations")
	}
}

func TestEvalMerge_DerivedFailureRollsBackRawEvidence(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	ctx := context.Background()
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `CREATE FUNCTION reject_eval_derived() RETURNS trigger LANGUAGE plpgsql AS $$
	BEGIN IF NEW.source_kind='agent_memory_challenge_derived' THEN RAISE EXCEPTION 'synthetic derived failure'; END IF; RETURN NEW; END $$;
	CREATE TRIGGER reject_eval_derived BEFORE INSERT ON memory_facts FOR EACH ROW EXECUTE FUNCTION reject_eval_derived()`)
	if err != nil {
		t.Fatal(err)
	}
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	if _, err := s.AddMessages(ctx, "derived-atomic", "s", []biz.EvalMessage{{Role: "user", Content: "My name is Ada", MessageID: "name"}}); err == nil {
		t.Fatal("projection failure must fail the atomic Add")
	}
	if facts := evalC3AllFacts(t, r, "derived-atomic"); len(facts) != 0 {
		t.Fatalf("failed Add left raw evidence or a projection: %+v", facts)
	}
}
