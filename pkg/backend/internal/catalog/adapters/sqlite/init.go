package sqlite

import "arenea/backend/internal/kernel/pkg/db"

// Open is the shared SQLite opener for Catalog-driven SQL adapters (row #30).
var Open = db.OpenSQLite
