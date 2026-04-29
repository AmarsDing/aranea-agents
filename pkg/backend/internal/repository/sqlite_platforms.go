package repository

import (
	"database/sql"
	"errors"
	"fmt"

	"arenea/backend/internal/domain"
)

type platformTable struct {
	name      string
	keyColumn string
}

var platformTables = map[string]platformTable{
	"avatar-assets":       {name: "avatar_assets", keyColumn: "asset_key"},
	"agent-categories":    {name: "agent_category_nodes", keyColumn: "category_key"},
	"llm-provider-models": {name: "llm_provider_models", keyColumn: "model_key"},
	"hooks":               {name: "hooks", keyColumn: "hook_key"},
	"channels":            {name: "channel", keyColumn: "channel_key"},
	"mcp-servers":         {name: "mcp_server", keyColumn: "server_key"},
	"skills":              {name: "skill", keyColumn: "skill_key"},
	"cron-tasks":          {name: "cron_task", keyColumn: "task_key"},
	"monitor-events":      {name: "monitor_events", keyColumn: "event_key"},
	"monitor-traces":      {name: "monitor_traces", keyColumn: "trace_key"},
}

func platformTableFor(resource string) (platformTable, error) {
	table, ok := platformTables[resource]
	if !ok {
		return platformTable{}, fmt.Errorf("unsupported resource: %s", resource)
	}
	return table, nil
}

