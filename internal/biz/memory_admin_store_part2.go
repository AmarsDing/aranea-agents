package biz

// MemoryAdminDeps composes the fine-grained admin store interfaces required by MemoryAdminUsecase.
// Unlike SessionAdminStore, this does not include L2RecallStore or L3FactWriter
// (which are injected via separate fields). This narrows the interface to only
// what MemoryAdminUsecase actually needs.
// Stability:evolving
type MemoryAdminDeps interface {
	L0AdminStore
	L1AdminReader
	L1TaskWriter
	L1TaskBoardWriter
	L1FieldWriter
	L1IdleTaskReader
	L1ExpiredFieldCleaner
	L2EpisodeWriter
	L3FactReader
	L3FactReviewStore
	L3ConflictStore
	PIIReviewStore
	L4EntityStore
	L4EvolutionStore
}

// MemoryLayerPorts groups the independent L0–L4 persistence ports used on the
// production runtime path (agent builder + MemorySet). This is a DTO of
// single-layer interfaces (ISP), not a god interface: callers must use the
// field they need. Replaces the deprecated SessionAdminStore aggregate field.
type MemoryLayerPorts struct {
	L0Admin    L0AdminStore
	L1Reader   L1AdminReader
	L1Tasks    L1TaskWriter
	L1Fields   L1FieldWriter
	L1Schema   L1SchemaReader
	L3Reader   L3FactReader
	L3Writer   L3FactWriter
	L4Entities L4EntityStore
}

// HasWorkingMemory reports whether L1 working-memory tool injection can run.
func (p MemoryLayerPorts) HasWorkingMemory() bool {
	return p.L1Reader != nil && p.L1Tasks != nil && p.L1Fields != nil
}

// SessionAdminStore is the composed admin port for L0–L4 session memory.
//
// Deprecated: This composed interface has grown too large (L0–L4 + recall/write).
// Production paths (Wire, MemorySet, agent builder) no longer depend on this
// type. New code must use the fine-grained sub-interfaces (L0AdminStore,
// L1AdminReader, L1TaskWriter, L3FactReader, L3FactWriter, L4EntityStore, …)
// or MemoryLayerPorts / MemoryAdminDeps. Retained only for tests and the
// data-layer adapter compile-time check.
// Stability:evolving
type SessionAdminStore interface {
	MemoryAdminDeps
	L2RecallStore
	L3FactWriter
}
