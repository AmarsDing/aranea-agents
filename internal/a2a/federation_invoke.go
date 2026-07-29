package a2a

import (
	"context"

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
