package callbacks

import (
	"context"

	trpctool "trpc.group/trpc-go/trpc-agent-go/tool"
)

// Decision is the explicit outcome of a waterfall interception point
// (P1-3, DSH §2.1). A hook must never express "reject" by returning a
// generic error — an error means the interceptor itself failed — nor
// express "rewrite" by mutating args in place: BeforeToolArgs.Arguments
// is a []byte copied into the tool call, so in-place writes never reach
// execution (latent bug fixed in P1-3; the framework's only write-back
// channel is BeforeToolResult.ModifiedArguments, see
// pkg/trpc-agent-go/internal/flow/processor/functioncall.go:1722).
//
// Asymmetry note: BeforeModel hooks mutate args.Request (a *Request
// pointer shared with the framework), so in-place message edits there
// ARE the framework-sanctioned rewrite channel — BeforeModelResult has
// no ModifiedRequest field. Decision therefore maps BeforeTool only.
type Decision struct {
	kind    decisionKind
	reason  string // reject: model-facing message
	rewrite []byte // rewrite: replacement tool arguments
}

type decisionKind int

const (
	decisionPass decisionKind = iota
	decisionReject
	decisionRewrite
)

// Pass lets the tool call proceed unchanged.
func Pass() Decision { return Decision{kind: decisionPass} }

// Reject blocks the tool call; reason is the model-facing message
// delivered as the tool result (short-circuit, no error log).
func Reject(reason string) Decision { return Decision{kind: decisionReject, reason: reason} }

// RewriteArgs continues the tool call with replacement arguments.
func RewriteArgs(args []byte) Decision { return Decision{kind: decisionRewrite, rewrite: args} }

// BeforeToolResult maps the decision onto the framework's BeforeTool
// contract:
//
//	Reject  → CustomResult (short-circuits before execution; the model
//	          sees the reason as the tool result, no spurious callback
//	          error is logged)
//	Rewrite → ModifiedArguments (the ONLY channel the framework writes
//	          back to toolCall.Function.Arguments)
//	Pass    → context only
func (d Decision) BeforeToolResult(ctx context.Context) *trpctool.BeforeToolResult {
	switch d.kind {
	case decisionReject:
		return &trpctool.BeforeToolResult{Context: ctx, CustomResult: d.reason}
	case decisionRewrite:
		return &trpctool.BeforeToolResult{Context: ctx, ModifiedArguments: d.rewrite}
	default:
		return &trpctool.BeforeToolResult{Context: ctx}
	}
}
