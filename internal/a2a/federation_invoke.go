package a2a

import (
	"context"

	"aranea-agents/internal/biz"
	a2abiz "aranea-agents/internal/biz/a2a"
	"aranea-agents/pkg/loggateway"
)

// FederationRemoteInvoker adapts InvokeRemoteRegistry to the biz
// RemoteInvokeExecutor port (design F.6 step 9 / F.10): retry, SSRF
// validation and ClientAuthOptions behavior are inherited unchanged; only the
// retry policy is fixed to the package default at the composition root.
type FederationRemoteInvoker struct {
	lg          loggateway.Logger
	retryPolicy a2abiz.RetryPolicy
}

var _ a2abiz.RemoteInvokeExecutor = (*FederationRemoteInvoker)(nil)

// NewFederationRemoteInvoker constructs the adapter with the default retry policy.
func NewFederationRemoteInvoker(lg loggateway.Logger) *FederationRemoteInvoker {
	return &FederationRemoteInvoker{lg: lg, retryPolicy: a2abiz.DefaultRetryPolicy()}
}

// InvokeRemote executes one governed remote invocation.
func (i *FederationRemoteInvoker) InvokeRemote(ctx context.Context, remote a2abiz.RemoteAgent, capability, payloadJSON string, timeoutSec int) (string, error) {
	return InvokeRemoteRegistry(ctx, remote, capability, payloadJSON, timeoutSec, i.lg, i.retryPolicy)
}

// federationFlowLogWriter adapts biz.FlowLogWriter (service/event_adapter.go,
// backed by event.TraceEmitter + MonitorBus) to the a2abiz.FlowLogWriter port
// so FederationUsecase can emit a2a.invoke.* flow logs without importing
// internal/event or internal/biz. The a2a.* step ID prefix maps to
// TraceDomainA2A in the service adapter.
type federationFlowLogWriter struct {
	inner biz.FlowLogWriter
}

var _ a2abiz.FlowLogWriter = (*federationFlowLogWriter)(nil)

// NewFederationFlowLogWriter wraps the shared biz.FlowLogWriter for the
// federation usecase. Returns nil when inner is nil (tests), callers must
// nil-check.
func NewFederationFlowLogWriter(inner biz.FlowLogWriter) a2abiz.FlowLogWriter {
	if inner == nil {
		return nil
	}
	return &federationFlowLogWriter{inner: inner}
}

func (w *federationFlowLogWriter) LogFlowStart(ctx context.Context, sessionID, stepID, message string, pairs ...a2abiz.FlowLogPair) {
	w.inner.LogFlowStart(ctx, sessionID, stepID, message, flowLogPairs(pairs)...)
}

func (w *federationFlowLogWriter) LogFlowDone(ctx context.Context, sessionID, stepID, message string, pairs ...a2abiz.FlowLogPair) {
	w.inner.LogFlowDone(ctx, sessionID, stepID, message, flowLogPairs(pairs)...)
}

func (w *federationFlowLogWriter) LogFlowError(ctx context.Context, sessionID, stepID, message string, pairs ...a2abiz.FlowLogPair) {
	w.inner.LogFlowError(ctx, sessionID, stepID, message, flowLogPairs(pairs)...)
}

func flowLogPairs(pairs []a2abiz.FlowLogPair) []biz.LogPair {
	out := make([]biz.LogPair, 0, len(pairs))
	for _, p := range pairs {
		out = append(out, biz.LogPair{Key: p.Key, Value: p.Value})
	}
	return out
}
