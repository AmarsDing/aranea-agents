package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"aranea-agents/internal/biz"
	"aranea-agents/pkg/loggateway"

	trpcmodel "trpc.group/trpc-go/trpc-agent-go/model"
)

// fakeCompressorModel implements trpcmodel.Model for context-compressor tests.
// It records the last request so tests can assert on prompt construction.
type fakeCompressorModel struct {
	response *trpcmodel.Response
	err      error
	called   bool
	lastReq  *trpcmodel.Request
}

func (m *fakeCompressorModel) GenerateContent(_ context.Context, req *trpcmodel.Request) (<-chan *trpcmodel.Response, error) {
	m.called = true
	m.lastReq = req
	if m.err != nil {
		return nil, m.err
	}
	ch := make(chan *trpcmodel.Response, 1)
	if m.response != nil {
		ch <- m.response
	}
	close(ch)
	return ch, nil
}

func (m *fakeCompressorModel) Info() trpcmodel.Info {
	return trpcmodel.Info{Name: "fake-compressor-model"}
}

func buildCompressorResponse(content string) *trpcmodel.Response {
	return &trpcmodel.Response{
		Choices: []trpcmodel.Choice{
			{Message: trpcmodel.Message{Role: trpcmodel.RoleAssistant, Content: content}},
		},
	}
}

func compressorMessages(role, content string) []biz.ConsolidateMessage {
	return []biz.ConsolidateMessage{{Role: role, Content: content}}
}

// --- ShouldCompress ---

// TestShouldCompress_BelowThreshold: ratio below 0.80 returns false.
func TestShouldCompress_BelowThreshold(t *testing.T) {
	c := NewLLMContextCompressor(nil, loggateway.NewNoop())
	if c.ShouldCompress(0.79) {
		t.Fatalf("expected ShouldCompress(0.79)=false, got true")
	}
	if c.ShouldCompress(0.0) {
		t.Fatalf("expected ShouldCompress(0.0)=false, got true")
	}
}

// TestShouldCompress_AtOrAboveThreshold: ratio >= 0.80 returns true.
func TestShouldCompress_AtOrAboveThreshold(t *testing.T) {
	c := NewLLMContextCompressor(nil, loggateway.NewNoop())
	if !c.ShouldCompress(0.80) {
		t.Fatalf("expected ShouldCompress(0.80)=true, got false")
	}
	if !c.ShouldCompress(0.95) {
		t.Fatalf("expected ShouldCompress(0.95)=true, got false")
	}
}

// --- Compress ---

// TestCompress_Success: LLM returns a summary; verify prompt construction
// and result metrics.
func TestCompress_Success(t *testing.T) {
	model := &fakeCompressorModel{response: buildCompressorResponse("Summary of the conversation")}
	c := NewLLMContextCompressor(model, loggateway.NewNoop())

	msgs := []biz.ConsolidateMessage{
		{Role: "user", Content: "Hello, I need help with Go"},
		{Role: "assistant", Content: "Sure, what do you need?"},
	}
	out, err := c.Compress(context.Background(), "", msgs)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if !model.called {
		t.Fatalf("expected LLM to be called")
	}
	if out.Summary != "Summary of the conversation" {
		t.Fatalf("unexpected summary: %q", out.Summary)
	}
	if out.EvictedCount != 2 {
		t.Fatalf("expected EvictedCount=2, got %d", out.EvictedCount)
	}
	if out.AfterChars != len("Summary of the conversation") {
		t.Fatalf("expected AfterChars=%d, got %d", len("Summary of the conversation"), out.AfterChars)
	}
	if out.BeforeChars <= 0 {
		t.Fatalf("expected BeforeChars>0, got %d", out.BeforeChars)
	}
	// Verify prompt contains the evicted message content.
	if model.lastReq == nil || len(model.lastReq.Messages) < 2 {
		t.Fatalf("expected request with at least 2 messages, got %+v", model.lastReq)
	}
	combined := ""
	for _, m := range model.lastReq.Messages {
		combined += m.Content
	}
	if !strings.Contains(combined, "Hello, I need help with Go") {
		t.Fatalf("expected prompt to contain evicted message content, got: %s", combined)
	}
}

// TestCompress_WithExistingSummary: recursive summary merges existing summary
// with new messages — verify both appear in the prompt.
func TestCompress_WithExistingSummary(t *testing.T) {
	model := &fakeCompressorModel{response: buildCompressorResponse("Merged summary")}
	c := NewLLMContextCompressor(model, loggateway.NewNoop())

	existing := "Previous summary: user discussed Python"
	msgs := compressorMessages("user", "Now I want to talk about Go")
	out, err := c.Compress(context.Background(), existing, msgs)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if out.Summary != "Merged summary" {
		t.Fatalf("unexpected summary: %q", out.Summary)
	}
	// Prompt must include both the existing summary and the new message.
	combined := ""
	for _, m := range model.lastReq.Messages {
		combined += m.Content
	}
	if !strings.Contains(combined, "Previous summary: user discussed Python") {
		t.Fatalf("expected prompt to contain existing summary, got: %s", combined)
	}
	if !strings.Contains(combined, "Now I want to talk about Go") {
		t.Fatalf("expected prompt to contain new message, got: %s", combined)
	}
}

// TestCompress_NoMessages: empty evictedMessages is a no-op (no LLM call).
func TestCompress_NoMessages(t *testing.T) {
	model := &fakeCompressorModel{response: buildCompressorResponse("should not be used")}
	c := NewLLMContextCompressor(model, loggateway.NewNoop())

	out, err := c.Compress(context.Background(), "", nil)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if model.called {
		t.Fatalf("expected LLM NOT to be called for empty messages")
	}
	if out.Summary != "" {
		t.Fatalf("expected empty summary, got %q", out.Summary)
	}
	if out.EvictedCount != 0 {
		t.Fatalf("expected EvictedCount=0, got %d", out.EvictedCount)
	}
}

// TestCompress_LLMFailure: LLM error is propagated.
func TestCompress_LLMFailure(t *testing.T) {
	model := &fakeCompressorModel{err: errors.New("LLM API down")}
	c := NewLLMContextCompressor(model, loggateway.NewNoop())

	msgs := compressorMessages("user", "hello")
	_, err := c.Compress(context.Background(), "", msgs)
	if err == nil {
		t.Fatalf("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "LLM API down") {
		t.Fatalf("expected error to contain 'LLM API down', got: %v", err)
	}
}

// TestCompress_EmptyLLMResponse: empty LLM response returns empty summary but
// still populates metrics.
func TestCompress_EmptyLLMResponse(t *testing.T) {
	model := &fakeCompressorModel{response: buildCompressorResponse("   ")}
	c := NewLLMContextCompressor(model, loggateway.NewNoop())

	msgs := compressorMessages("user", "hello")
	out, err := c.Compress(context.Background(), "", msgs)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if out.Summary != "" {
		t.Fatalf("expected empty summary after trim, got %q", out.Summary)
	}
	if out.EvictedCount != 1 {
		t.Fatalf("expected EvictedCount=1, got %d", out.EvictedCount)
	}
}

// TestCompress_NilLLM: nil LLM returns an error (implementation not wired).
func TestCompress_NilLLM(t *testing.T) {
	c := NewLLMContextCompressor(nil, loggateway.NewNoop())
	msgs := compressorMessages("user", "hello")
	_, err := c.Compress(context.Background(), "", msgs)
	if err == nil {
		t.Fatalf("expected error for nil LLM, got nil")
	}
}
