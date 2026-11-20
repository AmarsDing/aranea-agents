import { describe, expect, it } from 'vitest';
import { annotateAgentOptions, pickDefaultAgent, type MemoryFootprint } from '../memoryFootprint';

const fp: MemoryFootprint = {
  agents: [
    { agent_id: 'gone', facts: 99, episodes: 0 }, // 不在可选列表里（无权限或已删）
    { agent_id: 'b', facts: 5, episodes: 2 },
  ],
  unowned: [{ scope_type: 'canary', facts: 320 }],
};

describe('memoryFootprint', () => {
  it('defaults to the visible agent with the most memory, else the first agent', () => {
    expect(pickDefaultAgent(['a', 'b', 'c'], fp)).toBe('b');
    expect(pickDefaultAgent(['a', 'c'], fp)).toBe('a');
    expect(pickDefaultAgent([], fp)).toBeNull();
  });
  it('labels agents that have memory and moves them to the top', () => {
    const out = annotateAgentOptions(
      [
        { label: 'A', value: 'a' },
        { label: 'B', value: 'b' },
      ],
      fp,
      (n) => `${n} 条`,
    );
    expect(out).toEqual([
      { label: 'B · 7 条', value: 'b' },
      { label: 'A', value: 'a' },
    ]);
  });
});
