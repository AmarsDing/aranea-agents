package sqlite

import (
	"database/sql"

	"arenea/backend/internal/domain"
	toolstorage "arenea/backend/internal/capability/storage"
)

// ToolRepository 将工具持久化委托给 capability/storage SQLite 适配器。
type ToolRepository struct {
	db *sql.DB
}

// NewToolRepository 返回基于 *sql.DB 的工具仓库。
func NewToolRepository(db *sql.DB) *ToolRepository {
	return &ToolRepository{db: db}
}

func (r *ToolRepository) store() *toolstorage.SQLiteStore {
	return toolstorage.NewSQLiteStore(r.db)
}

// InsertToolInvocation 记录一次工具调用。
func (r *ToolRepository) InsertToolInvocation(t domain.ToolInvocation) (domain.ToolInvocation, error) {
	return r.store().InsertToolInvocation(t)
}

// SearchTools 分页查询工具元数据。
func (r *ToolRepository) SearchTools(query domain.ToolListQuery) (domain.ToolListResult, error) {
	return r.store().SearchTools(query)
}

// GetToolByID 按 id 取工具。
func (r *ToolRepository) GetToolByID(id string) (domain.Tool, error) {
	return r.store().GetToolByID(id)
}

// CreateTool 创建工具行。
func (r *ToolRepository) CreateTool(input domain.ToolUpsertInput) (domain.Tool, error) {
	return r.store().CreateTool(input)
}

// UpdateTool 更新工具行。
func (r *ToolRepository) UpdateTool(id string, input domain.ToolUpsertInput) (domain.Tool, error) {
	return r.store().UpdateTool(id, input)
}

// DeleteTool 删除（软删）工具。
func (r *ToolRepository) DeleteTool(id string) error {
	return r.store().DeleteTool(id)
}

// UpdateToolEnabled 切换启用位。
func (r *ToolRepository) UpdateToolEnabled(id string, enabled bool) (domain.Tool, error) {
	return r.store().UpdateToolEnabled(id, enabled)
}

// SearchToolInvocations 查询工具执行记录。
func (r *ToolRepository) SearchToolInvocations(query domain.ToolRunQuery) (domain.ToolRunResult, error) {
	return r.store().SearchToolInvocations(query)
}
