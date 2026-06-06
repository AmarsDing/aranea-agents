import { describe, it, expect } from 'vitest';
import { filterIndustries, summarizeIndustries } from '../industryMarketFilters';
import type { Industry } from '../types';

function makeIndustry(overrides: Partial<Industry> = {}): Industry {
  return {
    id: 'ind-1',
    key: 'softwaredev',
    name: '软件开发',
    description: '系统开发 / UE 游戏',
    icon: '💻',
    scenario_key: 'engineering',
    enabled: true,
    sort_order: 0,
    deptCount: 10,
    posCount: 45,
    agentCount: 96,
    installed: 12,
    ...overrides,
  };
}

describe('filterIndustries', () => {
  const industries: Industry[] = [
    makeIndustry({ key: 'softwaredev', name: '软件开发', description: '系统开发 / UE 游戏', enabled: true }),
    makeIndustry({ id: 'ind-2', key: 'selfmedia', name: '自媒体', description: '网文 / 短视频', enabled: true }),
    makeIndustry({ id: 'ind-3', key: 'finance', name: '金融投资', description: '证券研究 / 量化', enabled: false }),
  ];

  it('returns all when filters are empty/default', () => {
    expect(filterIndustries(industries, { query: '', status: 'all', source: 'all' })).toHaveLength(3);
  });

  it('matches by name (case-insensitive)', () => {
    // 用含英文字段的 industry 验证大小写不敏感
    const eng = makeIndustry({ key: 'marketing', name: 'Marketing', description: 'campaigns' });
    const result = filterIndustries([eng], { query: 'MARKET', status: 'all', source: 'all' });
    expect(result).toHaveLength(1);
  });

  it('matches by Chinese name', () => {
    const result = filterIndustries(industries, { query: '软件', status: 'all', source: 'all' });
    expect(result).toHaveLength(1);
    expect(result[0].key).toBe('softwaredev');
  });

  it('matches by key', () => {
    const result = filterIndustries(industries, { query: 'finance', status: 'all', source: 'all' });
    expect(result).toHaveLength(1);
    expect(result[0].key).toBe('finance');
  });

  it('matches by description', () => {
    const result = filterIndustries(industries, { query: 'UE', status: 'all', source: 'all' });
    expect(result).toHaveLength(1);
    expect(result[0].key).toBe('softwaredev');
  });

  it('trims whitespace from query', () => {
    const result = filterIndustries(industries, { query: '   金融   ', status: 'all', source: 'all' });
    expect(result).toHaveLength(1);
    expect(result[0].key).toBe('finance');
  });

  it('returns empty when no match', () => {
    const result = filterIndustries(industries, { query: '不存在的关键词', status: 'all', source: 'all' });
    expect(result).toHaveLength(0);
  });

  it('filters by status=enabled', () => {
    const result = filterIndustries(industries, { query: '', status: 'enabled', source: 'all' });
    expect(result).toHaveLength(2);
    expect(result.every((i) => i.enabled)).toBe(true);
  });

  it('filters by status=disabled', () => {
    const result = filterIndustries(industries, { query: '', status: 'disabled', source: 'all' });
    expect(result).toHaveLength(1);
    expect(result[0].key).toBe('finance');
  });

  it('combines query and status', () => {
    // 'selfmedia' is enabled — should match
    const a = filterIndustries(industries, { query: 'selfmedia', status: 'enabled', source: 'all' });
    expect(a).toHaveLength(1);

    // 'finance' is disabled — should not match enabled filter
    const b = filterIndustries(industries, { query: 'finance', status: 'enabled', source: 'all' });
    expect(b).toHaveLength(0);
  });

  it('source=custom returns empty (not yet supported in Industry type)', () => {
    const result = filterIndustries(industries, { query: '', status: 'all', source: 'custom' });
    expect(result).toHaveLength(0);
  });
});

describe('summarizeIndustries', () => {
  it('returns zeros for empty input', () => {
    const s = summarizeIndustries([]);
    expect(s).toEqual({
      total: 0,
      enabled: 0,
      disabled: 0,
      departments: 0,
      positions: 0,
      agents: 0,
      installed: 0,
    });
  });

  it('aggregates counts across all industries', () => {
    const s = summarizeIndustries([
      makeIndustry({ deptCount: 10, posCount: 45, agentCount: 96, installed: 12, enabled: true }),
      makeIndustry({ id: 'i2', deptCount: 5, posCount: 28, agentCount: 54, installed: 7, enabled: true }),
      makeIndustry({ id: 'i3', deptCount: 6, posCount: 32, agentCount: 58, installed: 3, enabled: false }),
    ]);
    expect(s.total).toBe(3);
    expect(s.enabled).toBe(2);
    expect(s.disabled).toBe(1);
    expect(s.departments).toBe(21);
    expect(s.positions).toBe(105);
    expect(s.agents).toBe(208);
    expect(s.installed).toBe(22);
  });

  it('falls back to posCount when agentCount is missing', () => {
    const s = summarizeIndustries([makeIndustry({ deptCount: 1, posCount: 10, agentCount: undefined, installed: 0 })]);
    expect(s.positions).toBe(10);
    expect(s.agents).toBe(10); // fallback
  });

  it('handles all fields undefined gracefully', () => {
    const s = summarizeIndustries([makeIndustry()]);
    // defaults: enabled=true, no counts
    expect(s.total).toBe(1);
    expect(s.enabled).toBe(1);
    expect(s.departments).toBe(10);
    expect(s.positions).toBe(45);
    expect(s.agents).toBe(96);
  });
});
