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
  it('uses cards only for columns tagged with card roles', () => {
    expect(defaultViewMode(cols)).toBe('table');
    expect(defaultViewMode([{ name: 'name', cardRole: 'title' }, { name: 'actions' }])).toBe('cards');
    expect(defaultViewMode([{ name: 'raw', cardRole: 'hidden' }, { name: 'actions' }])).toBe('table');
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

describe('splitColumns with card roles', () => {
  const roleCols = [
    { name: 'name', label: '名称', field: 'name', cardRole: 'title' as const },
    { name: 'source', label: '来源', field: 'source', cardRole: 'meta' as const },
    { name: 'runs', label: '运行', field: 'runs', cardRole: 'metric' as const },
    { name: 'enabled', label: '启用', field: 'enabled', cardRole: 'status' as const },
    { name: 'scope', label: '作用域', field: 'scope', cardRole: 'meta' as const },
    { name: 'raw', label: '原始', field: 'raw', cardRole: 'hidden' as const },
    { name: 'note', label: '备注', field: 'note' },
    { name: 'actions', label: '操作' },
  ];

  it('places each column by its role and keeps unlabeled ones in the body', () => {
    const layout = splitColumns(roleCols);
    expect(layout.title?.name).toBe('name');
    expect(layout.status?.name).toBe('enabled');
    expect(layout.metric?.name).toBe('runs');
    expect(layout.meta.map((c) => c.name)).toEqual(['source', 'scope']);
    expect(layout.body.map((c) => c.name)).toEqual(['note']);
    expect(layout.action?.name).toBe('actions');
  });

  it('falls back to the first unlabeled column as title', () => {
    const layout = splitColumns([{ name: 'a', cardRole: 'meta' as const }, { name: 'b' }, { name: 'c' }]);
    expect(layout.title?.name).toBe('b');
    expect(layout.body.map((c) => c.name)).toEqual(['c']);
    expect(layout.status).toBeNull();
  });
});
