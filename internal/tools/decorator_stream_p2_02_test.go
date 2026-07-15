package tools

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// --- P2-02 Streaming Guard Tests ---
//
// These tests verify that streamableToolDecorator.StreamableCall enforces:
//   - Byte budget (StreamBudget)
//   - Deadline (StreamTimeout)
//   - Context cancellation
//   - Clean pass-through for normal streams
//   - Error propagation from the inner tool

// slowStreamTool is a test double that produces chunks at a controlled rate
// and respects context cancellation. It implements both CallableTool and
// StreamableTool.
type slowStreamTool struct {
	name         string
	chunkContent string
	interval     time.Duration
	chunkCount   int
}

func (s *slowStreamTool) Declaration() *trpctool.Declaration {
	return &trpctool.Declaration{Name: s.name}
}

func (s *slowStreamTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	return "ok", nil
}

func (s *slowStreamTool) StreamableCall(ctx context.Context, jsonArgs []byte) (*trpctool.StreamReader, error) {
	stream := trpctool.NewStream(10)
	go func() {
		defer stream.Writer.Close()
		for i := 0; i < s.chunkCount; i++ {
			select {
			case <-ctx.Done():
				return
			case <-time.After(s.interval):
			}
			chunk := trpctool.StreamChunk{Content: s.chunkContent}
			if closed := stream.Writer.Send(chunk, nil); closed {
				return
			}
		}
	}()
	return stream.Reader, nil
}

// drainStream reads all chunks from a StreamReader until EOF or error.
// Returns the collected chunk contents and the terminal error (nil for
// clean EOF).
func drainStream(reader *trpctool.StreamReader) ([]any, error) {
	var chunks []any
	for {
		chunk, err := reader.Recv()
		if err != nil {
			if err == io.EOF {
				return chunks, nil
			}
			return chunks, err
		}
		chunks = append(chunks, chunk.Content)
	}
}

// TestStreamableCall_Passthrough verifies that a normal stream passes chunks
// through the proxy unchanged, ending with clean EOF.
func TestStreamableCall_Passthrough(t *testing.T) {
	inner := &slowStreamTool{
		name:         "passthrough_tool",
		chunkContent: "chunk",
		interval:    5 * time.Millisecond,
		chunkCount:   3,
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		StreamTimeout: 5 * time.Second,
		StreamBudget:  DefaultStreamBudget,
		Logger:        loggateway.NewNoop(),
	})
	st := d.(trpctool.StreamableTool)
	reader, err := st.StreamableCall(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	defer reader.Close()

	chunks, streamErr := drainStream(reader)
	if streamErr != nil {
		t.Errorf("expected clean EOF, got error: %v", streamErr)
	}
	if len(chunks) != 3 {
		t.Errorf("expected 3 chunks, got %d", len(chunks))
	}
}

// TestStreamableCall_BudgetExceeded verifies that a stream exceeding the
// byte budget is terminated with a "budget exceeded" error.
func TestStreamableCall_BudgetExceeded(t *testing.T) {
	// Each chunk is 50 chars; json.Marshal of a 50-char string = 52 bytes
	// (50 chars + 2 quote chars). Budget = 100 bytes.
	// After 2 chunks (104 bytes), budget is exceeded.
	inner := &slowStreamTool{
		name:         "budget_tool",
		chunkContent: strings.Repeat("x", 50),
		interval:    5 * time.Millisecond,
		chunkCount:   10,
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		StreamTimeout: 5 * time.Second,
		StreamBudget:  100,
		Logger:        loggateway.NewNoop(),
	})
	st := d.(trpctool.StreamableTool)
	reader, err := st.StreamableCall(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	defer reader.Close()

	chunks, streamErr := drainStream(reader)
	if streamErr == nil {
		t.Fatal("expected budget exceeded error, got clean EOF")
	}
	if !strings.Contains(streamErr.Error(), "budget exceeded") {
		t.Errorf("expected 'budget exceeded' error, got: %v", streamErr)
	}
	// Should have received at least 1 chunk before budget exceeded.
	if len(chunks) == 0 {
		t.Error("expected at least 1 chunk before budget exceeded")
	}
	// Should NOT have received all 10 chunks.
	if len(chunks) >= 10 {
		t.Error("expected stream to terminate before all chunks were sent")
	}
}

// TestStreamableCall_DeadlineExceeded verifies that a stream exceeding the
// timeout is terminated with a "cancelled" error.
func TestStreamableCall_DeadlineExceeded(t *testing.T) {
	inner := &slowStreamTool{
		name:         "deadline_tool",
		chunkContent: "chunk",
		interval:    20 * time.Millisecond,
		chunkCount:   100,
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		StreamTimeout: 10 * time.Millisecond, // Very short timeout
		StreamBudget:  DefaultStreamBudget,
		Logger:        loggateway.NewNoop(),
	})
	st := d.(trpctool.StreamableTool)
	reader, err := st.StreamableCall(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	defer reader.Close()

	_, streamErr := drainStream(reader)
	if streamErr == nil {
		t.Fatal("expected cancellation error, got clean EOF")
	}
	if !strings.Contains(streamErr.Error(), "cancelled") {
		t.Errorf("expected 'cancelled' error, got: %v", streamErr)
	}
}

