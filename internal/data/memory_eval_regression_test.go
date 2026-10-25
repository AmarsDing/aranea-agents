package data

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/internal/data/vector"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

func TestEvalStore_OptionsRetrieveEvidenceWithoutInventingChoices(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "choice-user", "s", []biz.EvalMessage{{Role: "user", Content: "The rover identifier is obsidian-key17.", MessageID: "m"}}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "choice-user", "Which statement is consistent?", 10, "A. The rover identifier is obsidian-key17.", "B. The rover identifier is invented-code99.")
	if err != nil {
		t.Fatal(err)
	}
	text := evalContents(items)
	if !strings.Contains(text, "obsidian-key17") || strings.Contains(text, "invented-code99") || strings.Contains(text, "A. The rover") {
		t.Fatalf("choice text must only guide retrieval: %s", text)
	}
	items, err = store.SearchMemories(ctx, "other-user", "Which statement is consistent?", 10, "obsidian-key17")
	if err != nil || len(items) != 0 {
		t.Fatalf("choice expansion leaked across user scope: %+v %v", items, err)
	}
}

func TestEvalStore_NoImportanceOnlyEvidence(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "noise-user", "s", []biz.EvalMessage{{Role: "user", Content: "My favorite color is blue", MessageID: "m"}}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "noise-user", "zeta-unseen-codename", 10)
	if err != nil || len(items) != 0 {
		t.Fatalf("unrelated importance/recency is not evidence: %+v %v", items, err)
	}
	items, err = store.SearchMemories(ctx, "noise-user", "What do you know about me?", 10)
	if err != nil || !strings.Contains(evalContents(items), "blue") {
		t.Fatalf("a broad profile question needs user evidence: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_SourceAnchoredCalendarAndMultiDateEvidence(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	msgs := []biz.EvalMessage{
		{Role: "user", Content: "The comet inspection happened on 2024-02-29.", MessageID: "a", Timestamp: "2024-03-20T10:00:00Z"},
		{Role: "user", Content: "The comet shuttle departs 2024-02-06 and arrives 2024-02-08.", MessageID: "b", Timestamp: "2024-03-20T10:01:00Z"},
		{Role: "user", Content: "I checked the telescope.", MessageID: "c", Timestamp: "2024-03-21T10:00:00Z"},
	}
	if _, err := store.AddMessages(ctx, "dated-user", "s", msgs); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "dated-user", "What comet inspection happened last month?", 20)
	if err != nil || !strings.Contains(evalContents(items), "2024-02-29") {
		t.Fatalf("query used wall clock or collapsed month: %s, %v", evalContents(items), err)
	}
	items, err = store.SearchMemories(ctx, "dated-user", "Where was the comet shuttle on 2024-02-08?", 20)
	if err != nil || !strings.Contains(evalContents(items), "arrives 2024-02-08") {
		t.Fatalf("second date mention lost: %s, %v", evalContents(items), err)
	}
	items, err = store.SearchMemories(ctx, "dated-user", "What happened on 2024-03-21?", 20)
	if err != nil || !strings.Contains(evalContents(items), "telescope") {
		t.Fatalf("a date-only question needs temporally matching evidence: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_HistoricalValuesAndReplayWithoutTimestamp(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	old := []biz.EvalMessage{{Role: "user", Content: "My favorite color is blue", MessageID: "old"}}
	next := []biz.EvalMessage{{Role: "user", Content: "My favorite color is red", MessageID: "next"}}
	for _, msgs := range [][]biz.EvalMessage{old, next, old} {
		if _, err := store.AddMessages(ctx, "history-user", "s", msgs); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.SearchMemories(ctx, "history-user", "What is my favorite color now?", 20)
	if err != nil || !strings.Contains(evalContents(items), "red") || strings.Contains(evalContents(items), "blue") {
		t.Fatalf("timestamp-free replay revived stale state: %s %v", evalContents(items), err)
	}
	items, err = store.SearchMemories(ctx, "history-user", "What was my previous favorite color?", 20)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "blue") || !strings.Contains(text, "superseded") {
		t.Fatalf("history needs labeled stored old value: %s %v", text, err)
	}
}

func TestEvalStore_SameSessionIDStillIsolatesL2(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	for _, user := range []string{"tenant-alder", "tenant-birch"} {
		if _, err := store.AddMessages(ctx, user, "same-session", []biz.EvalMessage{{Role: "user", Content: "Project marker " + user, MessageID: "same-message"}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, user := range []string{"tenant-alder", "tenant-birch"} {
		items, err := store.SearchMemories(ctx, user, "Summarize all conversations about project markers", 20)
		text := evalContents(items)
		if err != nil || !strings.Contains(text, user) || strings.Contains(strings.ReplaceAll(text, user, ""), "tenant-") {
			t.Fatalf("L2 session collision for %s: %s %v", user, text, err)
		}
	}
	if _, err := store.SearchMemories(ctx, "", "project", 10); err == nil {
		t.Fatal("empty scope must not become a global search")
	}
}

func TestEvalStore_VectorSimilarityDoesNotInvalidateOtherAttribute(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	fvs := &fakeVectorStore{}
	store := NewEvalMemoryStore(r.data, evalStaticEmbedder{}, loggateway.NewNoop()).(*evalMemoryStore)
	store.facts = newL3FactRepo(r.data, fvs)
	store.syncer = nil
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "semantic-user", "s", []biz.EvalMessage{{Role: "user", Content: "My favorite color is blue", MessageID: "old"}}); err != nil {
		t.Fatal(err)
	}
	old := evalC3FindFact(t, evalC3AllFacts(t, r, "semantic-user"), "color is blue")
	fvs.hits = []vector.VectorHit{{ID: old["id"].(string), Score: 0.99}}
	if _, err := store.AddMessages(ctx, "semantic-user", "s", []biz.EvalMessage{{Role: "user", Content: "Actually, my favorite food is pasta", MessageID: "next"}}); err != nil {
		t.Fatal(err)
	}
	old = evalC3FindFact(t, evalC3AllFacts(t, r, "semantic-user"), "color is blue")
	if old["valid_until"] != "" {
		t.Fatalf("similarity deleted a different attribute: %v", old)
	}
}

func TestEvalStore_AddRollbackPreservesPreviousState(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "atomic-user", "s", []biz.EvalMessage{{Role: "user", Content: "My favorite color is blue", MessageID: "old"}}); err != nil {
		t.Fatal(err)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `CREATE FUNCTION reject_eval_test_atom() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.statement LIKE '%FAIL_ATOM%' THEN RAISE EXCEPTION 'deliberate test insert failure'; END IF; RETURN NEW; END $$;
	CREATE TRIGGER reject_eval_test_atom BEFORE INSERT ON memory_facts FOR EACH ROW EXECUTE FUNCTION reject_eval_test_atom()`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.AddMessages(ctx, "atomic-user", "s", []biz.EvalMessage{{Role: "user", Content: "My favorite color is red. FAIL_ATOM must abort this batch.", MessageID: "failed"}})
	if err == nil {
		t.Fatal("fault injection did not abort Add")
	}
	facts := evalC3AtomFacts(t, r, "atomic-user")
	if len(facts) != 1 || facts[0]["valid_until"] != "" {
		t.Fatalf("failed Add damaged previous state: %v", facts)
	}
}

func TestEvalStore_RuleEvidenceKeepsShortExamples(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	if _, err := store.AddMessages(ctx, "rule-user", "s", []biz.EvalMessage{{Role: "system", Content: "Rule: labels map to colors in the following examples.\nA=red\nB=blue\nC=green", MessageID: "rule"}}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "rule-user", "Apply the rule to label B", 20)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "B=blue") || !strings.Contains(text, "Rule:") {
		t.Fatalf("rule context or training example dropped: %s %v", text, err)
	}
}

func TestEvalStore_AggregateCanReachEarlySession(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop()).(*evalMemoryStore)
	ctx := context.Background()
	for i := 0; i < 9; i++ {
		text := fmt.Sprintf("Routine ledger entry %d", i)
		if i == 0 {
			text = "Botany notes: cedar thrives on dry ridges"
		}
		if _, err := store.AddMessages(ctx, "aggregate-user", fmt.Sprintf("s-%d", i), []biz.EvalMessage{{Role: "user", Content: text, MessageID: "m"}}); err != nil {
			t.Fatal(err)
		}
	}
	var items []evalCollectedItem
	store.appendAggregateEpisodes(ctx, "aggregate-user", "Summarize botany notes", time.Now(), map[string]struct{}{}, &items)
	found := false
	for _, item := range items {
		found = found || strings.Contains(item.item.Content, "cedar")
	}
	if !found {
		t.Fatal("aggregation only considered recent sessions")
	}
}

func TestEvalEvidenceExcerpt_RetainsRelevantTail(t *testing.T) {
	source := strings.Repeat("unrelated filler ", 400) + "terminal signature lavenderclock"
	got := evalEvidenceExcerpt(source, []string{"lavenderclock"}, 220)
	if !strings.Contains(got, "lavenderclock") || len([]rune(got)) > 240 {
		t.Fatalf("tail lost or budget exceeded: %s", got)
	}
}

func TestMaskEvalEvidence_MinimalDisclosure(t *testing.T) {
	content := "联系方式alice@example.com，社保号123-45-6789，api_key=private-demo-value；手机号13812345678; phone +1-415-555-1212; home 123 Maple Street"
	got := MaskEvalEvidence(content, "What is the user's favorite color?")
	for _, secret := range []string{"alice@example.com", "123-45-6789", "private-demo-value", "13812345678", "415-555-1212", "123 Maple Street"} {
		if strings.Contains(got, secret) {
			t.Errorf("unrequested identifier %q leaked: %s", secret, got)
		}
	}
	if got := MaskEvalEvidence(content, "用户的邮箱是什么"); !strings.Contains(got, "alice@example.com") || strings.Contains(got, "13812345678") {
		t.Fatalf("category-specific disclosure broken: %s", got)
	}
}

func TestEvalStore_OutOfOrderUpdatesKeepNewestState(t *testing.T) {
	store := setupEvalStore(t)
	ctx := context.Background()
	newer := biz.EvalMessage{Role: "user", Content: "My favorite color is red", MessageID: "newer", Timestamp: "2024-03-20T10:00:00Z"}
	older := biz.EvalMessage{Role: "user", Content: "My favorite color is blue", MessageID: "older", Timestamp: "2024-01-01T10:00:00Z"}
	if _, err := store.AddMessages(ctx, "late-user", "s", []biz.EvalMessage{newer}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.AddMessages(ctx, "late-user", "s", []biz.EvalMessage{older}); err != nil {
		t.Fatal(err)
	}
	items, err := store.SearchMemories(ctx, "late-user", "What is my favorite color now?", 20)
	if err != nil || strings.Contains(evalContents(items), "blue") || !strings.Contains(evalContents(items), "red") {
		t.Fatalf("late old message became current: %s %v", evalContents(items), err)
	}
	items, err = store.SearchMemories(ctx, "late-user", "What was my favorite color on 2024-02-10?", 20)
	if err != nil || !strings.Contains(evalContents(items), "blue") || strings.Contains(evalContents(items), "red") {
		t.Fatalf("historical state interval lost: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_ConcurrentUpdatesSerializePerUser(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	store := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	for _, m := range []biz.EvalMessage{
		{Role: "user", Content: "My favorite color is blue", MessageID: "old", Timestamp: "2024-01-01T10:00:00Z"},
		{Role: "user", Content: "My favorite color is red", MessageID: "new", Timestamp: "2024-03-20T10:00:00Z"},
	} {
		wg.Add(1)
		m := m
		safego.Go(ctx, "test.eval_concurrent_update", func() {
			defer wg.Done()
			_, err := store.AddMessages(ctx, "parallel-user", "s", []biz.EvalMessage{m})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.SearchMemories(ctx, "parallel-user", "What is my favorite color now?", 20)
	if err != nil || strings.Contains(evalContents(items), "blue") || !strings.Contains(evalContents(items), "red") {
		t.Fatalf("concurrent write left two current states: %s %v", evalContents(items), err)
	}
}
