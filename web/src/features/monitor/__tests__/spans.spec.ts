import { describe, expect, it } from 'vitest';
import {
  countSpanNodes,
  filterSpanTree,
  flattenSpanTree,
  formatSpanDuration,
  normalizeTraceSpans,
  spanStatusTone,
  spanTimelineMaxEnd,
} from '../spans';

describe('spanStatusTone', () => {
  it('成功态', () => {
    expect(spanStatusTone('ok')).toBe('ok');
    expect(spanStatusTone('success')).toBe('ok');
  });

  it('运行态', () => {
    expect(spanStatusTone('running')).toBe('running');
  });

  it('失败态', () => {
    expect(spanStatusTone('error')).toBe('error');
    expect(spanStatusTone('failed')).toBe('error');
  });

  it('超时/中断为告警态', () => {
    expect(spanStatusTone('timeout')).toBe('warn');
    expect(spanStatusTone('interrupted')).toBe('warn');
  });

  it('空与未知为 idle', () => {
    expect(spanStatusTone('')).toBe('idle');
    expect(spanStatusTone(undefined)).toBe('idle');
    expect(spanStatusTone('whatever')).toBe('idle');
  });
});

describe('normalizeTraceSpans — 扁平 parent_id 线协议（持久化 spans）', () => {
  const wire = [
    { id: 'root', parent_id: '', kind: 'team', name: 'team.run', status: 'ok', start_ms: 0, duration_ms: 900 },
    { id: 'a', parent_id: 'root', kind: 'agent', name: 'agent.build', status: 'ok', start_ms: 10, duration_ms: 200 },
    {
      id: 'b',
      parent_id: 'root',
      kind: 'tool',
      name: 'tool.call',
      status: 'error',
      start_ms: 300,
      duration_ms: 100,
      error: 'boom',
    },
    { id: 'c', parent_id: 'a', kind: 'llm', name: 'llm.gen', status: 'ok', start_ms: 20, duration_ms: 50 },
  ];

  it('按 parent_id 建树并计算 depth', () => {
    const roots = normalizeTraceSpans(wire);
    expect(roots).toHaveLength(1);
    const root = roots[0]!;
    expect(root.id).toBe('root');
    expect(root.depth).toBe(0);
    expect(root.children.map((n) => n.id)).toEqual(['a', 'b']);
    const a = root.children[0]!;
    expect(a.depth).toBe(1);
    expect(a.children[0]!.id).toBe('c');
    expect(a.children[0]!.depth).toBe(2);
  });

  it('子树错误向上卷起 hasError', () => {
    const roots = normalizeTraceSpans(wire);
    const root = roots[0]!;
    expect(root.hasError).toBe(true);
    expect(root.children[0]!.hasError).toBe(false);
    expect(root.children[1]!.hasError).toBe(true);
  });

  it('孤儿节点（parent_id 悬空）提升为根', () => {
    const roots = normalizeTraceSpans([
      { id: 'orphan', parent_id: 'ghost', name: 'x', status: 'ok', start_ms: 0, duration_ms: 1 },
    ]);
    expect(roots).toHaveLength(1);
    expect(roots[0]!.id).toBe('orphan');
    expect(roots[0]!.depth).toBe(0);
  });

  it('子节点按 start_ms 排序', () => {
    const roots = normalizeTraceSpans([
      { id: 'r', parent_id: '', name: 'r', status: 'ok', start_ms: 0, duration_ms: 10 },
      { id: 's2', parent_id: 'r', name: 's2', status: 'ok', start_ms: 8, duration_ms: 1 },
      { id: 's1', parent_id: 'r', name: 's1', status: 'ok', start_ms: 2, duration_ms: 1 },
    ]);
    expect(roots[0]!.children.map((n) => n.id)).toEqual(['s1', 's2']);
  });
});

