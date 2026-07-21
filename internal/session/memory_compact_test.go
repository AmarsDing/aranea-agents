package session

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

type stubMemoryFactReader struct {
	facts []biz.MemoryFactEntry
	err   error
}

func (s *stubMemoryFactReader) ReadSessionMemoryFacts(_ context.Context, _ string) ([]biz.MemoryFactEntry, error) {
	return s.facts, s.err
}

func makeMsg(role string, turn int, content string) biz.ChatMessage {
	return biz.ChatMessage{Role: role, TurnNumber: turn, ContentMarkdown: content}
}

func TestTryMemoryCompact_emptyBody(t *testing.T) {
	r := tryMemoryCompact(context.Background(), nil, &stubMemoryFactReader{}, nil, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("empty body should not compact")
	}
}

func TestTryMemoryCompact_nilReader(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	r := tryMemoryCompact(context.Background(), body, nil, nil, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("nil reader should not compact")
	}
}

func TestTryMemoryCompact_readerError(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{err: errors.New("db down")}, nil, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("reader error should not compact")
	}
}

func TestTryMemoryCompact_noFacts(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{facts: nil}, nil, "s1", loggateway.NewNoop())
	if r.didCompact {
		t.Fatal("no facts should not compact")
	}
}

func TestTryMemoryCompact_withFacts(t *testing.T) {
	body := []biz.ChatMessage{
		makeMsg("user", 1, "hello"),
		makeMsg("assistant", 2, "hi"),
	}
	facts := []biz.MemoryFactEntry{
		{Statement: "user prefers Go", Scope: "static", Confidence: 0.9},
		{Statement: "project uses Kratos", Scope: "dynamic", Confidence: 0.8},
	}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{facts: facts}, nil, "s1", loggateway.NewNoop())
	if !r.didCompact {
		t.Fatal("facts present should compact")
	}
	if r.fromTurn != 1 || r.toTurn != 2 {
		t.Fatalf("turns = %d–%d, want 1–2", r.fromTurn, r.toTurn)
	}
	if r.summaryMarkdown == "" {
		t.Fatal("summary should not be empty")
	}
}

func TestTryMemoryCompact_factWithScope(t *testing.T) {
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}
	facts := []biz.MemoryFactEntry{
		{Statement: "fact A", Scope: "static", Confidence: 0.9},
		{Statement: "fact B", Scope: "", Confidence: 0.7},
	}
	r := tryMemoryCompact(context.Background(), body, &stubMemoryFactReader{facts: facts}, nil, "s1", loggateway.NewNoop())
	if !r.didCompact {
		t.Fatal("should compact")
	}
}

func TestMemoryCompactEnabled_default(t *testing.T) {
	if !memoryCompactEnabled(biz.Agent{}) {
		t.Fatal("default should enable memory compact")
	}
}

func TestMemoryCompactEnabled_withSettings(t *testing.T) {
	if !memoryCompactEnabled(biz.Agent{Settings: &biz.AgentRuntimeSettings{MemoryCompactEnabled: true}}) {
		t.Fatal("explicitly enabled should return true")
	}
}

func TestMemoryCompactEnabled_disabled(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{MemoryCompactEnabled: false}}
	if memoryCompactEnabled(ag) {
		t.Fatal("explicitly disabled should return false")
	}
}

func TestMemoryCompactEnabled_compressOff(t *testing.T) {
	ag := biz.Agent{Settings: &biz.AgentRuntimeSettings{L0SnapshotMode: "off", MemoryCompactEnabled: true}}
	if memoryCompactEnabled(ag) {
		t.Fatal("compress off should disable memory compact")
	}
}

