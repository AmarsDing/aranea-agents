package data

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestEvalStore_LateIntermediateStateRepairsHistory(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	for i, m := range []biz.EvalMessage{
		{Content: "My favorite color is red", Timestamp: "2024-01-01T09:00:00Z"},
		{Content: "My favorite color is blue", Timestamp: "2024-03-01T09:00:00Z"},
		{Content: "My favorite color is green", Timestamp: "2024-02-01T09:00:00Z"},
	} {
		m.Role, m.MessageID = "user", fmt.Sprint(i)
		if _, err := s.AddMessages(context.Background(), "chain", "s", []biz.EvalMessage{m}); err != nil {
			t.Fatal(err)
		}
	}
	red := evalC3FindFact(t, evalC3AllFacts(t, r, "chain"), "color is red")
	if red["valid_until"] != "2024-02-01T09:00:00Z" {
		t.Fatalf("late middle state left overlapping history: %v", red)
	}
	items, err := s.SearchMemories(context.Background(), "chain", "What was my favorite color on 2024-02-15?", 20)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "green") || strings.Contains(text, "color is red") || strings.Contains(text, "color is blue") {
		t.Fatalf("as-of state is not unique: %s %v", text, err)
	}
}

func TestEvalStore_GovernanceFindsEarlyAttribute(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "deep", "s", []biz.EvalMessage{{Role: "user", Content: "My favorite color is blue", MessageID: "old", Timestamp: "2024-01-01T09:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	_, err := r.data.RWDB().WriteDB(ctx).ExecContext(ctx, `INSERT INTO memory_facts(id,scope_type,scope_id,user_id,fingerprint,statement,statement_normalized,fact_kind,status,created_at,updated_at,valid_from,metadata_json)
		SELECT 'filler-'||n,'user','deep','deep','fingerprint-'||n,'My favorite invented attribute '||n||' is value','filler '||n,'preference','active','2024-02-01T09:00:00Z','2024-02-01T09:00:00Z','2024-02-01T09:00:00Z','{"governance_key":"unrelated"}' FROM generate_series(1,2100) n`)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.AddMessages(ctx, "deep", "s", []biz.EvalMessage{{Role: "user", Content: "My favorite color is red", MessageID: "new", Timestamp: "2024-03-01T09:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	old := evalC3FindFact(t, evalC3AllFacts(t, r, "deep"), "color is blue")
	if old["valid_until"] == "" {
		t.Fatal("early current attribute escaped governance")
	}
}

func TestEvalStore_PastResidenceCannotReplacePresent(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "past", "s", []biz.EvalMessage{
		{Role: "user", Content: "我住在杭州", MessageID: "current", Timestamp: "2024-03-01T09:00:00Z"},
		{Role: "user", Content: "我以前住在北京", MessageID: "past", Timestamp: "2024-03-02T09:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "past", "我现在住在哪里", 20)
	if text := evalContents(items); err != nil || !strings.Contains(text, "杭州") || strings.Contains(text, "北京") {
		t.Fatalf("past state leaked into current answer: %s %v", text, err)
	}
	items, err = s.SearchMemories(ctx, "past", "我以前住在哪里", 20)
	if err != nil || !strings.Contains(evalContents(items), "北京") {
		t.Fatalf("past evidence lost: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_DateHeadingDatesFollowingAtoms(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	if _, err := s.AddMessages(context.Background(), "diary", "s", []biz.EvalMessage{{Role: "user", Content: "2024-02-06：\nWe calibrated the spectrometer.\nThe prism passed inspection.", MessageID: "diary", Timestamp: "2024-03-20T09:00:00Z"}}); err != nil {
		t.Fatal(err)
	}
	f := evalC3FindFact(t, evalC3AllFacts(t, r, "diary"), "prism")
	var metadata struct {
		Spans []biz.EvalDateSpan `json:"event_spans"`
	}
	if err := json.Unmarshal([]byte(f["metadata_json"].(string)), &metadata); err != nil {
		t.Fatal(err)
	}
	if len(metadata.Spans) != 1 || metadata.Spans[0].Start != "2024-02-06" {
		t.Fatalf("heading date lost: %+v", metadata)
	}
}

func TestEvalStore_DateFilteringPrecedesCandidateCap(t *testing.T) {
	s := setupEvalStore(t)
	msgs := []biz.EvalMessage{{Role: "user", Content: "The spectrometer found the amber signature.", MessageID: "target", Timestamp: "2024-05-20T09:00:00Z"}}
	for i := 0; i < 150; i++ {
		for _, date := range []string{"2024-01-01T09:00:00Z", "2024-08-01T09:00:00Z"} {
			msgs = append(msgs, biz.EvalMessage{Role: "user", Content: fmt.Sprintf("Routine unrelated ledger %d at %s", i, date), MessageID: fmt.Sprint(i, date), Timestamp: date})
		}
	}
	if _, err := s.AddMessages(context.Background(), "calendar-cap", "s", msgs); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(context.Background(), "calendar-cap", "What happened on 2024-05-20?", 1)
	if err != nil || !strings.Contains(evalContents(items), "amber signature") {
		t.Fatalf("in-range evidence lost before filter: %s %v", evalContents(items), err)
	}
}

func TestEvalStore_RuleMessageSurvivesSmallTopK(t *testing.T) {
	s := setupEvalStore(t).(*evalMemoryStore)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "rule-card", "s", []biz.EvalMessage{{Role: "system", MessageID: "training", Content: "Rule: use the following calibration samples.\nA=red\nB=blue\nC=green"}}); err != nil {
		t.Fatal(err)
	}
	s.episodes = nil // Isolate the L3 path; no full-transcript fallback can hide a lost sample.
	items, err := s.SearchMemories(ctx, "rule-card", "Apply the calibration rule to label B", 1)
	text := evalContents(items)
	if err != nil || len(items) != 1 || !strings.Contains(text, "Rule:") || !strings.Contains(text, "B=blue") || !strings.Contains(text, "C=green") {
		t.Fatalf("rule/sample unit broken at top_k=1: %s %v", text, err)
	}
}

func TestEvalStore_GlobalSummaryCoversSessions(t *testing.T) {
	s := setupEvalStore(t).(*evalMemoryStore)
	ctx := context.Background()
	for session, lines := range map[string][]string{
		"dense":    {"Botany covers cedar", "Finance covers taxes", "Astronomy covers pulsars", "Music covers harmony", "Chemistry covers enzymes", "Architecture covers arches"},
		"sparse-a": {"Botany covers spruce"}, "sparse-b": {"Finance covers rebates"},
	} {
		var msgs []biz.EvalMessage
		stamp := "2024-01-01T09:00:00Z"
		if session == "sparse-a" {
			stamp = "2024-02-01T09:00:00Z"
		}
		if session == "sparse-b" {
			stamp = "2024-03-01T09:00:00Z"
		}
		for i, line := range lines {
			msgs = append(msgs, biz.EvalMessage{Role: "user", Content: line, MessageID: fmt.Sprint(i), Timestamp: stamp})
		}
		if _, err := s.AddMessages(ctx, "summary-cover", session, msgs); err != nil {
			t.Fatal(err)
		}
	}
	s.episodes = nil
	items, err := s.SearchMemories(ctx, "summary-cover", "Summarize all conversations", 3)
	if err != nil {
		t.Fatal(err)
	}
	for _, session := range []string{"dense", "sparse-a", "sparse-b"} {
		if !strings.Contains(evalContents(items), "sess-"+session+"]") {
			t.Fatalf("summary omitted %s: %s", session, evalContents(items))
		}
	}
	covered := map[string]bool{}
	err = s.appendEvalCandidates(ctx, "summary-cover", []string{"Summarize all conversations"}, biz.EvalQueryAnalysis{Aggregate: true}, 3, func(raw []byte, _ float64) { covered[factJSONString(raw, "source_session_id")] = true })
	if err != nil || len(covered) != 3 {
		t.Fatalf("candidate cap lost sparse sources: %v %v", covered, err)
	}
}

func TestEvalEvidenceExcerpt_PrefersSpecificTail(t *testing.T) {
	source := "Rules for the calibration. " + strings.Repeat("ordinary filler ", 500) + " The azurite sample maps to Q7."
	got := evalEvidenceExcerpt(source, []string{"Apply rules to the azurite sample"}, 160)
	if !strings.Contains(got, "azurite") {
		t.Fatalf("common early word displaced specific evidence: %q", got)
	}
	_ = evalEvidenceExcerpt(strings.Repeat("İ", 150)+" unique-tail", []string{"unique-tail"}, 40)
}

func TestEvalStore_FunctionTransferRetrievesStoredDemonstrations(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	var msgs []biz.EvalMessage
	for i, text := range []string{"f(1)=4", "f (2)=7", "f(3)=10"} {
		msgs = append(msgs, biz.EvalMessage{Role: "user", Content: text, MessageID: fmt.Sprint(i)})
	}
	if _, err := s.AddMessages(ctx, "transfer", "s", msgs); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "transfer", "What is f(8)?", 20)
	text := evalContents(items)
	if err != nil || !strings.Contains(text, "f(1)=4") || !strings.Contains(text, "f (2)=7") || !strings.Contains(text, "f(3)=10") || strings.Contains(text, "f(8)=25") {
		t.Fatalf("transfer must return demonstrations, not an answer: %s %v", text, err)
	}
}

func TestEvalStore_LocalCalendarAnchorAcrossMidnight(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "local-day", "s", []biz.EvalMessage{{Role: "user", MessageID: "m", Content: "Yesterday I calibrated the spectrometer.", Timestamp: "2024-03-20T00:30:00+08:00"}}); err != nil {
		t.Fatal(err)
	}
	items, err := s.SearchMemories(ctx, "local-day", "What happened yesterday?", 10)
	if text := evalContents(items); err != nil || !strings.Contains(text, "[2024-03-19") || !strings.Contains(text, "spectrometer") {
		t.Fatalf("source timezone lost: %s %v", text, err)
	}
}

func TestEvalStore_CalendarOnlyPeriodQueries(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "periods", "s", []biz.EvalMessage{
		{Role: "user", Content: "I catalogued the amber prism.", MessageID: "old", Timestamp: "2023-05-01T09:00:00Z"},
		{Role: "user", Content: "I calibrated the blue spectrometer.", MessageID: "new", Timestamp: "2024-03-19T09:00:00Z"},
		{Role: "user", Content: "I finished the alignment.", MessageID: "anchor", Timestamp: "2024-03-20T09:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ query, want, absent string }{
		{"What happened in 2023?", "amber prism", "blue spectrometer"},
		{"What happened this month?", "blue spectrometer", "amber prism"},
		{"What happened on March 19?", "blue spectrometer", "alignment"},
	} {
		items, err := s.SearchMemories(ctx, "periods", tc.query, 10)
		text := evalContents(items)
		if err != nil || !strings.Contains(text, tc.want) || strings.Contains(text, tc.absent) {
			t.Errorf("%q: %s %v", tc.query, text, err)
		}
	}
}

func TestEvalStore_SingleValueAttributesUpdateIndependently(t *testing.T) {
	s := setupEvalStore(t)
	ctx := context.Background()
	var msgs []biz.EvalMessage
	for i, line := range []string{"My email is old@example.com", "My timezone is Europe/Rome", "My preferred language is English", "My email is new@example.com", "My timezone is Asia/Shanghai", "My preferred language is Chinese"} {
		msgs = append(msgs, biz.EvalMessage{Role: "user", Content: line, MessageID: fmt.Sprint(i)})
	}
	if _, err := s.AddMessages(ctx, "attributes", "s", msgs); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct{ query, want, absent string }{
		{"What is my email now?", "new@example.com", "old@example.com"},
		{"What is my timezone now?", "Asia/Shanghai", "Europe/Rome"},
		{"What is my preferred language now?", "Chinese", "English"},
	} {
		items, err := s.SearchMemories(ctx, "attributes", tc.query, 20)
		text := evalContents(items)
		if err != nil || !strings.Contains(text, tc.want) || strings.Contains(text, tc.absent) {
			t.Errorf("%q: %s %v", tc.query, text, err)
		}
	}
}

func TestEvalStore_ExtendedDiaryRetainsReplayedHeading(t *testing.T) {
	r := setupL3FTSTestRepo(t, nil, 0)
	s := NewEvalMemoryStore(r.data, nil, loggateway.NewNoop())
	m := biz.EvalMessage{Role: "user", MessageID: "diary", Timestamp: "2024-03-20T09:00:00Z", Content: "2024-02-06：\nWe calibrated the spectrometer."}
	if _, err := s.AddMessages(context.Background(), "diary-extend", "s", []biz.EvalMessage{m}); err != nil {
		t.Fatal(err)
	}
	m.Content += "\nThe prism passed inspection."
	if _, err := s.AddMessages(context.Background(), "diary-extend", "s", []biz.EvalMessage{m}); err != nil {
		t.Fatal(err)
	}
	f := evalC3FindFact(t, evalC3AllFacts(t, r, "diary-extend"), "prism")
	if !strings.Contains(f["metadata_json"].(string), "2024-02-06") {
		t.Fatalf("replayed header did not date appended atom: %v", f["metadata_json"])
	}
}

func TestEvalStore_SameMessageIDKeepsDifferentSourceTimes(t *testing.T) {
	s := setupEvalStore(t).(*evalMemoryStore)
	ctx := context.Background()
	if _, err := s.AddMessages(ctx, "occurrence-times", "s", []biz.EvalMessage{
		{Role: "user", Content: "I inspected the prism.", MessageID: "reused", Timestamp: "2024-01-01T09:00:00Z"},
		{Role: "user", Content: "I calibrated the prism.", MessageID: "reused", Timestamp: "2024-02-01T09:00:00Z"},
	}); err != nil {
		t.Fatal(err)
	}
	s.episodes = nil
	items, err := s.SearchMemories(ctx, "occurrence-times", "Summarize all prism conversations", 20)
	if text := evalContents(items); err != nil || !strings.Contains(text, "2024-01-01") || !strings.Contains(text, "2024-02-01") {
		t.Fatalf("distinct occurrences collapsed: %s %v", text, err)
	}
}

func TestMaskEvalEvidence_CityAndCredentialAssignments(t *testing.T) {
	source := "I live at 123 Maple Street, Rome. My password is velvet-code77; 密码是松树口令。"
	got := MaskEvalEvidence(source, "Which city do I live in?")
	if strings.Contains(got, "123 Maple Street") || strings.Contains(got, "velvet-code77") || strings.Contains(got, "松树口令") || !strings.Contains(got, "Rome") {
		t.Fatalf("excess disclosure: %s", got)
	}
	if got = MaskEvalEvidence("I live at 123 Maple Street, Rome.", "What is my street address?"); !strings.Contains(got, "123 Maple Street") {
		t.Fatalf("explicit address evidence lost: %s", got)
	}
}

func TestEvalEvidenceDate_UnknownSourceDoesNotBecomeIngestionDay(t *testing.T) {
	raw := []byte(`{"id":"unknown","statement":"user: Yesterday I inspected the prism","created_at":"2026-10-22T12:00:00Z","valid_from":"2026-10-22T12:00:00Z","metadata_json":"{\"source_timestamp\":\"\"}"}`)
	env, ok := evalFactEnvelope(raw, 0)
	if !ok || env.day != "" || env.sourceTime != "" {
		t.Fatalf("unknown event time manufactured: %+v", env)
	}
}
