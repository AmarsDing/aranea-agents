import { describe, expect, it } from "vitest";
import { normalizeCatalogModelSummary } from "../catalogWire";

describe("catalogWire", () => {
  it("reads snake_case pricing from API JSON", () => {
    const model = normalizeCatalogModelSummary({
      id: "deepseek-v4-pro",
      name: "DeepSeek V4 Pro",
      input_usd_per_1m: 0.435,
      output_usd_per_1m: 0.87,
      cache_read_usd_per_1m: 0.003625,
      context_tokens: 1000000,
      output_tokens: 384000,
      reasoning: true,
      family: "deepseek-thinking",
    });
    expect(model.inputUsdPer1m).toBe(0.435);
    expect(model.outputUsdPer1m).toBe(0.87);
    expect(model.cacheReadUsdPer1m).toBe(0.003625);
    expect(model.contextTokens).toBe(1000000);
    expect(model.family).toBe("deepseek-thinking");
  });

  it("reads nested models.dev cost.input / cost.output", () => {
    const model = normalizeCatalogModelSummary({
      id: "deepseek-v4-pro",
      cost: { input: 0.435, output: 0.87, cache_read: 0.003625 },
    });
    expect(model.inputUsdPer1m).toBe(0.435);
    expect(model.outputUsdPer1m).toBe(0.87);
    expect(model.cacheReadUsdPer1m).toBe(0.003625);
  });
});
