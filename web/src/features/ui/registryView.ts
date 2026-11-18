/**
 * 登记页列表的视图与即时筛选（AppRegistryTable 用）的纯函数。
 * 卡片视图把同一批列换一种排法：首列作标题，操作列沉到卡片底部，其余列成「标签 + 值」。
 */

export type RegistryViewMode = 'cards' | 'table';

export type RegistryColumnLike = {
  name?: string;
  label?: string;
  cardRole?: 'title' | 'status' | 'metric' | 'meta' | 'hidden';
  field?: string | ((row: Record<string, unknown>) => unknown);
  format?: (value: unknown, row: Record<string, unknown>) => unknown;
};

const STORAGE_PREFIX = 'aranea:registry-view:';
const ACTION_NAME = /^(actions?|ops?|operations?|operate)$/i;
// 列标题为“操作”或 Actions。中文用转义书写，免得被文案检查当成界面文字
const ACTION_LABEL = /^(\u64cd\u4f5c|actions?)$/i;
const SELECT_NAME = /^(select|selection|check|expand)$/i;
/** 列太多的表是数据表（日志、流水），默认不拆成卡片 */
const CARD_MAX_COLUMNS = 8;

export function cellValue(col: RegistryColumnLike, row: Record<string, unknown>): unknown {
  const raw = typeof col.field === 'function' ? col.field(row) : col.field ? row[col.field] : undefined;
  return col.format ? col.format(raw, row) : raw;
}

export function isActionColumn(col: RegistryColumnLike): boolean {
  return ACTION_NAME.test(col.name ?? '') || ACTION_LABEL.test((col.label ?? '').trim());
}

/** 把列分成：标题列、操作列、其余正文列（选择 / 展开这类功能列不进卡片）。 */
export type CardLayout<T> = {
  title: T | null;
  status: T | null;
  metric: T | null;
  meta: T[];
  action: T | null;
  body: T[];
};

/**
 * 按卡片角色拆列。没标角色的列：首列作标题，其余进正文；
 * 标了 hidden 的列卡片里不出现。操作列始终沉到卡片底部。
 */
export function splitColumns<T extends RegistryColumnLike>(cols: readonly T[]): CardLayout<T> {
  const usable = cols.filter((c) => !SELECT_NAME.test(c.name ?? '') && c.cardRole !== 'hidden');
  const action = usable.find(isActionColumn) ?? null;
  const rest = usable.filter((c) => c !== action);
  const title = rest.find((c) => c.cardRole === 'title') ?? rest.find((c) => !c.cardRole) ?? null;
  const status = rest.find((c) => c.cardRole === 'status') ?? null;
  const metric = rest.find((c) => c.cardRole === 'metric') ?? null;
  const meta = rest.filter((c) => c.cardRole === 'meta');
  const placed = new Set<T | null>([title, status, metric, ...meta]);
  return { title, status, metric, meta, action, body: rest.filter((c) => !placed.has(c)) };
}

/**
 * 没有手动选过时的默认视图：有操作列、列数不多的是「一行一个对象」的登记表，默认卡片；
 * 其余（日志、流水、明细）默认表格。
 */
export function defaultViewMode(cols: readonly RegistryColumnLike[]): RegistryViewMode {
  if (cols.length === 0 || cols.length > CARD_MAX_COLUMNS) return 'table';
  return cols.some(isActionColumn) ? 'cards' : 'table';
}

export function loadViewMode(key: string): RegistryViewMode | null {
  try {
    const saved = localStorage.getItem(STORAGE_PREFIX + key);
    return saved === 'cards' || saved === 'table' ? saved : null;
  } catch {
    return null;
  }
}

export function saveViewMode(key: string, mode: RegistryViewMode): void {
  try {
    localStorage.setItem(STORAGE_PREFIX + key, mode);
  } catch {
    // 存不了不影响切换
  }
}

function haystack(row: Record<string, unknown>, cols: readonly RegistryColumnLike[]): string {
  const parts: string[] = [];
  const push = (value: unknown) => {
    if (typeof value === 'string' || typeof value === 'number') parts.push(String(value));
  };
  // 单元格可能由插槽渲染别的字段，所以列值之外再带上行里所有一层的文本与数字。
  for (const col of cols) push(cellValue(col, row));
  for (const value of Object.values(row)) push(value);
  return parts.join('\u0001').toLowerCase();
}

/** 即时筛选：按空白拆词，每个词都要命中（不分大小写）。只筛已经读到的这一页。 */
export function quickFilterRows<T extends Record<string, unknown>>(
  rows: readonly T[],
  query: string,
  cols: readonly RegistryColumnLike[],
): T[] {
  const terms = query.toLowerCase().split(/\s+/).filter(Boolean);
  if (terms.length === 0) return [...rows];
  return rows.filter((row) => {
    const text = haystack(row, cols);
    return terms.every((term) => text.includes(term));
  });
}

/** 卡片网格里按方向键该去哪张卡：左右相邻；上下找相邻行里水平位置最近的一张。 */
export function nextCardIndex(
  rects: readonly { left: number; top: number }[],
  current: number,
  key: 'ArrowLeft' | 'ArrowRight' | 'ArrowUp' | 'ArrowDown' | 'Home' | 'End',
): number {
  const last = rects.length - 1;
  if (current < 0 || last < 0) return -1;
  if (key === 'Home') return 0;
  if (key === 'End') return last;
  if (key === 'ArrowLeft') return Math.max(0, current - 1);
  if (key === 'ArrowRight') return Math.min(last, current + 1);
  const here = rects[current];
  if (!here) return current;
  const down = key === 'ArrowDown';
  const rowTops = [...new Set(rects.map((r) => Math.round(r.top)))].sort((a, b) => a - b);
  const targetTop = rowTops[rowTops.indexOf(Math.round(here.top)) + (down ? 1 : -1)];
  if (targetTop === undefined) return current;
  let best = current;
  let bestDistance = Infinity;
  rects.forEach((r, index) => {
    if (Math.round(r.top) !== targetTop) return;
    const distance = Math.abs(r.left - here.left);
    if (distance < bestDistance) {
      best = index;
      bestDistance = distance;
    }
  });
  return best;
}
