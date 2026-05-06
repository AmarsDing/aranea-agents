package agent

import (
	"google.golang.org/adk/memory"
)

// NewADKMemoryService returns the default in-process memory backend (ADK in-memory).
// For SQLite-backed recall via memory_entities, use [NewSessionSQLiteMemoryService].
func NewADKMemoryService() memory.Service {
	return memory.InMemoryService()
}
