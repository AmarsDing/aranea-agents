package sqlite

import "arenea/backend/internal/kernel/pkg/db"

// Open is the shared SQLite opener for Identity-driven SQL adapters (row #30).
// The monolith still uses repository.NewSQLiteRepository for the global pool.
var Open = db.OpenSQLite
