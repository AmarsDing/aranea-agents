import { describe, expect, it } from 'vitest';
import { buildModelCostSlices, buildProviderCostSlicesFromTopModels } from '../usageBreakdownSlices';
import type { ModelUsageBreakdownRow } from '../types';

const rows: ModelUsageBreakdownRow[] = [
  {
    provider_code: 'openai',
    model_api_id: 'gpt-4',
    model_display_name: 'GPT-4',
    agent_id: '',
    agent_key: '',
    call_count: 10,
    input_tokens: 0,
    output_tokens: 0,
    total_tokens: 0,
    total_cost_micro_usd: 3_000_000,
    avg_latency_ms: 0,
    avg_tokens_per_second: 0,
    success_rate: 1,
  },
  {
    provider_code: 'anthropic',
    model_api_id: 'claude',
    model_display_name: 'Claude',
    agent_id: '',
    agent_key: '',
    call_count: 5,
    input_tokens: 0,
    output_tokens: 0,
    total_tokens: 0,
    total_cost_micro_usd: 1_000_000,
    avg_latency_ms: 0,
    avg_tokens_per_second: 0,
    success_rate: 1,
  },
];

describe('usageBreakdownSlices', () => {
  it('builds model slices by cost desc', () => {
    const slices = buildModelCostSlices(rows, 2);
    expect(slices).toHaveLength(2);
    expect(slices[0].value).toBeCloseTo(3);
    expect(slices[0].name).toContain('openai');
  });

  it('aggregates provider from top model rows', () => {
    const slices = buildProviderCostSlicesFromTopModels(rows);
    expect(slices).toHaveLength(2);
    const openai = slices.find((s) => s.name === 'openai');
    expect(openai?.value).toBeCloseTo(3);
  });
});
