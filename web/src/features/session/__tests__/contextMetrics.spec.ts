import { describe, expect, it } from 'vitest';
import {
  CHAT_CONTEXT_WINDOW_TOKENS,
  chatContextRatio,
  contextRatioFromPrompt,
} from '../contextMetrics';

describe('chat context window', () => {
  it('uses a fixed 256K product budget', () => {
    expect(CHAT_CONTEXT_WINDOW_TOKENS).toBe(256_000);
  });

  it('computes ratio against 256K, not a provider window', () => {
    expect(chatContextRatio(64_000)).toBeCloseTo(64_000 / 256_000);
    expect(contextRatioFromPrompt(64_000, 128_000)).toBeCloseTo(0.5);
    expect(chatContextRatio(64_000)).not.toBeCloseTo(0.5);
  });
});
