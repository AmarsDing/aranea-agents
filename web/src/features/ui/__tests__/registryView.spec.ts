import { describe, expect, it } from 'vitest';
import { cellValue, defaultViewMode, nextCardIndex, quickFilterRows, splitColumns } from '../registryView';

const cols = [
  { name: 'select', label: '' },
  { name: 'name', label: '名称', field: 'name' },
  { name: 'status', label: '状态', field: (row: Record<string, unknown>) => (row.enabled ? '启用' : '停用') },
  { name: 'runs', label: '调用', field: 'runs', format: (v: unknown) => `${String(v)} 次` },
  { name: 'actions', label: '操作' },
];

describe('splitColumns', () => {
  it('puts the first usable column in the title, actions at the bottom, the rest in the body', () => {
    const { title, action, body } = splitColumns(cols);
    expect(title?.name).toBe('name');
    expect(action?.name).toBe('actions');
    expect(body.map((c) => c.name)).toEqual(['status', 'runs']);
  });

  it('recognises an action column by its label when the name says nothing', () => {
    expect(
      splitColumns([
        { name: 'a', field: 'a' },
        { name: 'x', label: ' 操作 ' },
      ]).action?.name,
    ).toBe('x');
  });
});

describe('defaultViewMode', () => {
  it('uses cards for object registries and tables for logs', () => {
    expect(defaultViewMode(cols)).toBe('cards');
    expect(defaultViewMode(cols.filter((c) => c.name !== 'actions'))).toBe('table');
    const wide = [...Array.from({ length: 9 }, (_, i) => ({ name: `c${i}` })), { name: 'actions' }];
    expect(defaultViewMode(wide)).toBe('table');
    expect(defaultViewMode([])).toBe('table');
  });
});

describe('cellValue', () => {
  it('reads string fields, function fields and applies format', () => {
    const row = { name: 'hook-a', enabled: true, runs: 3 };
    expect(cellValue(cols[1]!, row)).toBe('hook-a');
    expect(cellValue(cols[2]!, row)).toBe('启用');
    expect(cellValue(cols[3]!, row)).toBe('3 次');
    expect(cellValue(cols[4]!, row)).toBeUndefined();
  });
});

describe('quickFilterRows', () => {
  const rows = [
    { name: 'Alpha Hook', key: 'alpha', enabled: true, runs: 3 },
    { name: 'Beta', key: 'pre_tool', enabled: false, runs: 12 },
  ];

  it('matches every term, case-insensitively, across column values and row fields', () => {
    expect(quickFilterRows(rows, 'alpha', cols).map((r) => r.key)).toEqual(['alpha']);
    expect(quickFilterRows(rows, 'PRE_TOOL', cols).map((r) => r.key)).toEqual(['pre_tool']);
    expect(quickFilterRows(rows, '停用 12', cols).map((r) => r.key)).toEqual(['pre_tool']);
    expect(quickFilterRows(rows, 'beta 启用', cols)).toEqual([]);
  });

  it('returns everything for a blank query', () => {
    expect(quickFilterRows(rows, '   ', cols)).toHaveLength(2);
  });
});

describe('nextCardIndex', () => {
  // 3 列 × 2 行，第二行只有 2 张
  const rects = [
    { left: 0, top: 0 },
    { left: 100, top: 0 },
    { left: 200, top: 0 },
    { left: 0, top: 80 },
    { left: 100, top: 80 },
  ];

  it('moves sideways within bounds', () => {
    expect(nextCardIndex(rects, 0, 'ArrowLeft')).toBe(0);
    expect(nextCardIndex(rects, 1, 'ArrowRight')).toBe(2);
    expect(nextCardIndex(rects, 4, 'ArrowRight')).toBe(4);
    expect(nextCardIndex(rects, 3, 'Home')).toBe(0);
    expect(nextCardIndex(rects, 0, 'End')).toBe(4);
  });

  it('moves vertically to the nearest card in the adjacent row', () => {
    expect(nextCardIndex(rects, 1, 'ArrowDown')).toBe(4);
    expect(nextCardIndex(rects, 2, 'ArrowDown')).toBe(4);
    expect(nextCardIndex(rects, 3, 'ArrowUp')).toBe(0);
    expect(nextCardIndex(rects, 0, 'ArrowUp')).toBe(0);
    expect(nextCardIndex(rects, 4, 'ArrowDown')).toBe(4);
  });
});
