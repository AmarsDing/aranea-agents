package repository

import (
	"strings"

	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) ListChatOptions(optionType string) ([]domain.ChatOption, error) {
	query := `SELECT type, key, label, enabled, sort_order, metadata_json FROM chat_options WHERE enabled = 1`
	args := []any{}
	if strings.TrimSpace(optionType) != "" {
		query += ` AND type = ?`
		args = append(args, optionType)
	}
	query += ` ORDER BY type ASC, sort_order ASC`
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []domain.ChatOption
	for rows.Next() {
		var v domain.ChatOption
		if err = rows.Scan(&v.Type, &v.Key, &v.Label, &v.Enabled, &v.SortOrder, &v.MetadataJSON); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
