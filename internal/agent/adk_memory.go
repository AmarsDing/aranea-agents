package agent

import (
	"aranea-agents/internal/data/sessionmemory"

	"google.golang.org/adk/memory"
)

// NewADKMemoryService returns the default in-process memory backend (ADK in-memory).
// For SQLite-backed recall via memory_entities, use [NewSessionSQLiteMemoryService].
func NewADKMemoryService() memory.Service {
	return memory.InMemoryService()
}

// RunnerMemoryService selects ADK memory.Service: SQLite session entities when store is wired, else in-memory.
func RunnerMemoryService(store *sessionmemory.Store) memory.Service {
	if m := NewSessionSQLiteMemoryService(store); m != nil {
		return m
	}
	return NewADKMemoryService()
}
