import { describe, expect, it } from 'vitest';
import { failureKey, groupFailures } from '../useFailureReasons';
import type { ModelTokenUsageEvent } from '../types';

function ev(p: Partial<ModelTokenUsageEvent>): ModelTokenUsageEvent {
  return {
    id: 'e',
    occurred_at: '2026-11-15T04:00:00Z',
    agent_key: '',
    status: 'failed',
    error_message: '',
    ...p,
  } as ModelTokenUsageEvent;
}

const MISSING =
  '[CHAT_AGENT/INTERNAL] 智能体构建失败，请检查智能体配置后重试 ([LLM_PROVIDER_MODEL/NOT_FOUND] provider model not found)';

describe('groupFailures', () => {
  it('同一错误合成一行，记次数、涉及的 Agent、最近一次，并认出「模型不存在」', () => {
    const rows = groupFailures([
      ev({
        agent_id: 'agent___skills__',
        agent_name: '技能管家',
        error_message: MISSING,
        occurred_at: '2026-11-15T04:27:00Z',
      }),
      ev({ agent_id: 'agent___skills__', agent_name: '技能管家', error_message: MISSING }),
      ev({
        agent_id: 'agent___memory__',
        agent_name: '记忆管家',
        error_message: MISSING,
        occurred_at: '2026-11-13T07:53:00Z',
      }),
      ev({ agent_id: 'a2', error_message: 'Too Many Requests (429)' }),
    ]);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toMatchObject({ count: 3, cause: 'modelMissing', fix: 'agent', lastAt: '2026-11-15T04:27:00Z' });
    expect(rows[0].agents.map((a) => a.name)).toEqual(['技能管家', '记忆管家']);
    expect(rows[1]).toMatchObject({ count: 1, cause: 'rateLimited', fix: '' });
  });

  it('错误里每次都不同的 ID、长数字不影响归并', () => {
    const a = failureKey({ error_message: 'session 448baab2-81ab-47cc-90aa-fb0189668812 failed after 12345 ms' });
    const b = failureKey({ error_message: 'session 0593bedb-3ac1-4237-a4ac-adedce5840e0 failed after 98765 ms' });
    expect(a).toBe(b);
  });

  it('认不出的错误原样显示，不给修复去处', () => {
    const [row] = groupFailures([ev({ error_message: 'something odd' })]);
    expect(row).toMatchObject({ cause: '', fix: '', message: 'something odd' });
  });
});
