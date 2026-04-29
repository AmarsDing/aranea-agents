package sqlite

import "arenea/backend/internal/kernel/pkg/db"

// Open is the shared SQLite opener for Conversation-driven SQL adapters (row #30).
var Open = db.OpenSQLite
