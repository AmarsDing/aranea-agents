package repository

import (
	"errors"

	"arenea/backend/internal/domain"
)

func (r *SQLiteRepository) AddAuditLog(l domain.AuditLog) error {
	if l.ID == "" {
		return errors.New("id is required")
	}
	if l.CreatedAt == "" {
		l.CreatedAt = nowISO()
	}
	_, err := r.db.Exec(
		`INSERT INTO audit_logs(id, action, resource, resource_id, request_id, detail, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		l.ID, l.Action, l.Resource, l.ResourceID, l.RequestID, l.Detail, l.CreatedAt,
	)
	return err
}

func (r *SQLiteRepository) ListAuditLogs(limit int) ([]domain.AuditLog, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := r.db.Query(
		`SELECT id, action, resource, resource_id, request_id, detail, created_at
		 FROM audit_logs ORDER BY created_at DESC LIMIT ?`,
		limit,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var result []domain.AuditLog
	for rows.Next() {
		var v domain.AuditLog
		if err = rows.Scan(&v.ID, &v.Action, &v.Resource, &v.ResourceID, &v.RequestID, &v.Detail, &v.CreatedAt); err != nil {
			return nil, err
		}
		result = append(result, v)
	}
	return result, rows.Err()
}
