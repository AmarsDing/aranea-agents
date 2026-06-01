CREATE TABLE IF NOT EXISTS industries (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL UNIQUE,
  name TEXT NOT NULL DEFAULT '',
  icon TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  scenario_key TEXT NOT NULL DEFAULT '',
  enabled INTEGER NOT NULL DEFAULT 1,
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS departments (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  industry_key TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  responsibilities_json TEXT NOT NULL DEFAULT '{}',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(key, industry_key)
);

CREATE INDEX idx_departments_industry_key ON departments(industry_key);

CREATE TABLE IF NOT EXISTS positions (
  id TEXT PRIMARY KEY,
  key TEXT NOT NULL DEFAULT '',
  name TEXT NOT NULL DEFAULT '',
  department_key TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  responsibilities_json TEXT NOT NULL DEFAULT '{}',
  skills_required_json TEXT NOT NULL DEFAULT '[]',
  seniority_level TEXT NOT NULL DEFAULT '',
  sort_order INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT '',
  UNIQUE(key, department_key)
);

CREATE INDEX idx_positions_department_key ON positions(department_key);