describe('normalizeTraceSpans — 嵌套 children 旧协议（metadata.spans）', () => {
  it('递归 children 建树', () => {
    const roots = normalizeTraceSpans([
      {
        id: 'root',
        name: 'root',
        status: 'ok',
        start_ms: 0,
        duration_ms: 100,
        children: [{ id: 'kid', name: 'kid', status: 'error', start_ms: 5, duration_ms: 10 }],
      },
    ]);
    expect(roots).toHaveLength(1);
    expect(roots[0]!.children[0]!.id).toBe('kid');
    expect(roots[0]!.children[0]!.parentId).toBe('root');
    expect(roots[0]!.hasError).toBe(true);
  });

  it('非对象条目被忽略，空输入返回空', () => {
    expect(normalizeTraceSpans([])).toEqual([]);
    expect(normalizeTraceSpans([null, 42, 'x'])).toEqual([]);
  });
});

describe('flattenSpanTree', () => {
  const roots = normalizeTraceSpans([
    { id: 'r', parent_id: '', name: 'r', status: 'ok', start_ms: 0, duration_ms: 10 },
    { id: 'a', parent_id: 'r', name: 'a', status: 'ok', start_ms: 1, duration_ms: 2 },
    { id: 'b', parent_id: 'a', name: 'b', status: 'ok', start_ms: 1, duration_ms: 1 },
  ]);

  it('先序展开全部', () => {
    expect(flattenSpanTree(roots, new Set()).map((n) => n.id)).toEqual(['r', 'a', 'b']);
  });

  it('折叠节点跳过其子树', () => {
    expect(flattenSpanTree(roots, new Set(['a'])).map((n) => n.id)).toEqual(['r', 'a']);
  });
});

describe('filterSpanTree', () => {
  const roots = normalizeTraceSpans([
    { id: 'r', parent_id: '', name: 'team.run', kind: 'team', status: 'ok', start_ms: 0, duration_ms: 10 },
    { id: 'a', parent_id: 'r', name: 'agent.build', kind: 'agent', status: 'ok', start_ms: 1, duration_ms: 2 },
    { id: 't', parent_id: 'r', name: 'web_search', kind: 'tool', status: 'ok', start_ms: 3, duration_ms: 4 },
  ]);

  it('命中子节点时保留祖先链', () => {
    const filtered = filterSpanTree(roots, 'web');
    expect(filtered).toHaveLength(1);
    expect(filtered[0]!.id).toBe('r');
    expect(filtered[0]!.children.map((n) => n.id)).toEqual(['t']);
  });

  it('按 kind 匹配', () => {
    const filtered = filterSpanTree(roots, 'tool');
    expect(filtered[0]!.children.map((n) => n.id)).toEqual(['t']);
  });

  it('空关键字原样返回', () => {
    expect(filterSpanTree(roots, '  ')).toBe(roots);
  });

  it('无命中返回空', () => {
    expect(filterSpanTree(roots, 'zzz')).toEqual([]);
  });
});

describe('spanTimelineMaxEnd / countSpanNodes', () => {
  it('maxEnd 取 start+duration 最大值，空树回退 1', () => {
    const roots = normalizeTraceSpans([
      { id: 'r', parent_id: '', name: 'r', status: 'ok', start_ms: 10, duration_ms: 90 },
      { id: 'a', parent_id: 'r', name: 'a', status: 'ok', start_ms: 400, duration_ms: 100 },
    ]);
    expect(spanTimelineMaxEnd(roots)).toBe(500);
    expect(spanTimelineMaxEnd([])).toBe(1);
  });

  it('计数含全部层级', () => {
    const roots = normalizeTraceSpans([
      { id: 'r', parent_id: '', name: 'r', status: 'ok', start_ms: 0, duration_ms: 1 },
      { id: 'a', parent_id: 'r', name: 'a', status: 'ok', start_ms: 0, duration_ms: 1 },
    ]);
    expect(countSpanNodes(roots)).toBe(2);
  });
});

describe('formatSpanDuration', () => {
  it('<1000 毫秒原样', () => {
    expect(formatSpanDuration(0)).toBe('0ms');
    expect(formatSpanDuration(42)).toBe('42ms');
  });

  it('≥1000 转秒一位小数', () => {
    expect(formatSpanDuration(1000)).toBe('1.0s');
    expect(formatSpanDuration(53200)).toBe('53.2s');
  });
});
