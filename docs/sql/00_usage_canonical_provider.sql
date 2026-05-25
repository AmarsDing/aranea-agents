-- Usage events: store models.dev canonical provider id at write time (read-side alias fallback for legacy rows).
ALTER TABLE model_token_usage_events ADD COLUMN canonical_provider_code TEXT NOT NULL DEFAULT '';
