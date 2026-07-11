package memory

import (
	"context"
	"errors"
	"testing"

	"aranea-agents/internal/biz"
)

// fakeManualCompressor implements biz.ManualCompressor for testing.
type fakeManualCompressor struct {
	result *biz.CompactResult
	err    error
	called bool
	lastSid string
	lastPreserve string
}

func (f *fakeManualCompressor) CompactSession(_ context.Context, sessionID, preserve string) (*biz.CompactResult, error) {
	f.called = true
	f.lastSid = sessionID
	f.lastPreserve = preserve
	return f.result, f.err
}

func TestCompactExecute_Success(t *testing.T) {
	mc := &fakeManualCompressor{
		result: &biz.CompactResult{
			Compacted:             true,
			FromTurn:              1,
			ToTurn:                5,
			EstimatedTokensBefore: 8000,
			EstimatedTokensAfter:  3000,
			CompressionLevel:      "llm_compact",
		},
	}
	ctx := WithManualCompressor(context.Background(), mc)
	ctx = WithCompactSessionID(ctx, "sess-123")

	out, err := compactExecute(ctx, CompactInput{PreserveInstruction: "keep code snippets"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("expected success=true")
	}
	if !out.Compacted {
		t.Error("expected compacted=true")
	}
	if out.BeforeTokens != 8000 {
		t.Errorf("expected before_tokens=8000, got %d", out.BeforeTokens)
	}
	if out.AfterTokens != 3000 {
		t.Errorf("expected after_tokens=3000, got %d", out.AfterTokens)
	}
	if out.CompressionLevel != "llm_compact" {
		t.Errorf("expected compression_level=llm_compact, got %s", out.CompressionLevel)
	}
	if out.FromTurn != 1 || out.ToTurn != 5 {
		t.Errorf("expected from_turn=1, to_turn=5, got from=%d to=%d", out.FromTurn, out.ToTurn)
	}
	if mc.lastSid != "sess-123" {
		t.Errorf("expected sessionID=sess-123, got %s", mc.lastSid)
	}
	if mc.lastPreserve != "keep code snippets" {
		t.Errorf("expected preserve='keep code snippets', got %s", mc.lastPreserve)
	}
}

func TestCompactExecute_NotCompacted(t *testing.T) {
	mc := &fakeManualCompressor{
		result: &biz.CompactResult{Compacted: false},
	}
	ctx := WithManualCompressor(context.Background(), mc)
	ctx = WithCompactSessionID(ctx, "sess-456")

	out, err := compactExecute(ctx, CompactInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("expected success=true even when not compacted")
	}
	if out.Compacted {
		t.Error("expected compacted=false")
	}
}

func TestCompactExecute_NilResult(t *testing.T) {
	mc := &fakeManualCompressor{result: nil}
	ctx := WithManualCompressor(context.Background(), mc)
	ctx = WithCompactSessionID(ctx, "sess-789")

	out, err := compactExecute(ctx, CompactInput{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !out.Success {
		t.Error("expected success=true for nil result")
	}
	if out.Compacted {
		t.Error("expected compacted=false for nil result")
	}
}

func TestCompactExecute_CompressorError(t *testing.T) {
	mc := &fakeManualCompressor{err: errors.New("compressor failed")}
	ctx := WithManualCompressor(context.Background(), mc)
	ctx = WithCompactSessionID(ctx, "sess-err")

	_, err := compactExecute(ctx, CompactInput{})
	if err == nil {
		t.Fatal("expected error from compressor")
	}
}

func TestCompactExecute_NoCompressor(t *testing.T) {
	ctx := WithCompactSessionID(context.Background(), "sess-nocomp")
	_, err := compactExecute(ctx, CompactInput{})
	if err == nil {
		t.Fatal("expected error when ManualCompressor is nil")
	}
}

func TestCompactExecute_NoSessionID(t *testing.T) {
	mc := &fakeManualCompressor{}
	ctx := WithManualCompressor(context.Background(), mc)
	// No session ID injected
	_, err := compactExecute(ctx, CompactInput{})
	if err == nil {
		t.Fatal("expected error when session_id is empty")
	}
	if mc.called {
		t.Error("compressor should not be called without session_id")
	}
}

func TestCompactExecute_EmptySessionID(t *testing.T) {
	mc := &fakeManualCompressor{}
	ctx := WithManualCompressor(context.Background(), mc)
	ctx = WithCompactSessionID(ctx, "   ")
	_, err := compactExecute(ctx, CompactInput{})
	if err == nil {
		t.Fatal("expected error for empty session_id")
	}
}

func TestCompactExecute_TrimsPreserveInstruction(t *testing.T) {
	mc := &fakeManualCompressor{result: &biz.CompactResult{Compacted: true}}
	ctx := WithManualCompressor(context.Background(), mc)
	ctx = WithCompactSessionID(ctx, "sess-trim")

	_, _ = compactExecute(ctx, CompactInput{PreserveInstruction: "  keep this  "})
	if mc.lastPreserve != "keep this" {
		t.Errorf("expected trimmed preserve='keep this', got %q", mc.lastPreserve)
	}
}

func TestNewCompactTool_Name(t *testing.T) {
	tool := NewCompactTool()
	d := tool.Declaration()
	if d == nil {
		t.Fatal("expected non-nil declaration")
	}
	if d.Name != "compact" {
		t.Errorf("expected tool name 'compact', got %q", d.Name)
	}
}

func TestManualCompressorFromCtx_Nil(t *testing.T) {
	if ManualCompressorFromCtx(context.Background()) != nil {
		t.Error("expected nil for empty context")
	}
}

func TestCompactSessionIDFromCtx_Nil(t *testing.T) {
	if CompactSessionIDFromCtx(context.Background()) != "" {
		t.Error("expected empty string for empty context")
	}
}
