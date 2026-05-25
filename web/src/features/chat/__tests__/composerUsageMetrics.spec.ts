import { describe, expect, it } from "vitest";
import { formatComposerUsageDetail, composerContextColor } from "../composerUsageMetrics";

describe("composerUsageMetrics", () => {
  it("formats merged usage line", () => {
    const line = formatComposerUsageDetail({
      contextRatio: 0.5,
      contextUsedTokens: 64_000,
      contextWindow: 128_000,
      inputTokens: 120_000,
      outputTokens: 8_000,
      totalTokens: 128_000,
      totalCostMicroUsd: 1_250_000,
    });
    expect(line).toContain("ctx 64.0k/128.0k");
    expect(line).toContain("in 120.0k");
    expect(line).toContain("out 8.0k");
    expect(line).toContain("Σ 128.0k");
    expect(line).toContain("$1.25");
  });

  it("maps context status to progress color", () => {
    expect(composerContextColor("warning", 0.5)).toBe("warning");
    expect(composerContextColor(undefined, 0.85)).toBe("negative");
  });
});
