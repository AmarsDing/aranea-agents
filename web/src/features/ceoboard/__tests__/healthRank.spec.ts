// 健康度档位/折叠/排序单测（整合 P2-8，自 ceo-dashboard web/src/smoke.test.mjs
// 移植为 vitest 口径，断言内容逐项一致）。
import { describe, expect, it } from 'vitest';
import { healthRank, laneCollapsible, sortWorkflows } from '../healthRank';

describe('healthRank', () => {
  it('健康度档位:红>黄>绿>灰', () => {
    expect(healthRank('red')).toBeLessThan(healthRank('yellow'));
    expect(healthRank('yellow')).toBeLessThan(healthRank('green'));
    expect(healthRank('green')).toBeLessThan(healthRank('gray'));
  });

  it('未知档位归 gray 档(3)', () => {
    expect(healthRank('gray')).toBe(3);
    expect(healthRank('')).toBe(3);
    expect(healthRank('HEALTH_UNSPECIFIED')).toBe(3);
  });
});

describe('laneCollapsible', () => {
  it('折叠规则:全绿/灰可折叠,含红/黄不可', () => {
    expect(laneCollapsible(['green', 'gray'])).toBe(true);
    expect(laneCollapsible(['green', 'yellow'])).toBe(false);
    expect(laneCollapsible(['red'])).toBe(false);
  });
});

describe('sortWorkflows', () => {
  it('组内排序:红在最前;同级按告警数降序', () => {
    const ws = [
      { id: 'g1', health: 'green', alerts: 9 },
      { id: 'r1', health: 'red', alerts: 0 },
      { id: 'y1', health: 'yellow', alerts: 1 },
      { id: 'y2', health: 'yellow', alerts: 5 },
    ];
    const sorted = sortWorkflows(ws).map((w) => w.id);
    expect(sorted).toEqual(['r1', 'y2', 'y1', 'g1']);
  });

  it('同健康同告警按业务量降序,再按 id 字典序', () => {
    const ws = [
      { id: 'b', health: 'green', alerts: 0, volume_1h: 3 },
      { id: 'a', health: 'green', alerts: 0, volume_1h: 3 },
      { id: 'c', health: 'green', alerts: 0, volume_1h: 9 },
    ];
    expect(sortWorkflows(ws).map((w) => w.id)).toEqual(['c', 'a', 'b']);
  });

  it('不改动入参数组', () => {
    const ws = [
      { id: 'y', health: 'yellow' },
      { id: 'r', health: 'red' },
    ];
    sortWorkflows(ws);
    expect(ws.map((w) => w.id)).toEqual(['y', 'r']);
  });
});
