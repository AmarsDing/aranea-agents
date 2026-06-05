-- Version 20260612: Pricing rule and usage event column patches
ALTER TABLE model_pricing_rules ADD COLUMN input_price_usd_per_1m REAL NOT NULL DEFAULT 0;
ALTER TABLE model_pricing_rules ADD COLUMN output_price_usd_per_1m REAL NOT NULL DEFAULT 0;
ALTER TABLE model_pricing_rules ADD COLUMN cached_input_price_usd_per_1m REAL NOT NULL DEFAULT 0;
ALTER TABLE model_pricing_rules ADD COLUMN reasoning_price_usd_per_1m REAL NOT NULL DEFAULT 0;
ALTER TABLE model_pricing_rules ADD COLUMN embedding_price_usd_per_1m REAL NOT NULL DEFAULT 0;
ALTER TABLE model_pricing_rules ADD COLUMN cache_write_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0;
ALTER TABLE model_pricing_rules ADD COLUMN cache_write_price_usd_per_1m REAL NOT NULL DEFAULT 0;
ALTER TABLE model_token_usage_events ADD COLUMN cache_write_tokens INTEGER NOT NULL DEFAULT 0;
ALTER TABLE model_token_usage_events ADD COLUMN cache_write_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0;
ALTER TABLE model_token_usage_events ADD COLUMN cache_write_cost_micro_usd INTEGER NOT NULL DEFAULT 0;
ALTER TABLE model_token_usage_events ADD COLUMN canonical_provider_code TEXT NOT NULL DEFAULT '';
