package sqlite

import "database/sql"

// Store 承载与 Operations 相关的共库表访问（迁移 #31：cron_task_run 等）。
type Store struct {
	db *sql.DB
}

// NewStore 与 repository.SQLiteRepository 共用同一 *sql.DB。
func NewStore(db *sql.DB) *Store { return &Store{db: db} }
