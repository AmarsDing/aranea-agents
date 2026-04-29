package sqlite

import "database/sql"

// Store 提供 sessions / messages 表访问（与 legacy monolithic 库共库，见 aranea/docs/0 main design 迁移表 #25）。
type Store struct {
	db *sql.DB
}

// NewStore 在共享 *sql.DB 上构造（与 repository.SQLiteRepository 同库实例）。
func NewStore(db *sql.DB) *Store { return &Store{db: db} }
