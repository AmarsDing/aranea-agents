package agent

import (
	"context"
	"fmt"
	"time"

	"aranea-agents/pkg/loggateway"

	trpcagent "trpc.group/trpc-go/trpc-agent-go/agent"
)

const (
	// transferMaxDepth is the maximum allowed agent transfer nesting depth.
	transferMaxDepth = 3

	// transferTargetTimeout is the timeout applied to each target agent run.
	transferTargetTimeout = 120 * time.Second
)

// transferDepthKey is the context key used to track the current transfer depth.
type transferDepthKey struct{}

// TransferControllerImpl implements trpcagent.TransferController with depth
// limiting, target timeout, and transfer event logging.
type TransferControllerImpl struct {
	lg loggateway.Logger
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
	depth := transferDepthFromContext(ctx)
	newDepth := depth + 1

	c.lg.Info("Agent transfer",
		loggateway.StepID("agent.transfer"),
		loggateway.Str("from_agent", fromAgent),
		loggateway.Str("to_agent", toAgent),
		loggateway.Str("depth", fmt.Sprintf("%d", newDepth)),
	)

	if newDepth > transferMaxDepth {
		c.lg.Warn("Agent transfer 深度超限，已拒绝",
			loggateway.StepID("agent.transfer_depth_exceeded"),
			loggateway.Str("from_agent", fromAgent),
			loggateway.Str("to_agent", toAgent),
			loggateway.Str("depth", fmt.Sprintf("%d", newDepth)),
			loggateway.Str("max_depth", fmt.Sprintf("%d", transferMaxDepth)),
		)
		return 0, fmt.Errorf("transfer depth %d exceeds max %d: %s → %s", newDepth, transferMaxDepth, fromAgent, toAgent)
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

// transferDepthFromContext extracts the current transfer depth from context.
func transferDepthFromContext(ctx context.Context) int {
	if v, ok := ctx.Value(transferDepthKey{}).(int); ok {
		return v
	}
	return 0
}
