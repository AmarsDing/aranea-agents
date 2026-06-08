package session

import (
	"context"
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
		{3, 2, 0.20, 0.20},   // count >= threshold → full weight
		{2, 2, 0.20, 0.20},   // count == threshold → full weight
		{1, 2, 0.20, 0.10},   // count == 1 → half weight
		{0, 2, 0.20, 0},      // count == 0 → zero
		{5, 3, 0.10, 0.10},   // count >= threshold → full weight
		{1, 3, 0.10, 0.05},   // count == 1 → half weight
	}
	for _, tt := range tests {
		got := gradedScoreCount(tt.count, tt.threshold, tt.weight)
		if got != tt.want {
			t.Errorf("gradedScoreCount(%d, %d, %f) = %f, want %f",
				tt.count, tt.threshold, tt.weight, got, tt.want)
		}
	}
}

func TestShouldUseStructuredCompact(t *testing.T) {
	t.Run("high_ics_acceptable_ratio", func(t *testing.T) {
		cov := compactCoverage{
			HasIntent: true, HasState: true, DecisionCount: 3,
			FileCount: 2, FactCount: 3, HasPending: true,
		}
		// ICS = 1.0 >= 0.70, ratio = 5000/10000 = 0.50 <= 0.60
		got := shouldUseStructuredCompact(cov, 5000, 10000)
		if !got {
			t.Error("high ICS with good ratio should use structured compact")
		}
	})

	t.Run("low_ics", func(t *testing.T) {
		cov := compactCoverage{}
		got := shouldUseStructuredCompact(cov, 1000, 10000)
		if got {
			t.Error("low ICS should not use structured compact")
		}
	})

	t.Run("high_ics_ratio_too_high", func(t *testing.T) {
		cov := compactCoverage{
			HasIntent: true, HasState: true, DecisionCount: 3,
			FileCount: 2, FactCount: 3, HasPending: true,
		}
		// ICS = 1.0 >= 0.70, ratio = 7000/10000 = 0.70 > 0.60
		got := shouldUseStructuredCompact(cov, 7000, 10000)
		if got {
			t.Error("ratio > 0.60 should not use structured compact")
		}
	})

	t.Run("zero_original_tokens", func(t *testing.T) {
		cov := compactCoverage{
			HasIntent: true, HasState: true, DecisionCount: 3,
			FileCount: 2, FactCount: 3, HasPending: true,
		}
		// originalTokens <= 0 → return true
		got := shouldUseStructuredCompact(cov, 5000, 0)
		if !got {
			t.Error("zero originalTokens with high ICS should use structured compact")
		}
	})

	t.Run("boundary_ics", func(t *testing.T) {
		// Build coverage with ICS exactly 0.70
		// HasIntent=0.25, HasState=0.20, DecisionCount>=2=0.20, FileCount=1=0.075, total=0.725
		// Let's compute: 0.25+0.20+0.20+0.075 = 0.725
		cov := compactCoverage{
			HasIntent: true, HasState: true, DecisionCount: 2,
			FileCount: 1,
		}
		got := shouldUseStructuredCompact(cov, 5000, 10000)
		if !got {
			t.Error("ICS >= 0.70 with good ratio should use structured compact")
		}
	})
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
