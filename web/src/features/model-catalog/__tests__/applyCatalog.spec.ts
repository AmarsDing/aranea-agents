import { describe, expect, it } from "vitest";
import { applyCatalogModelFields, catalogModelToCost } from "../applyCatalog";
import type { CatalogModelSummary } from "../../../services/kratos/model_catalog/v1/index";

const deepseekV4Pro = {
  id: "deepseek-v4-pro",
  name: "DeepSeek V4 Pro",
  reasoning: true,
  inputUsdPer1m: 0.435,
  outputUsdPer1m: 0.87,
  cacheReadUsdPer1m: 0.003625,
  contextTokens: 1000000,
  outputTokens: 384000,
  interleavedJson: JSON.stringify({ field: "reasoning_content" }),
  family: "deepseek-thinking",
} as CatalogModelSummary;

describe("applyCatalog", () => {
  it("maps cost fields to provider form", () => {
    const cost = catalogModelToCost(deepseekV4Pro);
    expect(cost.input_usd_per_1m).toBe(0.435);
    expect(cost.output_usd_per_1m).toBe(0.87);
  });

  it("applyCatalogModelFields fills pricing and reasoning backfill", () => {
    const fields = applyCatalogModelFields("deepseek", deepseekV4Pro, true);
    expect(fields.model_api_id).toBe("deepseek-v4-pro");
    expect(fields.input_price_usd_per_1m).toBe(0.435);
    expect(fields.context_window_k).toBe(1000);
    expect(fields.reasoning_backfill).toBe(true);
  });
});
