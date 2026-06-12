// Package event provides backward-compatible aliases for event reliability types.
// New code should import aranea-agents/internal/event/contract directly.
//
// Deprecated: This file is a compatibility wrapper. The single source of truth
// for event reliability is internal/event/contract (Stability:stable).
// All new code should import the contract package directly instead of using
// these aliases.
package event

import "aranea-agents/internal/event/contract"

// EventReliability is a type alias for contract.EventReliability.
// Stability:evolving — compatibility wrapper; prefer contract.EventReliability.
type EventReliability = contract.EventReliability

const (
	// ReliabilityCritical events use WBPF (Write-Before-Publish-Fanout).
	ReliabilityCritical = contract.ReliabilityCritical

	// ReliabilityImportant events use BlockUpTo + async persistence.
	ReliabilityImportant = contract.ReliabilityImportant

	// ReliabilityInformational events are best-effort with no persistence.
	ReliabilityInformational = contract.ReliabilityInformational
)

// ClassifyEventReliability delegates to contract.ClassifyEventReliability.
func ClassifyEventReliability(t contract.EnvelopeType) EventReliability {
	return contract.ClassifyEventReliability(t)
}

// IsCriticalWBPFType delegates to contract.IsCriticalWBPFType.
func IsCriticalWBPFType(t contract.EnvelopeType) bool {
	return contract.IsCriticalWBPFType(t)
}

// RequiresBlockUpTo delegates to contract.RequiresBlockUpTo.
// It returns true if the event type must use BlockUpTo delivery
// (i.e., it is Critical or Important — never silently dropped).
func RequiresBlockUpTo(t contract.EnvelopeType) bool {
	return contract.RequiresBlockUpTo(t)
}
