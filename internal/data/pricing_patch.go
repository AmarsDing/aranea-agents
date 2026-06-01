package data

import (
	"context"

	"aranea-agents/internal/data/ent"
	"aranea-agents/pkg/loggateway"
)

func ensurePricingRulePatches(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	if c == nil {
		return nil
	}
	patches := []struct {
		col string
		ddl string
	}{
		{"input_price_usd_per_1m", `ALTER TABLE model_pricing_rules ADD COLUMN input_price_usd_per_1m REAL NOT NULL DEFAULT 0`},
		{"output_price_usd_per_1m", `ALTER TABLE model_pricing_rules ADD COLUMN output_price_usd_per_1m REAL NOT NULL DEFAULT 0`},
		{"cached_input_price_usd_per_1m", `ALTER TABLE model_pricing_rules ADD COLUMN cached_input_price_usd_per_1m REAL NOT NULL DEFAULT 0`},
		{"reasoning_price_usd_per_1m", `ALTER TABLE model_pricing_rules ADD COLUMN reasoning_price_usd_per_1m REAL NOT NULL DEFAULT 0`},
		{"embedding_price_usd_per_1m", `ALTER TABLE model_pricing_rules ADD COLUMN embedding_price_usd_per_1m REAL NOT NULL DEFAULT 0`},
		{"cache_write_price_micro_usd_per_1k", `ALTER TABLE model_pricing_rules ADD COLUMN cache_write_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0`},
		{"cache_write_price_usd_per_1m", `ALTER TABLE model_pricing_rules ADD COLUMN cache_write_price_usd_per_1m REAL NOT NULL DEFAULT 0`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, lg, "model_pricing_rules", p.col)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			return err
		}
	}
	return ensureUsageEventPricingPatches(ctx, c, lg)
}

func ensureUsageEventPricingPatches(ctx context.Context, c *ent.Client, lg loggateway.Logger) error {
	patches := []struct {
		col string
		ddl string
	}{
		{"cache_write_tokens", `ALTER TABLE model_token_usage_events ADD COLUMN cache_write_tokens INTEGER NOT NULL DEFAULT 0`},
		{"cache_write_price_micro_usd_per_1k", `ALTER TABLE model_token_usage_events ADD COLUMN cache_write_price_micro_usd_per_1k INTEGER NOT NULL DEFAULT 0`},
		{"cache_write_cost_micro_usd", `ALTER TABLE model_token_usage_events ADD COLUMN cache_write_cost_micro_usd INTEGER NOT NULL DEFAULT 0`},
		{"canonical_provider_code", `ALTER TABLE model_token_usage_events ADD COLUMN canonical_provider_code TEXT NOT NULL DEFAULT ''`},
	}
	for _, p := range patches {
		has, err := sqliteColumnExists(ctx, c, lg, "model_token_usage_events", p.col)
		if err != nil {
			return err
		}
		if has {
			continue
		}
		if _, err := c.ExecContext(ctx, p.ddl); err != nil {
			return err
		}
	}
	return nil
}
