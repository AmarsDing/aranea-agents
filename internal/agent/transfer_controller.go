package agent

import (
	"context"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/apierror"
	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

const (
	// transferTargetTimeout is the timeout applied to each target agent run.
	transferTargetTimeout = 120 * time.Second
)

// Transfer depth shares the unified delegation bound (P1-4):
// maxDelegateDepth() (env ARANEA_MAX_DELEGATE_DEPTH, default 3), same as
// the agent-as-tool channel, so both delegation paths enforce one limit.

// TransferControllerImpl implements trpcagent.TransferController with depth
// limiting, target timeout, and transfer event logging.
//
// The controller is intentionally per-run (created in buildTurnRunOptions for
// each turn). Depth is tracked via an atomic counter scoped to a single run,
// since the framework invokes OnTransfer sequentially within one run but does
// not propagate depth through the context. A per-run instance ensures the
// counter resets at the start of each turn and isolates concurrent runs.
type TransferControllerImpl struct {
	lg    loggateway.Logger
	depth atomic.Int32
}

// NewTransferController creates a new TransferControllerImpl.
func NewTransferController(lg loggateway.Logger) *TransferControllerImpl {
	return &TransferControllerImpl{lg: lg.With(loggateway.Domain("agent"))}
}

// OnTransfer is called before the framework runs the target agent.
// It checks transfer depth, sets a target timeout, and logs the transfer event.
func (c *TransferControllerImpl) OnTransfer(
	ctx context.Context,
	fromAgent string,
	toAgent string,
) (time.Duration, error) {
	newDepth := int(c.depth.Add(1))

	c.lg.Info("Agent transfer",
		loggateway.StepID("agent.transfer"),
		loggateway.Str("from_agent", fromAgent),
		loggateway.Str("to_agent", toAgent),
		loggateway.Int("depth", newDepth),
	)

	if limit := maxDelegateDepth(); newDepth > limit {
		c.lg.Warn("Agent transfer 深度超限，已拒绝",
			loggateway.StepID("agent.transfer_depth_exceeded"),
			loggateway.Str("from_agent", fromAgent),
			loggateway.Str("to_agent", toAgent),
			loggateway.Int("depth", newDepth),
			loggateway.Int("max_depth", limit),
		)
		return 0, apierror.Forbidden(apierror.DomainAgent,
			"transfer depth %d exceeds max %d: %s → %s",
			newDepth, limit, fromAgent, toAgent)
	}

	return transferTargetTimeout, nil
}

// RuntimeState returns the map to be merged into RunOptions.RuntimeState
// to install this controller via trpcagent.MergeRuntimeState.
func (c *TransferControllerImpl) RuntimeState() map[string]any {
	return map[string]any{
		trpcagent.RuntimeStateKeyTransferController: trpcagent.TransferController(c),
	}
}