func TestCompactCoverage_ICS(t *testing.T) {
	t.Run("all_dimensions", func(t *testing.T) {
		cov := compactCoverage{
			HasIntent:     true,
			HasState:      true,
			DecisionCount: 3,
			FileCount:     2,
			FactCount:     5,
			HasPending:    true,
		}
		got := cov.ICS()
		// 0.25 + 0.20 + 0.20 + 0.15 + 0.10 + 0.10 = 1.0
		if got != 1.0 {
			t.Errorf("ICS with all dimensions = %f, want 1.0", got)
		}
	})

	t.Run("none_set", func(t *testing.T) {
		cov := compactCoverage{}
		got := cov.ICS()
		if got != 0.0 {
			t.Errorf("ICS with no dimensions = %f, want 0.0", got)
		}
	})

	t.Run("partial", func(t *testing.T) {
		cov := compactCoverage{
			HasIntent:     true,
			DecisionCount: 1,
			FactCount:     2,
		}
		got := cov.ICS()
		// 0.25 + 0 + 0.10 + 0 + 0 + 0 = 0.35
		want := 0.25 + 0.10
		if got != want {
			t.Errorf("ICS partial = %f, want %f", got, want)
		}
	})

	t.Run("below_threshold_counts", func(t *testing.T) {
		cov := compactCoverage{
			DecisionCount: 1, // threshold=2, gets 0.5*weight = 0.10
			FileCount:     0, // 0
			FactCount:     1, // threshold=3, count=1, gets 0.5*weight = 0.05
		}
		got := cov.ICS()
		// 0 + 0 + 0.10 + 0 + 0.05 + 0 = 0.15
		want := 0.20*0.5 + 0.10*0.5
		if diff := got - want; diff < -1e-9 || diff > 1e-9 {
			t.Errorf("ICS below threshold = %f, want %f", got, want)
		}
	})
}

func TestGradedScore(t *testing.T) {
	tests := []struct {
		has    bool
		weight float64
		want   float64
	}{
		{true, 0.25, 0.25},
		{false, 0.25, 0},
		{true, 0, 0},
		{false, 0, 0},
	}
	for _, tt := range tests {
		got := gradedScore(tt.has, tt.weight)
		if got != tt.want {
			t.Errorf("gradedScore(%v, %f) = %f, want %f", tt.has, tt.weight, got, tt.want)
		}
	}
}

func TestGradedScoreCount(t *testing.T) {
	tests := []struct {
		count     int
		threshold int
		weight    float64
		want      float64
	}{
		{3, 2, 0.20, 0.20}, // count >= threshold → full weight
		{2, 2, 0.20, 0.20}, // count == threshold → full weight
		{1, 2, 0.20, 0.10}, // count == 1 → half weight
		{0, 2, 0.20, 0},    // count == 0 → zero
		{5, 3, 0.10, 0.10}, // count >= threshold → full weight
		{1, 3, 0.10, 0.05}, // count == 1 → half weight
	}
	for _, tt := range tests {
		got := gradedScoreCount(tt.count, tt.threshold, tt.weight)
		if got != tt.want {
			t.Errorf("gradedScoreCount(%d, %d, %f) = %f, want %f",
				tt.count, tt.threshold, tt.weight, got, tt.want)
		}
	}
}

func TestBuildCompactCoverage(t *testing.T) {
	t.Run("empty_facts", func(t *testing.T) {
		cov := buildCompactCoverage(nil, nil)
		if cov != (compactCoverage{}) {
			t.Errorf("empty facts = %#v, want zero struct", cov)
		}
	})

	t.Run("all_scopes", func(t *testing.T) {
		facts := []biz.MemoryFactEntry{
			{Statement: "intent fact", Scope: "intent"},
			{Statement: "state fact", Scope: "state"},
			{Statement: "decision 1", Scope: "decision"},
			{Statement: "decision 2", Scope: "decision"},
			{Statement: "file fact", Scope: "file"},
			{Statement: "pending fact", Scope: "pending"},
			{Statement: "other fact", Scope: "other"},
			{Statement: "another fact", Scope: "general"},
		}
		cov := buildCompactCoverage(facts, nil)
		if !cov.HasIntent {
			t.Error("expected HasIntent")
		}
		if !cov.HasState {
			t.Error("expected HasState")
		}
		if cov.DecisionCount != 2 {
			t.Errorf("DecisionCount = %d, want 2", cov.DecisionCount)
		}
		if cov.FileCount != 1 {
			t.Errorf("FileCount = %d, want 1", cov.FileCount)
		}
		if !cov.HasPending {
			t.Error("expected HasPending")
		}
		if cov.FactCount != 2 {
			t.Errorf("FactCount = %d, want 2 (unknown scopes)", cov.FactCount)
		}
	})

	t.Run("case_insensitive", func(t *testing.T) {
		facts := []biz.MemoryFactEntry{
			{Statement: "intent", Scope: "Intent"},
			{Statement: "state", Scope: "STATE"},
			{Statement: "decision", Scope: " Decision "},
		}
		cov := buildCompactCoverage(facts, nil)
		if !cov.HasIntent || !cov.HasState || cov.DecisionCount != 1 {
			t.Errorf("case insensitive mismatch: %#v", cov)
		}
	})
}

