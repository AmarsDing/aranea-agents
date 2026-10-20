// U0-4 useSidebarSelection 状态机 + U0-5 行信息密度纯函数 全覆盖。
import { describe, expect, it } from 'vitest';
import { nextTick, ref } from 'vue';
import { docStatusTone, formatChunkCount, relativeTimeParts, useSidebarSelection } from '../useSidebarSelection';

const order = ['a', 'b', 'c', 'd', 'e'];

describe('useSidebarSelection', () => {
  it('toggle 反转单项并把锚点移到该项', () => {
    const s = useSidebarSelection(() => 'v1|');
    s.toggle('a');
    expect(s.selectedIds.value).toEqual(['a']);
    s.toggle('c');
    expect(s.selectedIds.value).toEqual(['a', 'c']);
    s.toggle('a');
    expect(s.selectedIds.value).toEqual(['c']);
  });

  it('isSelected / count 与空 id 防御', () => {
    const s = useSidebarSelection(() => 'v1|');
    s.toggle('');
    expect(s.count.value).toBe(0);
    s.toggle('a');
    expect(s.isSelected('a')).toBe(true);
    expect(s.isSelected('b')).toBe(false);
    expect(s.count.value).toBe(1);
  });

  it('rangeTo 正向与反向均选中闭区间并保留锚点', () => {
    const s = useSidebarSelection(() => 'v1|');
    s.toggle('b');
    s.rangeTo('d', order);
    expect(s.selectedIds.value).toEqual(['b', 'c', 'd']);
    // 锚点仍是 b：继续 shift 到起点应收缩成 [a,b]。
    s.rangeTo('a', order);
    expect(s.selectedIds.value).toEqual(['a', 'b']);
  });

  it('rangeTo 无锚点或锚点不在序列时退化为单选并补锚点', () => {
    const s = useSidebarSelection(() => 'v1|');
    s.rangeTo('c', order);
    expect(s.selectedIds.value).toEqual(['c']);
    // 锚点在序列外（如目录重载后顺序变化）。
    s.rangeTo('x', order);
    expect(s.selectedIds.value).toEqual(['x']);
  });

  it('rangeTo 空 id 防御', () => {
    const s = useSidebarSelection(() => 'v1|');
    s.rangeTo('', order);
    expect(s.count.value).toBe(0);
  });

  it('clear 清空选择与锚点，幂等', () => {
    const s = useSidebarSelection(() => 'v1|');
    s.toggle('a');
    s.clear();
    expect(s.count.value).toBe(0);
    s.rangeTo('b', order);
    expect(s.selectedIds.value).toEqual(['b']);
    s.clear();
    s.clear();
    expect(s.count.value).toBe(0);
  });

  it('scopeKey 变化自动清空（目录切换）', async () => {
    const scope = ref('v1|docs/');
    const s = useSidebarSelection(() => scope.value);
    s.toggle('a');
    s.toggle('b');
    expect(s.count.value).toBe(2);
    scope.value = 'v1|other/';
    await nextTick();
    expect(s.count.value).toBe(0);
  });

  it('retain 只保留指定子集并修正锚点', () => {
    const s = useSidebarSelection(() => 'v1|');
    s.toggle('a');
    s.toggle('b');
    s.toggle('c');
    s.retain(['b']);
    expect(s.selectedIds.value).toEqual(['b']);
    s.retain([]);
    expect(s.selectedIds.value).toEqual([]);
    // 锚点清空后 rangeTo 退化单选仍可用。
    s.rangeTo('a', order);
    expect(s.selectedIds.value).toEqual(['a']);
  });
});

describe('docStatusTone（U0-5 状态点映射）', () => {
  it.each([
    ['indexed', 'ok'],
    ['indexing', 'busy'],
    ['pending', 'wait'],
    ['error', 'err'],
    ['', 'none'],
    ['whatever', 'none'],
  ])('%s → %s', (status, tone) => {
    expect(docStatusTone(status)).toBe(tone);
  });
});

describe('relativeTimeParts（U0-5 相对时间分段）', () => {
  const now = Date.parse('2026-10-20T12:00:00Z');

  it('1 分钟内（含未来时钟漂移）→ justNow', () => {
    expect(relativeTimeParts('2026-10-20T11:59:30Z', now)).toEqual({ kind: 'justNow' });
    expect(relativeTimeParts('2026-10-20T12:05:00Z', now)).toEqual({ kind: 'justNow' });
  });

  it('分钟 / 小时 / 天 分段', () => {
    expect(relativeTimeParts('2026-10-20T11:55:00Z', now)).toEqual({ kind: 'minutes', n: 5 });
    expect(relativeTimeParts('2026-10-20T09:00:00Z', now)).toEqual({ kind: 'hours', n: 3 });
    expect(relativeTimeParts('2026-10-17T12:00:00Z', now)).toEqual({ kind: 'days', n: 3 });
  });

  it('边界值：60 分钟进位小时，24 小时进位天，30 天进位日期', () => {
    expect(relativeTimeParts('2026-10-20T11:00:00Z', now)).toEqual({ kind: 'hours', n: 1 });
    expect(relativeTimeParts('2026-10-19T12:00:00Z', now)).toEqual({ kind: 'days', n: 1 });
    expect(relativeTimeParts('2026-09-20T12:00:00Z', now)?.kind).toBe('date');
  });

  it('非法/空输入 → null（组件隐藏）', () => {
    expect(relativeTimeParts('', now)).toBeNull();
    expect(relativeTimeParts('not-a-date', now)).toBeNull();
  });
});

describe('formatChunkCount（U0-5 角标）', () => {
  it('缺字段 / 0 / 负数 → 空串隐藏', () => {
    expect(formatChunkCount(undefined)).toBe('');
    expect(formatChunkCount(0)).toBe('');
    expect(formatChunkCount(-3)).toBe('');
    expect(formatChunkCount('x')).toBe('');
  });

  it('常规与 k 缩写', () => {
    expect(formatChunkCount(5)).toBe('5');
    expect(formatChunkCount(1200)).toBe('1.2k');
    expect(formatChunkCount(2000)).toBe('2k');
  });
});
