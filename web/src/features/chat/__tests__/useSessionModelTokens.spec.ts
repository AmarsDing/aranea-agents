import { describe, expect, it } from 'vitest';
import { aggregateByModel } from '../useSessionModelTokens';
import type { SessionTurn } from '../../session/types';

function makeTurn(overrides: Partial<SessionTurn>): SessionTurn {
  return {
    id: 't1',
    session_id: 's1',
    run_id: 'r1',
    turn_number: 1,
    user_message_id: 'u1',
    assistant_message_id: 'a1',
    owner_type: 'agent',
    agent_id: 'ag1',
    team_id: '',
    status: 'completed',
    started_at: '',
    ended_at: '',
    duration_ms: 0,
    first_token_ms: 0,
    model_call_count: 1,
    tool_call_count: 0,
    skill_call_count: 0,
    mcp_call_count: 0,
    input_tokens: 100,
    output_tokens: 20,
    total_tokens: 120,
    total_cost_micro_usd: 0,
    final_provider: 'deepseek',
    final_model: 'deepseek-v4-flash',
    final_content_preview: '',
    error_code: '',
    error_message: '',
    metadata_json: '',
    created_at: '',
    updated_at: '',
    ...overrides,
  };
}

describe('aggregateByModel per-turn context_budget parsing', () => {
  it('parses the context_budget ledger from metadata_json onto the point', () => {
    const metadata = JSON.stringify({
      trace_id: 'x',
      context_budget: {
        est_tokens: { static_prefix: 7400, tools_schema: 3500, history: 688 },
        est_total_input: 12000,
        tools_count: 10,
        top_tools: [{ name: 'memory_search', est_tokens: 743 }],
      },
    });
    const { series } = aggregateByModel([makeTurn({ metadata_json: metadata })], 1);
    const point = series[0]?.points[0];
    expect(point?.budget).toEqual({
      est_tokens: { static_prefix: 7400, tools_schema: 3500, history: 688 },
      est_total_input: 12000,
      tools_count: 10,
      top_tools: [{ name: 'memory_search', est_tokens: 743 }],
    });
  });

  it('returns null budget when metadata_json is empty or malformed', () => {
    const { series } = aggregateByModel(
      [
        makeTurn({ turn_number: 1, metadata_json: '' }),
        makeTurn({ turn_number: 2, metadata_json: '{not-json' }),
        makeTurn({ turn_number: 3, metadata_json: '{"trace_id":"t"}' }),
      ],
      3,
    );
    const points = series[0]?.points ?? [];
    expect(points).toHaveLength(3);
    for (const p of points) expect(p.budget).toBeNull();
  });

  it('keeps token totals intact alongside the budget', () => {
    const metadata = JSON.stringify({
      context_budget: { est_tokens: { history: 100 }, est_total_input: 100, tools_count: 0 },
    });
    const { series } = aggregateByModel([makeTurn({ metadata_json: metadata })], 1);
    const point = series[0]?.points[0];
    expect(point?.inputTokens).toBe(100);
    expect(point?.outputTokens).toBe(20);
    expect(point?.totalTokens).toBe(120);
    expect(series[0]?.totalAll).toBe(120);
  });
});
