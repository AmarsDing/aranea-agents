package repository

import (
	"database/sql"
	"encoding/json"
	"errors"
	"strings"

	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) SearchPlugins(query domain.PluginListQuery) (domain.PluginListResult, error) {
	if query.Limit <= 0 {
		query.Limit = 20
	}
	if query.Limit > 100 {
		query.Limit = 100
	}
	where := []string{"deleted_at = ''"}
	args := []any{}
	if search := strings.TrimSpace(query.Search); search != "" {
		like := "%" + strings.ToLower(search) + "%"
		where = append(where, "(LOWER(plugin_key) LIKE ? OR LOWER(name) LIKE ? OR LOWER(description) LIKE ?)")
		args = append(args, like, like, like)
	}
	if query.Category != "" {
		where = append(where, "category = ?")
		args = append(args, query.Category)
	}
	if query.Enabled == "true" {
		where = append(where, "enabled = 1")
	}
	if query.Enabled == "false" {
		where = append(where, "enabled = 0")
	}
	if query.CallbackPoint != "" {
		where = append(where, "callback_points_json LIKE ?")
		args = append(args, "%"+query.CallbackPoint+"%")
	}
	whereSQL := strings.Join(where, " AND ")
	var total int
	if err := r.db.QueryRow(`SELECT COUNT(1) FROM plugins WHERE `+whereSQL, args...).Scan(&total); err != nil {
		return domain.PluginListResult{}, err
	}
	listArgs := append([]any{}, args...)
	listArgs = append(listArgs, query.Limit, query.Offset)
	rows, err := r.db.Query(pluginSelectSQL()+` WHERE `+whereSQL+` ORDER BY sort_order ASC, created_at DESC LIMIT ? OFFSET ?`, listArgs...)
	if err != nil {
		return domain.PluginListResult{}, err
	}
	defer rows.Close()
	items, err := scanPlugins(rows)
	if err != nil {
		return domain.PluginListResult{}, err
	}
	return domain.PluginListResult{Items: items, Total: total, Limit: query.Limit, Offset: query.Offset}, nil
}

func (r *SQLiteRepository) UpsertPlugin(plugin domain.Plugin) (domain.Plugin, error) {
	now := nowISO()
	if plugin.ID == "" {
		plugin.ID = "plugin_" + plugin.Key
	}
	if plugin.Scope == "" {
		plugin.Scope = "global"
	}
	if plugin.CreatedAt == "" {
		plugin.CreatedAt = now
	}
	plugin.UpdatedAt = now
	callbacks, _ := json.Marshal(plugin.CallbackPoints)
	_, err := r.db.Exec(`
		INSERT INTO plugins(id, plugin_key, name, description, category, risk_level, status, enabled, scope, callback_points_json, sort_order, config_schema_json, config_json, default_config_json, created_at, updated_at, deleted_at)
		VALUES (?, ?, ?, ?, ?, ?, 'active', ?, ?, ?, ?, ?, ?, ?, ?, ?, '')
		ON CONFLICT(plugin_key) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			category = excluded.category,
			risk_level = excluded.risk_level,
			scope = excluded.scope,
			callback_points_json = excluded.callback_points_json,
			sort_order = excluded.sort_order,
			config_schema_json = excluded.config_schema_json,
			default_config_json = excluded.default_config_json,
			updated_at = excluded.updated_at,
			deleted_at = ''`,
		plugin.ID, plugin.Key, plugin.Name, plugin.Description, plugin.Category, plugin.RiskLevel, plugin.Enabled, plugin.Scope, string(callbacks), plugin.SortOrder, plugin.ConfigSchemaJSON, plugin.ConfigJSON, plugin.DefaultConfigJSON, plugin.CreatedAt, plugin.UpdatedAt,
	)
	if err != nil {
		return domain.Plugin{}, err
	}
	rows, err := r.db.Query(pluginSelectSQL()+` WHERE plugin_key = ? AND deleted_at = '' LIMIT 1`, plugin.Key)
	if err != nil {
		return domain.Plugin{}, err
	}
	defer rows.Close()
	items, err := scanPlugins(rows)
	if err != nil {
		return domain.Plugin{}, err
	}
	if len(items) == 0 {
		return domain.Plugin{}, sql.ErrNoRows
	}
	return items[0], nil
}

func (r *SQLiteRepository) UpdatePluginEnabled(id string, enabled bool) (domain.Plugin, error) {
	_, err := r.db.Exec(`UPDATE plugins SET enabled = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, enabled, nowISO(), id)
	if err != nil {
		return domain.Plugin{}, err
	}
	return r.getPluginByID(id)
}

func (r *SQLiteRepository) UpdatePluginConfig(id string, configJSON string) (domain.Plugin, error) {
	if strings.TrimSpace(configJSON) == "" {
		configJSON = "{}"
	}
	if !json.Valid([]byte(configJSON)) {
		return domain.Plugin{}, errors.New("plugin config_json must be valid JSON")
	}
	_, err := r.db.Exec(`UPDATE plugins SET config_json = ?, updated_at = ? WHERE id = ? AND deleted_at = ''`, configJSON, nowISO(), id)
	if err != nil {
		return domain.Plugin{}, err
	}
	return r.getPluginByID(id)
}

func (r *SQLiteRepository) ListEnabledPluginKeys() ([]string, error) {
	rows, err := r.db.Query(`SELECT plugin_key FROM plugins WHERE enabled = 1 AND deleted_at = '' ORDER BY sort_order ASC, created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	keys := []string{}
	for rows.Next() {
		var key string
		if err = rows.Scan(&key); err != nil {
			return nil, err
		}
		keys = append(keys, key)
	}
	return keys, rows.Err()
}

func (r *SQLiteRepository) getPluginByID(id string) (domain.Plugin, error) {
	rows, err := r.db.Query(pluginSelectSQL()+` WHERE id = ? AND deleted_at = '' LIMIT 1`, id)
	if err != nil {
		return domain.Plugin{}, err
	}
	defer rows.Close()
	items, err := scanPlugins(rows)
	if err != nil {
		return domain.Plugin{}, err
	}
	if len(items) == 0 {
		return domain.Plugin{}, sql.ErrNoRows
	}
	return items[0], nil
}

func pluginSelectSQL() string {
	return `SELECT id, plugin_key, name, description, category, risk_level, enabled, scope, callback_points_json, sort_order, config_schema_json, config_json, default_config_json, invoke_count, block_count, error_count, last_invoked_at, last_status, created_at, updated_at FROM plugins`
}

func scanPlugins(rows *sql.Rows) ([]domain.Plugin, error) {
	items := []domain.Plugin{}
	for rows.Next() {
		var item domain.Plugin
		var callbackJSON string
		if err := rows.Scan(
			&item.ID, &item.Key, &item.Name, &item.Description, &item.Category, &item.RiskLevel, &item.Enabled, &item.Scope,
			&callbackJSON, &item.SortOrder, &item.ConfigSchemaJSON, &item.ConfigJSON, &item.DefaultConfigJSON,
			&item.InvokeCount, &item.BlockCount, &item.ErrorCount, &item.LastInvokedAt, &item.LastStatus, &item.CreatedAt, &item.UpdatedAt,
		); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(callbackJSON), &item.CallbackPoints)
		item.Permissions = domain.PluginPermissions{CanView: true, CanToggle: true, CanEditConfig: true, CanViewLogs: true}
		items = append(items, item)
	}
	return items, rows.Err()
}
