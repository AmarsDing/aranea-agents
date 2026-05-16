-- ============================================================
-- 核心表: admins, system_settings, user_embedding_settings
-- ============================================================

CREATE TABLE IF NOT EXISTS admins (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  name TEXT NOT NULL DEFAULT '',
  email TEXT NOT NULL DEFAULT '',
  avatar TEXT NOT NULL DEFAULT '',
  access TEXT NOT NULL DEFAULT '',
  password TEXT NOT NULL DEFAULT '',
  create_time TEXT NOT NULL DEFAULT (datetime('now')),
  update_time TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS system_settings (
  id INTEGER PRIMARY KEY,
  root_directory TEXT NOT NULL DEFAULT '',
  work_directory TEXT NOT NULL DEFAULT '',
  update_time TEXT NOT NULL DEFAULT (datetime('now'))
);

CREATE TABLE IF NOT EXISTS user_embedding_settings (
  id INTEGER PRIMARY KEY AUTOINCREMENT,
  user_id TEXT NOT NULL UNIQUE,
  vector_dimension INTEGER NOT NULL,
  provider TEXT NOT NULL DEFAULT '',
  model TEXT NOT NULL DEFAULT '',
  options_json TEXT NOT NULL DEFAULT '{}',
  update_time TEXT NOT NULL DEFAULT (datetime('now'))
);
