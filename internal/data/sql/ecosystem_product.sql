CREATE TABLE IF NOT EXISTS ecosystem_products (
  id TEXT NOT NULL PRIMARY KEY,
  name TEXT NOT NULL,
  display_name TEXT NOT NULL DEFAULT '',
  description TEXT NOT NULL DEFAULT '',
  type TEXT NOT NULL DEFAULT 'skill_pack',
  author_id TEXT NOT NULL DEFAULT 'system',
  version TEXT NOT NULL DEFAULT '1.0.0',
  price_model TEXT NOT NULL DEFAULT 'free',
  price_cents INTEGER NOT NULL DEFAULT 0,
  rating REAL NOT NULL DEFAULT 0,
  install_count INTEGER NOT NULL DEFAULT 0,
  config_json TEXT NOT NULL DEFAULT '{}',
  status TEXT NOT NULL DEFAULT 'published',
  created_at TEXT NOT NULL DEFAULT '',
  updated_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE TABLE IF NOT EXISTS ecosystem_installs (
  id TEXT NOT NULL PRIMARY KEY,
  product_id TEXT NOT NULL,
  installed_ref_id TEXT NOT NULL DEFAULT '',
  created_at TEXT NOT NULL DEFAULT '',
  deleted_at TEXT NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS idx_ecosystem_products_type ON ecosystem_products(type);
CREATE INDEX IF NOT EXISTS idx_ecosystem_installs_product ON ecosystem_installs(product_id);
