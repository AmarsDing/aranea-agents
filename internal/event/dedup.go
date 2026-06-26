// Package event — deprecated dedup aliases.
//
// The canonical implementation lives in internal/event/contract/dedup.go.
// These aliases are retained temporarily for backwards compatibility while
// callers migrate to contract.NewEventDeduplicator. New code MUST import
// internal/event/contract directly.
//
// Deprecated: use internal/event/contract instead. Scheduled for removal in
// ADR-03 Phase 5 once EventBusConsumer is migrated to ActivityEventBus.
package event

import (
	"aranea-agents/internal/event/contract"
)

// DefaultDedupCapacity is the default capacity for EventDeduplicator.
//
// Deprecated: use contract.DefaultDedupCapacity.
const DefaultDedupCapacity = contract.DefaultDedupCapacity

// EventDeduplicator tracks seen event IDs to filter duplicates.
//
// Deprecated: use contract.EventDeduplicator.
type EventDeduplicator = contract.EventDeduplicator

// NewEventDeduplicator creates a deduplicator with the given capacity.
//
// Deprecated: use contract.NewEventDeduplicator.
func NewEventDeduplicator(capacity int) *EventDeduplicator {
	return contract.NewEventDeduplicator(capacity)
}