func TestMapFieldKindToScope(t *testing.T) {
	tests := []struct {
		fieldPath string
		want      string
	}{
		{"user_intent", "intent"},
		{"goal_description", "intent"},
		{"current_state", "state"},
		{"task_status", "state"},
		{"key_decision", "decision"},
		{"choice_made", "decision"},
		{"file_path", "file"},
		{"output_path", "file"},
		{"pending_items", "pending"},
		{"todo_list", "pending"},
		{"task_name", "pending"},
		{"random_field", "fact"},
		{"", "fact"},
	}
	for _, tt := range tests {
		got := mapFieldKindToScope(tt.fieldPath)
		if got != tt.want {
			t.Errorf("mapFieldKindToScope(%q) = %q, want %q", tt.fieldPath, got, tt.want)
		}
	}
}

func TestTruncateFieldText(t *testing.T) {
	t.Run("short_text", func(t *testing.T) {
		got := truncateFieldText("hello", 10)
		if got != "hello" {
			t.Errorf("short text = %q, want %q", got, "hello")
		}
	})

	t.Run("exact_length", func(t *testing.T) {
		got := truncateFieldText("hello", 5)
		if got != "hello" {
			t.Errorf("exact length = %q, want %q", got, "hello")
		}
	})

	t.Run("long_text", func(t *testing.T) {
		text := "abcdefghij"
		got := truncateFieldText(text, 5)
		want := "abcde…"
		if got != want {
			t.Errorf("truncated = %q, want %q", got, want)
		}
	})

	t.Run("empty_text", func(t *testing.T) {
		got := truncateFieldText("", 10)
		if got != "" {
			t.Errorf("empty text = %q, want empty", got)
		}
	})
}

func TestDecodeMap(t *testing.T) {
	t.Run("valid_json", func(t *testing.T) {
		raw := []byte(`{"id":"t1","task_title":"my task","status":"active"}`)
		m := decodeMap(raw)
		if m == nil {
			t.Fatal("expected non-nil map")
		}
		if m["id"] != "t1" {
			t.Errorf("id = %v, want t1", m["id"])
		}
		if m["task_title"] != "my task" {
			t.Errorf("task_title = %v, want 'my task'", m["task_title"])
		}
	})

	t.Run("empty_input", func(t *testing.T) {
		m := decodeMap(nil)
		if m != nil {
			t.Errorf("nil input = %v, want nil", m)
		}
		m = decodeMap([]byte{})
		if m != nil {
			t.Errorf("empty input = %v, want nil", m)
		}
	})

	t.Run("invalid_json", func(t *testing.T) {
		m := decodeMap([]byte(`not json`))
		if m != nil {
			t.Errorf("invalid json = %v, want nil", m)
		}
	})

	t.Run("non_object_json", func(t *testing.T) {
		m := decodeMap([]byte(`[1,2,3]`))
		if m != nil {
			t.Errorf("non-object json = %v, want nil", m)
		}
	})
}

// --- stubL1AdminReader ---

type stubL1AdminReader struct {
	taskRows  [][]byte
	taskErr   error
	fieldRows map[string]struct {
		rows [][]byte
		err  error
	}
	// stubs for unused interface methods
	getTaskRow  []byte
	getTaskErr  error
	getFieldRow []byte
	getFieldErr error
}

func (s *stubL1AdminReader) ListL1TaskRows(_ context.Context, _ string, _ string, _ string, _ string) ([][]byte, error) {
	return s.taskRows, s.taskErr
}

