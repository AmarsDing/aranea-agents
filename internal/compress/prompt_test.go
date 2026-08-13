package compress

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"
)

func TestPromptVersion(t *testing.T) {
	if PromptVersion != "v3" {
		t.Fatalf("got %q want %q", PromptVersion, "v3")
	}
}

// F5: Section 6（逐字用户消息）必须有上限——无上限时长会话中 Section 6 自身
// 就会撑爆摘要。v3 起仅保留最近 30 条逐字，更早的压缩为主题列表。
func TestDefaultSystemPrompt_Section6Capped(t *testing.T) {
	if !strings.Contains(DefaultSystemPrompt, "30") {
		t.Fatal("Section 6 cap (30 most recent verbatim) missing from system prompt")
	}
	if strings.Contains(DefaultSystemPrompt, "List EVERY user message verbatim. Do NOT summarize or omit any.") {
		t.Fatal("uncapped Section 6 wording still present")
	}
}

func TestDefaultSystemPrompt_NonEmpty(t *testing.T) {
	if strings.TrimSpace(DefaultSystemPrompt) == "" {
		t.Fatal("DefaultSystemPrompt is empty")
	}
}

func TestMemoryExtractPromptVersions(t *testing.T) {
	if MemoryExtractPromptVersion != "v1" {
		t.Fatalf("MemoryExtractPromptVersion: got %q want %q", MemoryExtractPromptVersion, "v1")
	}
	if MemoryExtractPromptV2Version != "v2" {
		t.Fatalf("MemoryExtractPromptV2Version: got %q want %q", MemoryExtractPromptV2Version, "v2")
	}
}

func TestExtractMemoryFactsFunctionName(t *testing.T) {
	if ExtractMemoryFactsFunctionName != "extract_memory_facts" {
		t.Fatalf("got %q want %q", ExtractMemoryFactsFunctionName, "extract_memory_facts")
	}
}

func TestExtractMemoryFactsFunctionSchema_Name(t *testing.T) {
	name, ok := ExtractMemoryFactsFunctionSchema["name"].(string)
	if !ok {
		t.Fatal("schema['name'] is not a string")
	}
	if name != "extract_memory_facts" {
		t.Fatalf("got %q want %q", name, "extract_memory_facts")
	}
}

func TestExtractMemoryFactsFunctionSchema_HasParameters(t *testing.T) {
	params, exists := ExtractMemoryFactsFunctionSchema["parameters"]
	if !exists || params == nil {
		t.Fatal("schema['parameters'] is missing or nil")
	}
}

func TestErrEmptyMemoryTranscript(t *testing.T) {
	if ErrEmptyMemoryTranscript == nil {
		t.Fatal("ErrEmptyMemoryTranscript is nil")
	}
	if ErrEmptyMemoryTranscript.Error() == "" {
		t.Fatal("ErrEmptyMemoryTranscript has empty message")
	}
}

func TestCompressErrors(t *testing.T) {
	errs := []error{ErrCatalogRequired, ErrHTTPClientRequired, ErrProviderModelRequired, ErrEmptyTranscript}
	for i, err := range errs {
		if err == nil {
			t.Fatalf("error at index %d is nil", i)
		}
	}
}

func TestStripJSONFence_NoFence(t *testing.T) {
	got := stripJSONFence("hello")
	if got != "hello" {
		t.Fatalf("got %q want %q", got, "hello")
	}
}

func TestStripJSONFence_JSONFence(t *testing.T) {
	got := stripJSONFence("```json\n{\"a\":1}\n```")
	if got != `{"a":1}` {
		t.Fatalf("got %q want %q", got, `{"a":1}`)
	}
}

func TestStripJSONFence_PlainFence(t *testing.T) {
	got := stripJSONFence("```\n{\"a\":1}\n```")
	if got != `{"a":1}` {
		t.Fatalf("got %q want %q", got, `{"a":1}`)
	}
}

func TestStripJSONFence_UpperCaseFence(t *testing.T) {
	got := stripJSONFence("```JSON\n{\"a\":1}\n```")
	if got != `{"a":1}` {
		t.Fatalf("got %q want %q", got, `{"a":1}`)
	}
}

func TestParseMemoryExtractFunctionCallArgs_Empty(t *testing.T) {
	facts, reason, err := ParseMemoryExtractFunctionCallArgs("")
	if err != nil {
		t.Fatal(err)
	}
	if facts != nil {
		t.Fatalf("facts: got %v want nil", facts)
	}
	if reason != "" {
		t.Fatalf("reason: got %q want %q", reason, "")
	}
}