func (r *SQLiteRepository) ListPlatformResources(resource string) ([]domain.PlatformResource, error) {
	table, err := platformTableFor(resource)
	if err != nil {
		return nil, err
	}
	query := fmt.Sprintf(`SELECT id, %s, name, description, status, enabled, sort_order, %s, %s, %s, %s, %s, config_json, metadata_json, created_at, updated_at, deleted_at FROM %s WHERE deleted_at = '' ORDER BY sort_order ASC, created_at DESC`,
		table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"), table.name)
	rows, err := r.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanPlatformRows(resource, rows)
}

func (r *SQLiteRepository) GetPlatformResource(resource string, id string) (domain.PlatformResource, error) {
	table, err := platformTableFor(resource)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	query := fmt.Sprintf(`SELECT id, %s, name, description, status, enabled, sort_order, %s, %s, %s, %s, %s, config_json, metadata_json, created_at, updated_at, deleted_at FROM %s WHERE id = ? AND deleted_at = ''`,
		table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"), table.name)
	row := r.db.QueryRow(query, id)
	return scanPlatformResource(resource, row)
}

func (r *SQLiteRepository) CreatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error) {
	table, err := platformTableFor(v.Resource)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if v.ID == "" || v.Key == "" || v.Name == "" {
		return domain.PlatformResource{}, errors.New("id, key and name are required")
	}
	now := nowISO()
	if v.CreatedAt == "" {
		v.CreatedAt = now
	}
	v.UpdatedAt = now
	if v.Status == "" {
		v.Status = "active"
	}
	query := fmt.Sprintf(`INSERT INTO %s(id, %s, name, description, status, enabled, sort_order, %s, %s, %s, %s, %s, config_json, metadata_json, created_at, updated_at, deleted_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		table.name, table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"))
	_, err = r.db.Exec(query, v.ID, v.Key, v.Name, v.Description, v.Status, v.Enabled, v.SortOrder, v.ParentID, v.Level, v.AgentID, v.Provider, v.Model, v.ConfigJSON, v.MetadataJSON, v.CreatedAt, v.UpdatedAt, v.DeletedAt)
	return v, err
}

func (r *SQLiteRepository) UpdatePlatformResource(v domain.PlatformResource) (domain.PlatformResource, error) {
	table, err := platformTableFor(v.Resource)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	current, err := r.GetPlatformResource(v.Resource, v.ID)
	if err != nil {
		return domain.PlatformResource{}, err
	}
	if v.Key == "" {
		v.Key = current.Key
	}
	if v.Name == "" {
		v.Name = current.Name
	}
	if v.Status == "" {
		v.Status = current.Status
	}
	v.CreatedAt = current.CreatedAt
	v.UpdatedAt = nowISO()
	query := fmt.Sprintf(`UPDATE %s SET %s = ?, name = ?, description = ?, status = ?, enabled = ?, sort_order = ?, %s = ?, %s = ?, %s = ?, %s = ?, %s = ?, config_json = ?, metadata_json = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`,
		table.name, table.keyColumn, optionalColumn(table.name, "parent_id"), optionalColumn(table.name, "level"), optionalColumn(table.name, "agent_id"), optionalColumn(table.name, "provider"), optionalColumn(table.name, "model"))
	_, err = r.db.Exec(query, v.Key, v.Name, v.Description, v.Status, v.Enabled, v.SortOrder, v.ParentID, v.Level, v.AgentID, v.Provider, v.Model, v.ConfigJSON, v.MetadataJSON, v.UpdatedAt, v.ID)
	return v, err
}

func (r *SQLiteRepository) DeletePlatformResource(resource string, id string) error {
	table, err := platformTableFor(resource)
	if err != nil {
		return err
	}
	if resource == "agent-categories" {
		if err = r.ensureCategoryCanDelete(id); err != nil {
			return err
		}
	}
	_, err = r.db.Exec(fmt.Sprintf(`UPDATE %s SET deleted_at = ?, status = 'deleted', updated_at = ? WHERE id = ? AND deleted_at = ''`, table.name), nowISO(), nowISO(), id)
	return err
}

func (r *SQLiteRepository) ValidateProviderModel(provider string, model string) (bool, error) {
	if provider == "" || model == "" {
		return false, nil
	}
	var count int
	err := r.db.QueryRow(`SELECT COUNT(1) FROM llm_provider_models WHERE provider = ? AND model = ? AND enabled = 1 AND deleted_at = ''`, provider, model).Scan(&count)
	return count > 0, err
}

func (r *SQLiteRepository) GetProviderModel(provider string, model string) (domain.PlatformResource, error) {
	if provider == "" || model == "" {
		return domain.PlatformResource{}, errors.New("provider and model are required")
	}
	row := r.db.QueryRow(
		`SELECT id, model_key, name, description, status, enabled, sort_order, parent_id, level, agent_id, provider, model, config_json, metadata_json, created_at, updated_at, deleted_at
		 FROM llm_provider_models
		 WHERE provider = ? AND model = ? AND enabled = 1 AND deleted_at = ''
		 LIMIT 1`,
		provider,
		model,
	)
	return scanPlatformResource("llm-provider-models", row)
}

func (r *SQLiteRepository) ensureCategoryCanDelete(id string) error {
	var children int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM agent_category_nodes WHERE parent_id = ? AND deleted_at = ''`, id).Scan(&children); err != nil {
		return err
	}
	if children > 0 {
		return fmt.Errorf("category has %d child nodes", children)
	}
	var agents int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM agents WHERE category_position_id = ? AND deleted_at = ''`, id).Scan(&agents); err != nil {
		return err
	}
	if agents > 0 {
		return fmt.Errorf("category is used by %d agents", agents)
	}
	return nil
}

func scanPlatformResource(resource string, row scanner) (domain.PlatformResource, error) {
	var v domain.PlatformResource
	v.Resource = resource
	err := row.Scan(&v.ID, &v.Key, &v.Name, &v.Description, &v.Status, &v.Enabled, &v.SortOrder, &v.ParentID, &v.Level, &v.AgentID, &v.Provider, &v.Model, &v.ConfigJSON, &v.MetadataJSON, &v.CreatedAt, &v.UpdatedAt, &v.DeletedAt)
	return v, err
}

func scanPlatformRows(resource string, rows *sql.Rows) ([]domain.PlatformResource, error) {
	var result []domain.PlatformResource
	for rows.Next() {
		v, err := scanPlatformResource(resource, rows)
		if err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
