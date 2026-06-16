package event

import (
	"aranea-agents/internal/event/contract"
	"aranea-agents/pkg/loggateway"
)

// Re-export contract types for backward compatibility.
type (
	DropPolicy       = contract.DropPolicy
	ChannelPriority  = contract.ChannelPriority
	SubscribeOptions = contract.SubscribeOptions
	Bus              = contract.Bus
)

const (
	DropOldest              = contract.DropOldest
	DropNewest              = contract.DropNewest
	BlockUpTo               = contract.BlockUpTo
	ChannelPriorityCritical = contract.ChannelPriorityCritical
	ChannelPriorityNormal   = contract.ChannelPriorityNormal
)

// NewBus returns a new in-process event bus.
// Delegates to the framework bus.Bus[Envelope] implementation via busAdapter.
// Drop events are logged via loggateway (if lg is non-nil) or silently discarded.
func NewBus(lg loggateway.Logger) Bus {
	return newBusAdapter(lg)
}
