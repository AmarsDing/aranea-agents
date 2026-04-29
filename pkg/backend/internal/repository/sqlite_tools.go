package repository

import (
	"arenea/backend/internal/capability/adapters/sqlite"
	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) toolRepo() *sqlite.ToolRepository {
	return sqlite.NewToolRepository(r.db)
}

// InsertToolInvocation is retained on the legacy repository while tool
// persistence migrates into the Capability/Tools storage adapter.
func (r *SQLiteRepository) InsertToolInvocation(t domain.ToolInvocation) (domain.ToolInvocation, error) {
	return r.toolRepo().InsertToolInvocation(t)
}

func (r *SQLiteRepository) SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error) {
	return r.toolRepo().SearchTools(query)
}

func (r *SQLiteRepository) GetToolByID(id string) (domain.Tool, error) {
	return r.toolRepo().GetToolByID(id)
}

func (r *SQLiteRepository) CreateTool(input domain.ToolUpsertInput) (domain.Tool, error) {
	return r.toolRepo().CreateTool(input)
}

func (r *SQLiteRepository) UpdateTool(id string, input domain.ToolUpsertInput) (domain.Tool, error) {
	return r.toolRepo().UpdateTool(id, input)
}

func (r *SQLiteRepository) DeleteTool(id string) error {
	return r.toolRepo().DeleteTool(id)
}

func (r *SQLiteRepository) UpdateToolEnabled(id string, enabled bool) (domain.Tool, error) {
	return r.toolRepo().UpdateToolEnabled(id, enabled)
}

func (r *SQLiteRepository) SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error) {
	return r.toolRepo().SearchToolInvocations(query)
}
