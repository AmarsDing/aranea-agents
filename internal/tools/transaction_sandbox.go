package tools

import (
	"context"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"
)

// TxProvider executes a function within a single database transaction.
// Implementations must roll back on error and commit on nil return.
// Defined here (not in biz) because the transaction sandbox is a tools-layer
// concern; data layer provides the implementation via Wire.
type TxProvider interface {
	ExecInTx(ctx context.Context, fn func(ctx context.Context) error) error
}

// TransactionSandbox wraps a tool call in a database transaction. If the call
// fails, the transaction is rolled back; if it succeeds, the transaction is
// committed atomically. This guarantees DB-modifying tools cannot leave the
// database in a partial state.
type TransactionSandbox struct {
	tx      TxProvider
	handler ToolHandler
	lg      loggateway.Logger
}

// NewTransactionSandbox creates a sandbox that wraps handler calls in
// transactions provided by tx. If tx is nil, Execute returns an error.
func NewTransactionSandbox(tx TxProvider, handler ToolHandler, lg loggateway.Logger) (*TransactionSandbox, error) {
	if tx == nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "tx provider is required")
	}
	if handler == nil {
		return nil, apierror.BadRequest(apierror.DomainTool, "handler is required")
	}
	if lg == nil {
		lg = loggateway.NewNoop()
	}
	return &TransactionSandbox{
		tx:      tx,
		handler: handler,
		lg:      lg.With(loggateway.Domain(apierror.DomainTool)),
	}, nil
}

// Execute runs the tool call inside a transaction. The handler receives a
// tx-aware context (provided by TxProvider) so that any Repo calls inside the
// handler participate in the same transaction. On handler failure the
// transaction rolls back; on success it commits.
func (s *TransactionSandbox) Execute(ctx context.Context, call ToolCall) ToolResult {
	if s == nil || s.tx == nil {
		return ToolResult{CallID: call.ID, Name: call.Name, Success: false, Error: "transaction sandbox not initialized"}
	}
	start := time.Now()

	var innerResult ToolResult
	err := s.tx.ExecInTx(ctx, func(txCtx context.Context) error {
		innerResult = s.handler(txCtx, call)
		if !innerResult.Success {
			return apierror.Internal(apierror.DomainTool, "tool %s failed: %s", call.Name, innerResult.Error)
		}
		return nil
	})

	result := ToolResult{
		CallID:     call.ID,
		Name:       call.Name,
		Output:     innerResult.Output,
		DurationMS: time.Since(start).Milliseconds(),
	}
	if err != nil {
		result.Success = false
		result.Error = err.Error()
		s.lg.Warn("transaction sandbox rolled back",
			loggateway.StepID("txsandbox.rollback"),
			loggateway.Str("call_id", call.ID),
			loggateway.Err(err))
		return result
	}
	result.Success = true
	return result
}
