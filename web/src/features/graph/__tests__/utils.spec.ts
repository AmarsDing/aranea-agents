import { describe, it, expect } from 'vitest';
import { buildCompositionChips, deriveGraphStatus } from '../utils';

describe('buildCompositionChips - R2-A.3 节点构成 chips', () => {
  it('按计数降序排列，同计数按类型名升序（确定性）', () => {
    const { chips, overflow } = buildCompositionChips({ llm: 2, agent: 5, tool: 5 });
    expect(chips).toEqual([
      { type: 'agent', count: 5 },
      { type: 'tool', count: 5 },
      { type: 'llm', count: 2 },
    ]);
    expect(overflow).toBe(0);
  });

  it('过滤计数为 0 的类型', () => {
    const { chips, overflow } = buildCompositionChips({ llm: 0, agent: 3, tool: undefined });
    expect(chips).toEqual([{ type: 'agent', count: 3 }]);
    expect(overflow).toBe(0);
  });

  it('超过 4 类折叠为 +N（N = 剩余类型数）', () => {
    const { chips, overflow } = buildCompositionChips({
      agent: 6,
      llm: 5,
      tool: 4,
      router: 3,
      join: 2,
      hitl: 1,
      function: 7,
    });
    expect(chips).toHaveLength(4);
    expect(chips.map((c) => c.type)).toEqual(['function', 'agent', 'llm', 'tool']);
    expect(overflow).toBe(3);
  });

  it('恰好 4 类不折叠', () => {
    const { chips, overflow } = buildCompositionChips({ agent: 1, llm: 1, tool: 1, router: 1 });
    expect(chips).toHaveLength(4);
    expect(overflow).toBe(0);
  });

  it('空构成返回空 chips + overflow 0', () => {
    expect(buildCompositionChips({})).toEqual({ chips: [], overflow: 0 });
  });

  it('支持自定义 max', () => {
    const { chips, overflow } = buildCompositionChips({ agent: 3, llm: 2, tool: 1 }, 2);
    expect(chips).toHaveLength(2);
    expect(overflow).toBe(1);
  });
});

describe('deriveGraphStatus - R2-A.1/A.2 状态映射', () => {
  it('无节点 = draft', () => {
    expect(deriveGraphStatus({ nodes: [] })).toBe('draft');
    expect(deriveGraphStatus({ nodes: null })).toBe('draft');
    expect(deriveGraphStatus({})).toBe('draft');
  });

  it('有节点 = ready', () => {
    expect(deriveGraphStatus({ nodes: [{}] })).toBe('ready');
  });
});
