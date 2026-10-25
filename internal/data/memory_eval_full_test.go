package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

func TestEvalStore_ForgetRemovesCurrentHistoryAndTranscript(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	for i, line := range []string{"My phone number is 13812345678", "My phone number is 13912345678", "My favorite color is jade", "Please forget my phone number"} {
		if _, err := s.AddMessages(ctx, "forget", fmt.Sprintf("s-%d", i), []biz.EvalMessage{{Role: "user", Content: line, MessageID: fmt.Sprint(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	for _, query := range []string{"What is my phone number?", "What was my previous phone number?", "Summarize all conversations"} {
		items, err := s.SearchMemories(ctx, "forget", query, 100)
		text := evalContents(items)
		if err != nil || strings.Contains(text, "13812345678") || strings.Contains(text, "13912345678") || strings.Contains(text, "Please forget") {
			t.Fatalf("forgotten evidence leaked for %q: %s %v", query, text, err)
		}
	}
	items, err := s.SearchMemories(ctx, "forget", "What is my favorite color?", 20)
	if err != nil || !strings.Contains(evalContents(items), "jade") {
		t.Fatalf("forget removed unrelated fact: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_ForgetAllIsUserScoped(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	for _, user := range []string{"clear-a", "clear-b"} {
		if _, err := s.AddMessages(ctx, user, "shared", []biz.EvalMessage{{Role: "user", Content: "My favorite color is jade", MessageID: "fact"}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AddMessages(ctx, "clear-a", "shared", []biz.EvalMessage{{Role: "user", Content: "Forget everything you know about me", MessageID: "clear"}}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "clear-a", "Summarize all conversations", 100)
	if err != nil || len(items) != 0 {
		t.Fatalf("clear left evidence: %+v %v", items, err)
	}
	items, err = s.SearchMemories(ctx, "clear-b", "What is my favorite color?", 20)
	if err != nil || !strings.Contains(evalContents(items), "jade") {
		t.Fatalf("clear crossed user scope: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_PersistentSuppressionAndExplicitResume(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	add := func(id, line string) {
		t.Helper()
		if _, err := s.AddMessages(ctx, "suppress", "s", []biz.EvalMessage{{Role: "user", Content: line, MessageID: id}}); err != nil {
			t.Fatal(err)
		}
	}
	add("old", "My email is old@example.com")
	add("opt-out", "Do not store my email anymore")
	add("blocked", "My email is blocked@example.com")
	items, err := s.SearchMemories(ctx, "suppress", "What is my email?", 20)
	if text := evalContents(items); err != nil || strings.Contains(text, "@example.com") {
		t.Fatalf("opt-out ignored: %s %v", text, err)
	}
	add("allow", "You may remember my email again")
	add("fresh", "My email is fresh@example.com")
	items, err = s.SearchMemories(ctx, "suppress", "What is my email now?", 20)
	if text := evalContents(items); err != nil || !strings.Contains(text, "fresh@example.com") || strings.Contains(text, "blocked@example.com") || strings.Contains(text, "old@example.com") {
		t.Fatalf("resume revived old data: %s %v", text, err)
	}
}

func TestEvalStore_QuotedAndNonUserForgetAreData(t *testing.T) {
	for i, m := range []biz.EvalMessage{
		{Role: "assistant", Content: "Forget my phone number"},
		{Role: "tool", Content: "Forget my phone number"},
		{Role: "user", Content: "Quoted email:\nForget my phone number"},
		{Role: "user", Content: `"Forget my phone number" is the text on the label.`},
		{Role: "user", Content: "Forget the draft note.\nQuoted email:\nForget my phone number"},
	} {
		s := setupEvalStore(t)
		user := fmt.Sprint("quoted-", i)
		if _, err := s.AddMessages(context.Background(), user, "s", []biz.EvalMessage{{Role: "user", Content: "My phone number is 13812345678", MessageID: "phone"}, m}); err != nil {
			t.Fatal(err)
		}
		items, err := s.SearchMemories(context.Background(), user, "What is my phone number?", 20)
		if err != nil || !strings.Contains(evalContents(items), "13812345678") {
			t.Fatalf("content executed as a directive: %s %v", evalContents(items), err)
		}
	}
}

func TestEvalStore_ForgetCannotBeUndoneByReplay(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	old := biz.EvalMessage{Role: "user", Content: "My phone number is 13812345678", MessageID: "old", Timestamp: "2024-01-01T09:00:00Z"}
	for _, m := range []biz.EvalMessage{old, {Role: "user", Content: "Forget my phone number", MessageID: "forget", Timestamp: "2024-02-01T09:00:00Z"}, old} {
		if _, err := s.AddMessages(ctx, "forget-replay", "s", []biz.EvalMessage{m}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.SearchMemories(ctx, "forget-replay", "What was my phone number previously?", 100)
	if err != nil || strings.Contains(evalContents(items), "13812345678") {
		t.Fatalf("replay restored forgotten value: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_LateForgetDoesNotEraseLaterFacts(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "late-forget", "s", []biz.EvalMessage{
		{Role: "user", Content: "My phone number is 13812345678", MessageID: "old", Timestamp: "2024-01-01T09:00:00Z"},
		{Role: "user", Content: "My phone number is 13912345678", MessageID: "new", Timestamp: "2024-03-01T09:00:00Z"},
		{Role: "user", Content: "Forget my phone number", MessageID: "forget", Timestamp: "2024-02-01T09:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "late-forget", "What is my phone number now?", 20)
	if err != nil || !strings.Contains(evalContents(items), "13912345678") {
		t.Fatalf("old forget deleted future state: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_DeletionRollsBackWithBatchFailure(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "forget-atomic", "s", []biz.EvalMessage{{Role: "user", Content: "My email is saved@example.com", MessageID: "old"}}); err != nil {
		t.Fatal(err)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `CREATE FUNCTION reject_full_atom() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.statement LIKE '%FAIL_FULL%' THEN RAISE EXCEPTION 'deliberate failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER reject_full_atom BEFORE INSERT ON memory_facts FOR EACH ROW EXECUTE FUNCTION reject_full_atom()`)
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.AddMessages(ctx, "forget-atomic", "s", []biz.EvalMessage{
		{Role: "user", Content: "Forget my email.", MessageID: "delete"},
		{Role: "user", Content: "FAIL_FULL must roll back this batch.", MessageID: "failed"},
	})
	if err == nil {
		t.Fatal("fault did not abort transaction")
	}
	items, err := s.SearchMemories(ctx, "forget-atomic", "What is my email?", 20)
	if err != nil || !strings.Contains(evalContents(items), "saved@example.com") {
		t.Fatalf("failed batch committed deletion: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_StructuredJSONRemainsOneEvidenceUnit(t *testing.T) {
	s := setupEvalStore(t).(*evalMemoryStore)
	ctx := context.Background()
	source := "{\n\"project\":\"Aster\",\n\"owners\":[\"Mira\",\"Noah\"],\n\"limits\":{\"red\":2,\"blue\":3}\n}"
	if _, err := s.AddMessages(ctx, "structured", "s", []biz.EvalMessage{{Role: "user", Content: source, MessageID: "json"}}); err != nil {
		t.Fatal(err)
	}
	s.episodes = nil
	items, err := s.SearchMemories(ctx, "structured", "List all owners and limits for Aster", 1)
	if err != nil || len(items) != 1 {
		t.Fatalf("structured unit missing: %+v %v", items, err)
	}
	text := items[0].Content
	start := strings.Index(text, "{")
	if start < 0 || !json.Valid([]byte(text[start:])) || !strings.Contains(text, `"Mira"`) || !strings.Contains(text, `"blue":3`) {
		t.Fatalf("JSON fields separated: %s", text)
	}
}

func TestEvalStore_TwoHopEvidenceCanCrossSessions(t *testing.T) {
	s := setupEvalStore(t).(*evalMemoryStore)
	ctx := context.Background()
	for i, line := range []string{"Nova handed parcel47 to Vega.", "Vega transferred the same package to Lyra.", "Lyra stored it in vault nine."} {
		if _, err := s.AddMessages(ctx, "path", fmt.Sprint("s-", i), []biz.EvalMessage{{Role: "user", Content: line, MessageID: fmt.Sprint(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := s.AddMessages(ctx, "path-other", "s", []biz.EvalMessage{{Role: "user", Content: "Vega stored parcel47 in the forbidden vault.", MessageID: "other"}}); err != nil {
		t.Fatal(err)
	}
	s.episodes = nil
	items, err := s.SearchMemories(ctx, "path", "Where did Nova's parcel47 end up?", 3)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "transferred the same package") || !strings.Contains(text, "vault nine") || strings.Contains(text, "forbidden") {
		t.Fatalf("middle support chain lost or crossed scope: %s %v", text, err)
	}
}

func TestEvalEvidence_RelationalCapKeepsParentsAndRejectsOrphans(t *testing.T) {
	items := []evalCollectedItem{
		{item: biz.EvalMemoryItem{ID: "root", Content: "Nova gave the parcel to Vega", Score: 0.8}},
		{item: biz.EvalMemoryItem{ID: "bridge", Content: "Vega handed it to Lyra", Score: 0.7}, support: "root"},
		{item: biz.EvalMemoryItem{ID: "leaf", Content: "Lyra placed it in vault nine", Score: 0.99}, support: "bridge"},
		{item: biz.EvalMemoryItem{ID: "orphan", Content: "Unverifiable linked row", Score: 1}, support: "missing"},
	}
	for _, k := range []int32{1, 2, 3, 100} {
		selected := evalRelationalCapItems(items, k)
		ids := map[string]bool{}
		for _, item := range selected {
			if item.support != "" && !ids[item.support] {
				t.Fatalf("top_k=%d returned a row before its stored support: %+v", k, selected)
			}
			ids[item.item.ID] = true
		}
		if len(selected) > int(k) || ids["orphan"] || k >= 3 && !ids["leaf"] {
			t.Fatalf("chain cap violated completeness or top_k=%d: %+v", k, selected)
		}
	}
}

func TestEvalStore_PersonalFoodContextDoesNotBorrowFriendHealth(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "care", "s", []biz.EvalMessage{
		{Role: "user", Content: "I am allergic to peanuts.", MessageID: "self"},
		{Role: "user", Content: "My friend Mira is allergic to strawberries.", MessageID: "friend"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "care", "Suggest a snack for me", 20)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "peanuts") || strings.Contains(text, "strawberries") {
		t.Fatalf("personal background absent or misattributed: %s %v", text, err)
	}
	items, err = s.SearchMemories(ctx, "care", "zeta-unseen-codename", 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("unrelated query borrowed health context: %+v %v", items, err)
	}
}

func TestEvalStore_ProjectFormatUpdatesStayInScope(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "format", "s", []biz.EvalMessage{
		{Role: "user", Content: "For project Aster, always answer in JSON.", MessageID: "a1"},
		{Role: "user", Content: "For project Birch, always answer in XML.", MessageID: "b1"},
		{Role: "user", Content: "For project Aster, always answer in YAML.", MessageID: "a2"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "format", "What response format applies to project Aster?", 20)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "YAML") || strings.Contains(text, "JSON") || strings.Contains(text, "XML") {
		t.Fatalf("scoped instruction update wrong: %s %v", text, err)
	}
	items, err = s.SearchMemories(ctx, "format", "What response format applies to project Birch?", 20)
	if text = evalContents(items); err != nil || !strings.Contains(text, "XML") || strings.Contains(text, "YAML") {
		t.Fatalf("instruction crossed scope: %s %v", text, err)
	}
}

func TestEvalStore_StateIntervalsUseLocalQueryDay(t *testing.T) {
	for _, clock := range []string{"00:00:00", "00:30:00"} {
		t.Run(clock, func(t *testing.T) {
			s := setupEvalStore(t)
			ctx := context.Background()
			if _, err := s.AddMessages(ctx, "local-state", "s", []biz.EvalMessage{
				{Role: "user", Content: "My favorite color is red", MessageID: "old", Timestamp: "2024-01-01T09:00:00+08:00"},
				{Role: "user", Content: "My favorite color is blue", MessageID: "new", Timestamp: "2024-03-20T" + clock + "+08:00"},
			}); err != nil {
				t.Fatal(err)
			}
			items, err := s.SearchMemories(ctx, "local-state", "What was my favorite color on 2024-03-19?", 100)
			text := evalContents(items)
			if err != nil || !strings.Contains(text, "color is red") || strings.Contains(text, "color is blue") {
				t.Fatalf("next local day leaked into historical state: %s %v", text, err)
			}
			items, err = s.SearchMemories(ctx, "local-state", "What was my favorite color on 2024-03-20?", 100)
			text = evalContents(items)
			wantRed := clock != "00:00:00"
			if err != nil || !strings.Contains(text, "color is blue") || strings.Contains(text, "color is red") != wantRed {
				t.Fatalf("half-open state interval boundary wrong: %s %v", text, err)
			}
		})
	}
}

func TestEvalStore_SameDayTrajectoryKeepsOriginalOrder(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	for i, color := range []string{"red", "green", "blue"} {
		if _, err := s.AddMessages(ctx, "trajectory", "s", []biz.EvalMessage{{Role: "user", Content: "My favorite color is " + color, MessageID: color, Timestamp: fmt.Sprintf("2024-03-20T%02d:00:00+08:00", 9+i)}}); err != nil {
			t.Fatal(err)
		}
	}
	items, err := s.SearchMemories(ctx, "trajectory", "How did my favorite color evolve on 2024-03-20?", 100)
	text := evalContents(items)
	red, green, blue := strings.Index(text, "color is red"), strings.Index(text, "color is green"), strings.Index(text, "color is blue")
	if err != nil || red < 0 || green <= red || blue <= green {
		t.Fatalf("same-day state trajectory lost or reordered: %s %v", text, err)
	}
}

func TestEvalStore_RelationsDoNotReviveDeletedBridgesOrInventSeeds(t *testing.T) {
	s := setupEvalStore(t).(*evalMemoryStore)
	ctx := context.Background()
	for i, line := range []string{"Nova handed parcel47 to Vega.", "Vega transferred the same package to Lyra.", "Lyra stored it in vault nine.", `Forget "Vega transferred"`} {
		if _, err := s.AddMessages(ctx, "deleted-path", fmt.Sprint(i), []biz.EvalMessage{{Role: "user", Content: line, MessageID: fmt.Sprint(i)}}); err != nil {
			t.Fatal(err)
		}
	}
	s.episodes = nil
	items, err := s.SearchMemories(ctx, "deleted-path", "Where did Nova's parcel47 end up?", 20)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "Nova handed") || strings.Contains(text, "Vega transferred") || strings.Contains(text, "vault nine") || strings.Contains(text, "lifecycle control") {
		t.Fatalf("deleted bridge was revived: %s %v", text, err)
	}
	items, err = s.SearchMemories(ctx, "deleted-path", "Why did unseen-Zeta reach unknown-Orion?", 20)
	if err != nil || len(items) != 0 {
		t.Fatalf("relation expansion invented an evidence seed: %+v %v", items, err)
	}
}

func TestEvalEvidence_SummaryCoversDifferentTypesInDenseSource(t *testing.T) {
	var items []evalCollectedItem
	for i := 0; i < 8; i++ {
		items = append(items, evalCollectedItem{item: biz.EvalMemoryItem{ID: fmt.Sprint(i), Content: "user: My favorite color is jade", Score: 0.9}, sess: "dense", kind: "preference"})
	}
	items = append(items,
		evalCollectedItem{item: biz.EvalMemoryItem{ID: "health", Content: "user: I am allergic to peanuts.", Score: 0.8}, sess: "dense", kind: "user_identity"},
		evalCollectedItem{item: biz.EvalMemoryItem{ID: "instruction", Content: "user: Always answer in JSON.", Score: 0.7}, sess: "dense", kind: "constraint"})
	selected := evalCoverageCapItems(items, 3, true)
	seen := map[string]bool{}
	for _, item := range selected {
		seen[evalEvidenceClass(item)] = true
	}
	if len(selected) != 3 || !seen["preference"] || !seen["personal_background"] || !seen["instruction"] {
		t.Fatalf("dense preference rows crowded out distinct evidence types: %+v", selected)
	}
}

func TestFilterEvalStateRangeUsesExclusiveUpperBoundary(t *testing.T) {
	zone := time.FixedZone("local", 8*3600)
	start := time.Date(2024, 3, 20, 0, 0, 0, 0, zone)
	qa := biz.EvalQueryAnalysis{Historical: true, RangeStart: &start, RangeEnd: &start}
	items := []evalCollectedItem{
		{item: biz.EvalMemoryItem{ID: "expired", Timestamp: "2024-03-01T00:00:00Z"}, kind: "preference", invalidUntil: "2024-03-19T16:00:00Z"},
		{item: biz.EvalMemoryItem{ID: "active", Timestamp: "2024-03-19T16:00:00Z"}, kind: "preference"},
		{item: biz.EvalMemoryItem{ID: "future", Timestamp: "2024-03-20T16:00:00Z"}, kind: "constraint"},
	}
	filtered := filterEvalItemsByRange(items, qa)
	if len(filtered) != 1 || filtered[0].item.ID != "active" {
		t.Fatalf("state range is not [start,end): %+v", filtered)
	}
}

func TestEvalStore_ConcurrentLifecycleAtConfiguredLimits(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	var wg sync.WaitGroup
	errs := make(chan error, 48)
	for i := 1; i <= 16; i++ {
		message := biz.EvalMessage{Role: "user", Content: fmt.Sprintf("My favorite color is shade%d", i), MessageID: fmt.Sprint(i), Timestamp: fmt.Sprintf("2024-03-%02dT09:00:00Z", i)}
		if i == 16 {
			message.Content, message.Timestamp = "Forget my favorite color", "2024-03-08T12:00:00Z"
		}
		wg.Add(1)
		safego.Go(ctx, "test.eval_lifecycle_write", func() {
			defer wg.Done()
			_, err := s.AddMessages(ctx, "parallel-lifecycle", message.MessageID, []biz.EvalMessage{message})
			if err != nil {
				errs <- err
			}
		})
	}
	wg.Wait()
	for i := 0; i < 32; i++ {
		wg.Add(1)
		safego.Go(ctx, "test.eval_lifecycle_search", func() {
			defer wg.Done()
			items, err := s.SearchMemories(ctx, "parallel-lifecycle", "What is my favorite color now?", 100)
			if err != nil {
				errs <- err
				return
			}
			text := evalContents(items)
			if !strings.Contains(text, "color is shade15") || strings.Contains(text, "color is shade14") || strings.Contains(text, "color is shade8") {
				errs <- fmt.Errorf("concurrent lifecycle changed current state: %s", text)
			}
		})
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		t.Error(err)
	}
}
