package tools

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"aranea-agents/pkg/loggateway"
)

// fakeTxProvider is an in-memory TxProvider for testing. It records whether
// the transaction committed (fn returned nil) or rolled back (fn returned err).
type fakeTxProvider struct {
	mu          sync.Mutex
	commitCount int
	rollbackCount int
	injectErr   error // if non-nil, ExecInTx returns this before calling fn
}

func (p *fakeTxProvider) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.injectErr != nil {
		p.rollbackCount++
		return p.injectErr
	}
	if err := fn(ctx); err != nil {
		p.rollbackCount++
		return err
	}
	p.commitCount++
	return nil
}

func (p *fakeTxProvider) commits() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.commitCount
}

func (p *fakeTxProvider) rollbacks() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.rollbackCount
}

// TestTransactionSandbox_SuccessCommits verifies that a handler returning
// success causes the transaction to commit exactly once.
func TestTransactionSandbox_SuccessCommits(t *testing.T) {
	tx := &fakeTxProvider{}
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true, Output: "done"}
	}

	sandbox, err := NewTransactionSandbox(tx, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewTransactionSandbox: %v", err)
	}

	result := sandbox.Execute(context.Background(), ToolCall{ID: "tx-1", Name: "db_write"})
	if !result.Success {
		t.Errorf("expected success, got error: %s", result.Error)
	}
	if result.Output != "done" {
		t.Errorf("expected output 'done', got %q", result.Output)
	}
	if got := tx.commits(); got != 1 {
		t.Errorf("expected 1 commit, got %d", got)
	}
	if got := tx.rollbacks(); got != 0 {
		t.Errorf("expected 0 rollbacks, got %d", got)
	}
}

// TestTransactionSandbox_HandlerFailureRollsBack verifies that a handler
// returning a failed result causes the transaction to roll back.
func TestTransactionSandbox_HandlerFailureRollsBack(t *testing.T) {
	tx := &fakeTxProvider{}
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: "db error"}
	}

	sandbox, err := NewTransactionSandbox(tx, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewTransactionSandbox: %v", err)
	}

	result := sandbox.Execute(context.Background(), ToolCall{ID: "tx-2", Name: "db_write"})
	if result.Success {
		t.Error("expected failure, got success")
	}
	if result.Error == "" {
		t.Error("expected non-empty error message")
	}
	if got := tx.commits(); got != 0 {
		t.Errorf("expected 0 commits on handler failure, got %d", got)
	}
	if got := tx.rollbacks(); got != 1 {
		t.Errorf("expected 1 rollback on handler failure, got %d", got)
	}
}

// TestTransactionSandbox_TxProviderErrorPropagates verifies that if the
// TxProvider itself fails (e.g. begin tx failed), the result is a failure.
func TestTransactionSandbox_TxProviderErrorPropagates(t *testing.T) {
	injected := errors.New("connection refused")
	tx := &fakeTxProvider{injectErr: injected}
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}

	sandbox, err := NewTransactionSandbox(tx, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewTransactionSandbox: %v", err)
	}

	result := sandbox.Execute(context.Background(), ToolCall{ID: "tx-3", Name: "db_write"})
	if result.Success {
		t.Error("expected failure when TxProvider errors")
	}
	if !strings.Contains(result.Error, "connection refused") {
		t.Errorf("expected error to contain 'connection refused', got %q", result.Error)
	}
	if got := tx.rollbacks(); got != 1 {
		t.Errorf("expected 1 rollback on TxProvider error, got %d", got)
	}
}

// TestTransactionSandbox_NilTxProviderRejectsConstruction verifies that
// NewTransactionSandbox rejects a nil TxProvider.
func TestTransactionSandbox_NilTxProviderRejectsConstruction(t *testing.T) {
	_, err := NewTransactionSandbox(nil, func(ctx context.Context, call ToolCall) ToolResult {
		return ToolResult{}
	}, loggateway.NewNoop())
	if err == nil {
		t.Error("expected error for nil TxProvider, got nil")
	}
}

// TestTransactionSandbox_NilHandlerRejectsConstruction verifies that
// NewTransactionSandbox rejects a nil handler.
func TestTransactionSandbox_NilHandlerRejectsConstruction(t *testing.T) {
	tx := &fakeTxProvider{}
	_, err := NewTransactionSandbox(tx, nil, loggateway.NewNoop())
	if err == nil {
		t.Error("expected error for nil handler, got nil")
	}
}

// TestTransactionSandbox_NilSandboxReturnsError verifies nil safety.
func TestTransactionSandbox_NilSandboxReturnsError(t *testing.T) {
	var sandbox *TransactionSandbox
	result := sandbox.Execute(context.Background(), ToolCall{ID: "x", Name: "y"})
	if result.Success {
		t.Error("expected failure for nil sandbox")
	}
	if !strings.Contains(result.Error, "not initialized") {
		t.Errorf("expected 'not initialized' error, got %q", result.Error)
	}
}

// TestTransactionSandbox_HandlerReceivesTxCtx verifies that the handler is
// called with the tx-aware context provided by TxProvider (not the original
// ctx). This ensures Repo calls inside the handler participate in the tx.
func TestTransactionSandbox_HandlerReceivesTxCtx(t *testing.T) {
	tx := &fakeTxProvider{}
	type ctxKey struct{}
	txCtxValue := "tx-context-marker"

	// Wrap ExecInTx to inject a marker into the context passed to fn.
	wrappedTx := &ctxInjectingTxProvider{
		inner: tx,
		key:   ctxKey{},
		val:   txCtxValue,
	}

	var seenCtxVal interface{}
	handler := func(ctx context.Context, call ToolCall) ToolResult {
		seenCtxVal = ctx.Value(ctxKey{})
		return ToolResult{CallID: call.ID, Name: call.Name, Success: true}
	}

	sandbox, err := NewTransactionSandbox(wrappedTx, handler, loggateway.NewNoop())
	if err != nil {
		t.Fatalf("NewTransactionSandbox: %v", err)
	}

	result := sandbox.Execute(context.Background(), ToolCall{ID: "tx-4", Name: "db_write"})
	if !result.Success {
		t.Fatalf("expected success, got: %s", result.Error)
	}
	if seenCtxVal != txCtxValue {
		t.Errorf("expected handler to receive tx ctx marker %v, got %v", txCtxValue, seenCtxVal)
	}
}

// ctxInjectingTxProvider wraps a TxProvider and injects a key/val pair into
// the context passed to fn, so tests can verify the handler receives the
// tx-aware context.
type ctxInjectingTxProvider struct {
	inner TxProvider
	key   interface{}
	val   interface{}
}

func (p *ctxInjectingTxProvider) ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error {
	txCtx := context.WithValue(ctx, p.key, p.val)
	return p.inner.ExecInTx(txCtx, fn)
}