func (s *stubL1AdminReader) ListL1FieldRows(_ context.Context, taskID string, _ bool, _ ...string) ([][]byte, error) {
	if s.fieldRows == nil {
		return nil, nil
	}
	entry, ok := s.fieldRows[taskID]
	if !ok {
		return nil, nil
	}
	return entry.rows, entry.err
}

func (s *stubL1AdminReader) GetL1TaskRow(_ context.Context, _ string, _ string) ([]byte, error) {
	return s.getTaskRow, s.getTaskErr
}

func (s *stubL1AdminReader) GetL1FieldRow(_ context.Context, _ string, _ string) ([]byte, error) {
	return s.getFieldRow, s.getFieldErr
}

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("mustMarshal: %v", err)
	}
	return b
}

// --- TestReadL1Facts ---

func TestReadL1Facts(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()

	t.Run("ListL1TaskRows_error", func(t *testing.T) {
		r := &stubL1AdminReader{taskErr: errors.New("db down")}
		got := readL1Facts(ctx, r, "s1", lg)
		if got != nil {
			t.Fatalf("expected nil on task list error, got %v", got)
		}
	})

	t.Run("ListL1TaskRows_empty", func(t *testing.T) {
		r := &stubL1AdminReader{taskRows: nil}
		got := readL1Facts(ctx, r, "s1", lg)
		if got != nil {
			t.Fatalf("expected nil on empty task rows, got %v", got)
		}
	})

	t.Run("task_no_id_skipped", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"task_title": "no id task", "status": "active"}),
		}
		r := &stubL1AdminReader{taskRows: taskRows}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 0 {
			t.Fatalf("task without id should be skipped, got %d facts", len(got))
		}
	})

	t.Run("task_empty_title_no_fact", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "", "status": "active"}),
		}
		r := &stubL1AdminReader{taskRows: taskRows}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 0 {
			t.Fatalf("task with empty title should add no fact, got %d facts", len(got))
		}
	})

	t.Run("task_active_status_intent", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "my task", "status": "active"}),
		}
		r := &stubL1AdminReader{taskRows: taskRows}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 1 {
			t.Fatalf("expected 1 fact, got %d", len(got))
		}
		if got[0].Scope != "intent" {
			t.Errorf("active task scope = %q, want %q", got[0].Scope, "intent")
		}
		if got[0].Statement != "my task" {
			t.Errorf("statement = %q, want %q", got[0].Statement, "my task")
		}
	})

	t.Run("task_non_active_status_pending", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "pending task", "status": "completed"}),
		}
		r := &stubL1AdminReader{taskRows: taskRows}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 1 {
			t.Fatalf("expected 1 fact, got %d", len(got))
		}
		if got[0].Scope != "pending" {
			t.Errorf("non-active task scope = %q, want %q", got[0].Scope, "pending")
		}
	})

	t.Run("ListL1FieldRows_error_skipped", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "my task", "status": "active"}),
		}
		r := &stubL1AdminReader{
			taskRows: taskRows,
			fieldRows: map[string]struct {
				rows [][]byte
				err  error
			}{
				"t1": {err: errors.New("field db down")},
			},
		}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 1 {
			t.Fatalf("expected 1 fact (task only, fields skipped), got %d", len(got))
		}
		if got[0].Statement != "my task" {
			t.Errorf("statement = %q, want %q", got[0].Statement, "my task")
		}
	})

	t.Run("field_empty_path_skipped", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "my task", "status": "active"}),
		}
		fieldRows := [][]byte{
			mustMarshal(t, map[string]any{"field_path": "", "value_text": "val"}),
		}
		r := &stubL1AdminReader{
			taskRows: taskRows,
			fieldRows: map[string]struct {
				rows [][]byte
				err  error
			}{
				"t1": {rows: fieldRows},
			},
		}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 1 {
			t.Fatalf("expected 1 fact (task only, empty-path field skipped), got %d", len(got))
		}
	})

	t.Run("field_with_value_text_uses_mapFieldKindToScope", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "", "status": "active"}),
		}
		fieldRows := [][]byte{
			mustMarshal(t, map[string]any{"field_path": "user_intent", "value_text": "build API"}),
		}
		r := &stubL1AdminReader{
			taskRows: taskRows,
			fieldRows: map[string]struct {
				rows [][]byte
				err  error
			}{
				"t1": {rows: fieldRows},
			},
		}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 1 {
			t.Fatalf("expected 1 fact, got %d", len(got))
		}
		if got[0].Scope != "intent" {
			t.Errorf("scope = %q, want %q", got[0].Scope, "intent")
		}
		want := "user_intent: build API"
		if got[0].Statement != want {
			t.Errorf("statement = %q, want %q", got[0].Statement, want)
		}
	})

	t.Run("field_without_value_text_uses_field_path", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "", "status": "active"}),
		}
		fieldRows := [][]byte{
			mustMarshal(t, map[string]any{"field_path": "random_field", "value_text": ""}),
		}
		r := &stubL1AdminReader{
			taskRows: taskRows,
			fieldRows: map[string]struct {
				rows [][]byte
				err  error
			}{
				"t1": {rows: fieldRows},
			},
		}
		got := readL1Facts(ctx, r, "s1", lg)
		if len(got) != 1 {
			t.Fatalf("expected 1 fact, got %d", len(got))
		}
		if got[0].Statement != "random_field" {
			t.Errorf("statement = %q, want %q", got[0].Statement, "random_field")
		}
	})
}