func TestParseMemoryExtractFunctionCallArgs_Valid(t *testing.T) {
	raw := `{"facts":[{"statement":"User prefers dark mode","subject_type":"preference","scope":"user","confidence":0.9}]}`
	facts, _, err := ParseMemoryExtractFunctionCallArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("got %d facts want 1", len(facts))
	}
	if facts[0].Statement != "User prefers dark mode" {
		t.Fatalf("statement: got %q want %q", facts[0].Statement, "User prefers dark mode")
	}
}

func TestParseMemoryExtractFunctionCallArgs_SkipsEmptyStatement(t *testing.T) {
	raw := `{"facts":[{"statement":"","subject_type":"preference","confidence":0.9},{"statement":"ok","subject_type":"preference","confidence":0.8}]}`
	facts, _, err := ParseMemoryExtractFunctionCallArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 1 {
		t.Fatalf("got %d facts want 1", len(facts))
	}
	if facts[0].Statement != "ok" {
		t.Fatalf("statement: got %q want %q", facts[0].Statement, "ok")
	}
}

func TestParseMemoryExtractFunctionCallArgs_DefaultsConfidence(t *testing.T) {
	raw := `{"facts":[{"statement":"test fact","subject_type":"preference","confidence":0}]}`
	facts, _, err := ParseMemoryExtractFunctionCallArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if facts[0].Confidence != 0.7 {
		t.Fatalf("confidence: got %v want 0.7", facts[0].Confidence)
	}
}

func TestParseMemoryExtractFunctionCallArgs_DefaultsSubjectType(t *testing.T) {
	raw := `{"facts":[{"statement":"test fact","subject_type":"","confidence":0.8}]}`
	facts, _, err := ParseMemoryExtractFunctionCallArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if facts[0].SubjectType != "other" {
		t.Fatalf("subject_type: got %q want %q", facts[0].SubjectType, "other")
	}
}

func TestParseMemoryExtractFunctionCallArgs_DefaultsScope(t *testing.T) {
	raw := `{"facts":[{"statement":"test fact","subject_type":"preference","confidence":0.8,"scope":""}]}`
	facts, _, err := ParseMemoryExtractFunctionCallArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if facts[0].Scope != "user" {
		t.Fatalf("scope: got %q want %q", facts[0].Scope, "user")
	}
}

func TestParseMemoryExtractFunctionCallArgs_NoFactsReason(t *testing.T) {
	raw := `{"facts":[],"no_facts_reason":"only_greetings"}`
	facts, reason, err := ParseMemoryExtractFunctionCallArgs(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(facts) != 0 {
		t.Fatalf("got %d facts want 0", len(facts))
	}
	if reason != "only_greetings" {
		t.Fatalf("reason: got %q want %q", reason, "only_greetings")
	}
}

func TestBuildMemoryExtractTranscript_Empty(t *testing.T) {
	got := BuildMemoryExtractTranscript(nil)
	if got != "" {
		t.Fatalf("got %q want %q", got, "")
	}
}

func TestBuildMemoryExtractTranscript_SkipsEmptyContent(t *testing.T) {
	got := BuildMemoryExtractTranscript([]struct{ Role, Content string }{
		{Role: "user", Content: ""},
		{Role: "assistant", Content: "  "},
		{Role: "user", Content: "hello"},
	})
	if got != "USER: hello" {
		t.Fatalf("got %q want %q", got, "USER: hello")
	}
}

func TestCompress_NilService(t *testing.T) {
	var s *LLMService
	_, err := s.Compress(context.Background(), Request{Transcript: "x", Provider: "openai", Model: "gpt-4o-mini"})
	if err != ErrCatalogRequired {
		t.Fatalf("got %v want ErrCatalogRequired", err)
	}
}

func TestCompress_EmptyProviderModel(t *testing.T) {
	s := NewLLMService(&biz.LlmProviderModelUsecase{}, &http.Client{}, loggateway.NewNoop())
	_, err := s.Compress(context.Background(), Request{Transcript: "x"})
	if err != ErrProviderModelRequired {
		t.Fatalf("got %v want ErrProviderModelRequired", err)
	}
}

func TestCompress_EmptyTranscript(t *testing.T) {
	s := NewLLMService(&biz.LlmProviderModelUsecase{}, &http.Client{}, loggateway.NewNoop())
	_, err := s.Compress(context.Background(), Request{Provider: "openai", Model: "gpt-4o-mini"})
	if err != ErrEmptyTranscript {
		t.Fatalf("got %v want ErrEmptyTranscript", err)
	}
}
