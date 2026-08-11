import { describe, expect, it } from 'vitest';
import { COMMAND_DEFS, filterCommands, pushMru, type CommandId, type CommandItem } from '../commands';

function items(): CommandItem[] {
  return COMMAND_DEFS.map((def) => ({ def, title: def.id }));
}

describe('filterCommands（SP2-6 命令面板过滤）', () => {
  it('空查询返回全部注册命令（10 条，SP2-8 新增 ingest-text）', () => {
    expect(filterCommands(items(), '')).toHaveLength(10);
    expect(filterCommands(items(), '   ')).toHaveLength(10);
  });

  it('子序列匹配：前缀命中排在散列命中前', () => {
    const res = filterCommands(items(), 'new');
    expect(res.map((c) => c.def.id)).toEqual(['new-note', 'new-folder']);
  });

  it('散列子序列也可命中（n-t → new-note）', () => {
    const res = filterCommands(items(), 'n-t');
    expect(res.map((c) => c.def.id)).toContain('new-note');
  });

  it('无匹配返回空数组', () => {
    expect(filterCommands(items(), 'zzzzzz')).toEqual([]);
  });
});

describe('filterCommands 别名搜索（P2-6）', () => {
  it('英文别名命中（graph → open-graph）', () => {
    expect(filterCommands(items(), 'graph').map((c) => c.def.id)).toEqual(['open-graph']);
  });

  it('拼音别名命中（baocun → save）', () => {
    expect(filterCommands(items(), 'baocun').map((c) => c.def.id)).toEqual(['save']);
  });

  it('标题命中优先于别名散列命中', () => {
    const res = filterCommands(items(), 'new');
    expect(res.map((c) => c.def.id).slice(0, 2)).toEqual(['new-note', 'new-folder']);
  });
});

describe('filterCommands MRU 置顶（P2-6）', () => {
  it('空查询时 MRU 置顶，其余保持注册顺序', () => {
    const res = filterCommands(items(), '', ['save', 'new-note']);
    expect(res.map((c) => c.def.id).slice(0, 2)).toEqual(['save', 'new-note']);
    expect(res[2].def.id).toBe('new-folder');
    expect(res).toHaveLength(10);
  });

  it('MRU 含未知 id 时自动跳过', () => {
    const res = filterCommands(items(), '  ', ['ghost' as CommandId, 'save']);
    expect(res[0].def.id).toBe('save');
    expect(res).toHaveLength(10);
  });

  it('键入查询后 MRU 不参与排序（相关性接管）', () => {
    const res = filterCommands(items(), 'save', ['new-note']);
    expect(res.map((c) => c.def.id)).toEqual(['save']);
  });
});

describe('pushMru（P2-6）', () => {
  it('置顶 + 去重', () => {
    expect(pushMru(['save', 'new-note'], 'new-note')).toEqual(['new-note', 'save']);
    expect(pushMru([], 'save')).toEqual(['save']);
  });

  it('截断到 keep 条', () => {
    expect(pushMru(['save', 'new-note', 'open-graph'], 'close-tab')).toEqual(['close-tab', 'save', 'new-note']);
  });
});
