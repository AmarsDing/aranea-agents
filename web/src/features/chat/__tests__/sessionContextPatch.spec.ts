import { describe, expect, it } from "vitest";
import {
  contextRatioFromUsage,
  contextStatusFromRatio,
  isSessionCompressNotice,
  sessionContextPatchFromCompressMeta,
  sessionContextPatchFromStepUsage,
  sessionContextPatchFromUsage,
} from "../sessionContextPatch";
import type { Envelope } from "../envelope";

describe("sessionContextPatch", () => {
  it("derives ratio from runner_completion usage", () => {
    expect(contextRatioFromUsage({ prompt_tokens: 50_000, completion_tokens: 100, total_tokens: 50_100, max_tokens: 128_000 })).toBeCloseTo(
      50_000 / 128_000
    );
  });

  it("caps ratio at 1", () => {
    expect(contextRatioFromUsage({ prompt_tokens: 200_000, completion_tokens: 0, total_tokens: 200_000, max_tokens: 128_000 })).toBe(1);
  });

  it("builds patch with turn token increment", () => {
    const patch = sessionContextPatchFromUsage(
      { prompt_tokens: 64_000, completion_tokens: 512, total_tokens: 64_512, max_tokens: 128_000, turn_total_tokens: 64_512 },
      { total_tokens: 1000, max_context_used_ratio: 0.3 }
    );
    expect(patch?.context_used_ratio).toBeCloseTo(0.5);
    expect(patch?.context_status).toBe("normal");
    expect(patch?.total_tokens).toBe(1000 + 64_512);
    expect(patch?.max_context_used_ratio).toBeCloseTo(0.5);
  });

  it("maps compress metadata to patch", () => {
    const patch = sessionContextPatchFromCompressMeta({
      kind: "system.session.compress",
      context_used_ratio: 0.22,
      context_used_tokens: 28_000,
      context_status: "normal",
    });
    expect(patch).toEqual({
      context_used_ratio: 0.22,
      context_used_tokens: 28_000,
      context_status: "normal",
    });
  });

  it("detects compress notice envelope", () => {
    const env = {
      type: "text_done",
      metadata: { kind: "system.session.compress" },
    } as Envelope;
    expect(isSessionCompressNotice(env)).toBe(true);
  });

  it("step usage patch skips total_tokens", () => {
    const patch = sessionContextPatchFromStepUsage({
      prompt_tokens: 70_000,
      context_prompt_tokens: 70_000,
      completion_tokens: 200,
      total_tokens: 70_200,
      max_tokens: 128_000,
      turn_total_tokens: 70_200,
    });
    expect(patch?.context_used_ratio).toBeCloseTo(70_000 / 128_000);
    expect(patch?.total_tokens).toBeUndefined();
    expect(patch?.input_tokens).toBeUndefined();
  });

  it("prefers context_prompt_tokens over prompt_tokens for ratio", () => {
    expect(
      contextRatioFromUsage({
        prompt_tokens: 90_000,
        context_prompt_tokens: 50_000,
        completion_tokens: 100,
        total_tokens: 90_100,
        max_tokens: 128_000,
      })
    ).toBeCloseTo(50_000 / 128_000);
  });

  it("maps status thresholds", () => {
    expect(contextStatusFromRatio(0.5)).toBe("normal");
    expect(contextStatusFromRatio(0.65)).toBe("warning");
    expect(contextStatusFromRatio(0.85)).toBe("critical");
    expect(contextStatusFromRatio(0.96)).toBe("exceeded");
  });
});
