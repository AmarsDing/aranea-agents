package knowledge

import (
	"testing"

	"aranea-agents/internal/biz"
)

func TestRetrievalEvaluator_NilLLM(t *testing.T) {
	e := NewRetrievalEvaluator(nil, nil, nil)
	assessment, err := e.Evaluate(nil, "test query", []biz.KnowledgeChunk{
		{ID: "1", Content: "some content", Score: 0.9},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !assessment.Sufficient {
		t.Error("nil LLM should default to sufficient=true")
	}
	if assessment.Confidence != 1.0 {
		t.Errorf("nil LLM confidence = %v, want 1.0", assessment.Confidence)
	}
}

func TestRetrievalEvaluator_EmptyChunks(t *testing.T) {
	e := &RetrievalEvaluator{}
	assessment, err := e.Evaluate(nil, "test query", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if assessment.Sufficient {
		t.Error("empty chunks should be insufficient")
	}
	if assessment.SupplementQuery != "test query" {
		t.Errorf("supplement query = %q, want %q", assessment.SupplementQuery, "test query")
	}
}

func TestParseAssessment(t *testing.T) {
	tests := []struct {
		name       string
		raw        string
		sufficient bool
		confidence float32
		supplement string
	}{
		{
			name:       "valid sufficient",
			raw:        `{"sufficient": true, "confidence": 0.9, "supplement_query": ""}`,
			sufficient: true,
			confidence: 0.9,
			supplement: "",
		},
		{
			name:       "valid insufficient",
			raw:        `{"sufficient": false, "confidence": 0.3, "supplement_query": "补充查询"}`,
			sufficient: false,
			confidence: 0.3,
			supplement: "补充查询",
		},
		{
			name:       "with code fence",
			raw:        "```json\n{\"sufficient\": true, \"confidence\": 0.8, \"supplement_query\": \"\"}\n```",
			sufficient: true,
			confidence: 0.8,
			supplement: "",
		},
		{
			name:       "invalid json defaults sufficient",
			raw:        "not json at all",
			sufficient: true,
			confidence: 0.5,
			supplement: "",
		},
		{
			name:       "partial json",
			raw:        `some prefix {"sufficient": false, "confidence": 0.6, "supplement_query": "test"} suffix`,
			sufficient: false,
			confidence: 0.6,
			supplement: "test",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parseAssessment(tt.raw)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got.Sufficient != tt.sufficient {
				t.Errorf("sufficient = %v, want %v", got.Sufficient, tt.sufficient)
			}
			if got.Confidence != tt.confidence {
				t.Errorf("confidence = %v, want %v", got.Confidence, tt.confidence)
			}
			if got.SupplementQuery != tt.supplement {
				t.Errorf("supplement_query = %q, want %q", got.SupplementQuery, tt.supplement)
			}
		})
	}
}

func TestBuildChunksSummary(t *testing.T) {
	chunks := []biz.KnowledgeChunk{
		{ID: "1", Content: "短内容"},
		{ID: "2", Content: string(make([]byte, 500))},
	}
	summary := buildChunksSummary(chunks, 100)
	if summary == "" {
		t.Error("summary should not be empty")
	}
}

func TestTruncateString(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		maxRunes int
		want     int
		ellipsis bool
	}{
		{"short", "hello", 10, 5, false},
		{"exact", "hello", 5, 5, false},
		{"truncate", "hello world", 5, 8, true},
		{"chinese", "你好世界测试", 3, 6, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := truncateString(tt.input, tt.maxRunes)
			runes := []rune(got)
			if tt.ellipsis && !endsWithEllipsis(got) {
				t.Errorf("expected ellipsis in %q", got)
			}
			if !tt.ellipsis && len(runes) != tt.want {
				t.Errorf("len(runes) = %d, want %d", len(runes), tt.want)
			}
		})
	}
}

func endsWithEllipsis(s string) bool {
	return len(s) >= 3 && s[len(s)-3:] == "..."
}

func TestParseJSONLoose(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"valid", `{"key": "value"}`, false},
		{"with whitespace", `  {"key": "value"}  `, false},
		{"with prefix", `result: {"key": "value"}`, false},
		{"empty", "", true},
		{"no braces", "just text", true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var m map[string]string
			err := parseJSONLoose(tt.input, &m)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseJSONLoose(%q) error = %v, wantErr %v", tt.input, err, tt.wantErr)
			}
		})
	}
}