// TestStreamableCall_ContextCancellation verifies that cancelling the
// caller's context terminates the stream.
func TestStreamableCall_ContextCancellation(t *testing.T) {
	inner := &slowStreamTool{
		name:         "cancel_tool",
		chunkContent: "chunk",
		interval:    20 * time.Millisecond,
		chunkCount:   100,
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		StreamTimeout: 5 * time.Second, // Long timeout so context cancel fires first
		StreamBudget:  DefaultStreamBudget,
		Logger:        loggateway.NewNoop(),
	})
	st := d.(trpctool.StreamableTool)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, err := st.StreamableCall(ctx, nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	defer reader.Close()

	// Cancel after a short delay.
	go func() {
		time.Sleep(10 * time.Millisecond)
		cancel()
	}()

	_, streamErr := drainStream(reader)
	// After context cancellation, the stream should terminate.
	// The error may be "cancelled" or clean EOF (if the inner tool closed
	// before the proxy noticed). Either is acceptable.
	if streamErr != nil && !strings.Contains(streamErr.Error(), "cancelled") {
		t.Errorf("expected 'cancelled' error or nil, got: %v", streamErr)
	}
}

// TestStreamableCall_InnerErrorPropagated verifies that an error from the
// inner tool is propagated to the consumer.
func TestStreamableCall_InnerErrorPropagated(t *testing.T) {
	expectedErr := errors.New("inner tool failure")
	inner := &decoratorMockStreamableTool{
		decoratorMockTool: decoratorMockTool{
			name: "error_tool",
			call: func(ctx context.Context, args []byte) (any, error) {
				return "ok", nil
			},
		},
		streamableCall: func(ctx context.Context, args []byte) (*trpctool.StreamReader, error) {
			stream := trpctool.NewStream(10)
			go func() {
				defer stream.Writer.Close()
				stream.Writer.Send(trpctool.StreamChunk{Content: "chunk1"}, nil)
				stream.Writer.Send(trpctool.StreamChunk{}, expectedErr)
			}()
			return stream.Reader, nil
		},
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		StreamTimeout: 5 * time.Second,
		StreamBudget:  DefaultStreamBudget,
		Logger:        loggateway.NewNoop(),
	})
	st := d.(trpctool.StreamableTool)
	reader, err := st.StreamableCall(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	defer reader.Close()

	chunks, streamErr := drainStream(reader)
	if streamErr == nil {
		t.Fatal("expected inner error, got clean EOF")
	}
	if !errors.Is(streamErr, expectedErr) {
		t.Errorf("expected error %v, got %v", expectedErr, streamErr)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk before error, got %d", len(chunks))
	}
}

// TestStreamableCall_DefaultTimeoutApplied verifies that a zero StreamTimeout
// in config defaults to DefaultStreamTimeout at runtime.
func TestStreamableCall_DefaultTimeoutApplied(t *testing.T) {
	inner := &slowStreamTool{
		name:         "default_timeout_tool",
		chunkContent: "chunk",
		interval:    5 * time.Millisecond,
		chunkCount:   1,
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		// StreamTimeout intentionally zero — should default to DefaultStreamTimeout.
		StreamBudget: DefaultStreamBudget,
		Logger:       loggateway.NewNoop(),
	})
	sd, ok := d.(*streamableToolDecorator)
	if !ok {
		t.Fatalf("expected *streamableToolDecorator, got %T", d)
	}
	if sd.cfg.StreamTimeout != 0 {
		t.Errorf("config StreamTimeout should be 0 (default applied at runtime), got %v", sd.cfg.StreamTimeout)
	}
	// Verify the stream completes normally with default timeout.
	st := d.(trpctool.StreamableTool)
	reader, err := st.StreamableCall(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	defer reader.Close()
	chunks, streamErr := drainStream(reader)
	if streamErr != nil {
		t.Errorf("expected clean EOF with default timeout, got: %v", streamErr)
	}
	if len(chunks) != 1 {
		t.Errorf("expected 1 chunk, got %d", len(chunks))
	}
}

// TestStreamableCall_NegativeBudgetDisablesBudget verifies that a negative
// StreamBudget disables budget enforcement (unlimited).
func TestStreamableCall_NegativeBudgetDisablesBudget(t *testing.T) {
	// Send 5 chunks of 50 chars each (52 bytes each = 260 bytes total).
	// With negative budget, all 5 chunks should pass through.
	inner := &slowStreamTool{
		name:         "unlimited_budget_tool",
		chunkContent: strings.Repeat("x", 50),
		interval:    5 * time.Millisecond,
		chunkCount:   5,
	}
	d := NewToolDecorator(inner, ToolDecoratorConfig{
		StreamTimeout: 5 * time.Second,
		StreamBudget:  -1, // unlimited
		Logger:        loggateway.NewNoop(),
	})
	st := d.(trpctool.StreamableTool)
	reader, err := st.StreamableCall(context.Background(), nil)
	if err != nil {
		t.Fatalf("StreamableCall error: %v", err)
	}
	defer reader.Close()

	chunks, streamErr := drainStream(reader)
	if streamErr != nil {
		t.Errorf("expected clean EOF with unlimited budget, got: %v", streamErr)
	}
	if len(chunks) != 5 {
		t.Errorf("expected 5 chunks with unlimited budget, got %d", len(chunks))
	}
}

// TestEstimateChunkBytes verifies byte estimation for various content types.
func TestEstimateChunkBytes(t *testing.T) {
	tests := []struct {
		name    string
		content any
		want    int
	}{
		{"nil", nil, 0},
		{"empty_string", "", 2},           // json.Marshal("") = `""` = 2 bytes
		{"short_string", "abc", 5},        // json.Marshal("abc") = `"abc"` = 5 bytes
		{"integer", 42, 2},                 // json.Marshal(42) = `42` = 2 bytes
		{"slice", []string{"a", "b"}, 9},  // json.Marshal(["a","b"]) = `["a","b"]` = 9 bytes
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			chunk := trpctool.StreamChunk{Content: tt.content}
			got := estimateChunkBytes(chunk)
			if got != tt.want {
				t.Errorf("estimateChunkBytes(%v) = %d, want %d", tt.content, got, tt.want)
			}
		})
	}
}