// --- TestTryMemoryCompact_withL1Reader ---

func TestTryMemoryCompact_withL1Reader(t *testing.T) {
	ctx := context.Background()
	lg := loggateway.NewNoop()
	body := []biz.ChatMessage{makeMsg("user", 1, "hello")}

	t.Run("both_readers_nil", func(t *testing.T) {
		r := tryMemoryCompact(ctx, body, nil, nil, "s1", lg)
		if r.didCompact {
			t.Fatal("both readers nil should not compact")
		}
	})

	t.Run("only_l1Reader_provides_facts", func(t *testing.T) {
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "L1 task", "status": "active"}),
		}
		l1 := &stubL1AdminReader{taskRows: taskRows}
		r := tryMemoryCompact(ctx, body, nil, l1, "s1", lg)
		if !r.didCompact {
			t.Fatal("l1Reader facts should compact")
		}
		if r.summaryMarkdown == "" {
			t.Fatal("summary should not be empty")
		}
	})

	t.Run("both_readers_provide_facts", func(t *testing.T) {
		facts := []biz.MemoryFactEntry{
			{Statement: "L3 fact", Scope: "static", Confidence: 0.9},
		}
		reader := &stubMemoryFactReader{facts: facts}
		taskRows := [][]byte{
			mustMarshal(t, map[string]any{"id": "t1", "task_title": "L1 task", "status": "active"}),
		}
		l1 := &stubL1AdminReader{taskRows: taskRows}
		r := tryMemoryCompact(ctx, body, reader, l1, "s1", lg)
		if !r.didCompact {
			t.Fatal("both readers should compact")
		}
		if r.summaryMarkdown == "" {
			t.Fatal("summary should not be empty")
		}
		// Verify both facts appear in the summary
		if !containsStr(r.summaryMarkdown, "L3 fact") {
			t.Error("summary should contain L3 fact")
		}
		if !containsStr(r.summaryMarkdown, "L1 task") {
			t.Error("summary should contain L1 task")
		}
	})
}

func TestMapFieldKindValueToScope(t *testing.T) {
	tests := []struct {
		kind string
		want string
	}{
		{"decision", "decision"},
		{"artifact", "file"},
		{"progress", "state"},
		{"constraint", "intent"},
		{"string", ""},
		{"", ""},
		{"unknown_kind", ""},
	}
	for _, tt := range tests {
		t.Run(tt.kind, func(t *testing.T) {
			if got := mapFieldKindValueToScope(tt.kind); got != tt.want {
				t.Errorf("mapFieldKindValueToScope(%q) = %q, want %q", tt.kind, got, tt.want)
			}
		})
	}
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || containsSubstr(s, sub))
}

func containsSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
