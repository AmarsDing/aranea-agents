// U0-4/U0-5 侧栏纯逻辑模块：多选状态机（ctrl/shift 点击语义）+ 行信息密度展示纯函数。
// 全部保持纯函数/最小依赖，便于 vitest 直接覆盖（侧边栏组件只做接线）。
import { computed, ref, watch, type ComputedRef } from 'vue';

export type SidebarSelection = ReturnType<typeof useSidebarSelection>;

/**
 * useSidebarSelection —— 文件列表多选状态机。
 * @param scopeKey 选择域标识（vaultId|prefix）；域变化自动清空（目录切换语义）。
 *
 * 语义（对照 Obsidian/VSCode 资源管理器）：
 * - toggle(id)：ctrl/cmd+click —— 反转单项选中，并把锚点移动到该项；
 * - rangeTo(id, orderedIds)：shift+click —— 在当前目录文件「展示顺序」内，
 *   选中锚点到 id 的闭区间（替换整个选择集）；无锚点或锚点不在序列中时退化为单选 id；
 * - clear()：清空选择与锚点；
 * - retain(ids)：批量操作后只保留失败项（成功的移出选择集）。
 */
export function useSidebarSelection(scopeKey: () => string) {
  // Set 保持插入序；每次变更整体替换以触发 Vue 响应式。
  const selected = ref<ReadonlySet<string>>(new Set());
  const anchor = ref('');

  const selectedIds: ComputedRef<string[]> = computed(() => Array.from(selected.value));
  const count: ComputedRef<number> = computed(() => selected.value.size);

  function isSelected(id: string): boolean {
    return selected.value.has(id);
  }

  function toggle(id: string) {
    if (!id) return;
    const next = new Set(selected.value);
    if (next.has(id)) next.delete(id);
    else next.add(id);
    selected.value = next;
    anchor.value = id;
  }

  function rangeTo(id: string, orderedIds: readonly string[]) {
    if (!id) return;
    const from = orderedIds.indexOf(anchor.value);
    const to = orderedIds.indexOf(id);
    if (from < 0 || to < 0) {
      // 无有效锚点（或目标不在当前序列）：退化为单选，并补锚点。
      selected.value = new Set([id]);
      anchor.value = id;
      return;
    }
    const [lo, hi] = from <= to ? [from, to] : [to, from];
    selected.value = new Set(orderedIds.slice(lo, hi + 1));
    // 锚点保持不动：按住 shift 连点可继续扩/缩范围（标准文件管理器语义）。
  }

  function clear() {
    if (!selected.value.size && !anchor.value) return;
    selected.value = new Set();
    anchor.value = '';
  }

  /** 批量操作后保留子集（如仅失败项），顺便修正失效锚点。 */
  function retain(ids: readonly string[]) {
    const keep = new Set(ids);
    const next = new Set([...selected.value].filter((id) => keep.has(id)));
    selected.value = next;
    if (anchor.value && !next.has(anchor.value)) {
      anchor.value = next.size ? Array.from(next)[next.size - 1] : '';
    }
  }

  // 目录/库切换：自动清空（U0-4 验收项）。
  watch(scopeKey, () => clear());

  return { selectedIds, count, isSelected, toggle, rangeTo, clear, retain };
}

// ---------- U0-5 行信息密度纯函数 ----------

export type DocStatusTone = 'ok' | 'busy' | 'wait' | 'err' | 'none';

/** 文档解析状态 → 状态点色调；空/未知状态隐藏（优雅降级）。 */
export function docStatusTone(status: string): DocStatusTone {
  switch (status) {
    case 'indexed':
      return 'ok';
    case 'indexing':
      return 'busy';
    case 'pending':
      return 'wait';
    case 'error':
      return 'err';
    default:
      return 'none';
  }
}

export type RelativeTimeParts =
  | { kind: 'justNow' }
  | { kind: 'minutes' | 'hours' | 'days'; n: number }
  | { kind: 'date'; date: string };

/**
 * relativeTimeParts —— 相对时间分段（<1min 刚刚，<60min N 分钟前，<24h N 小时前，
 * <30d N 天前，否则本地化日期）。返回结构供组件套 i18n 键，本身不含文案。
 * 现有工具（features/monitor eventView）与 graph/utils 版本分别绑死 monitor 命名空间
 * 与硬编码中文，不满足知识侧栏 i18n 键分离要求，故按规范自写纯函数。
 */
export function relativeTimeParts(iso: string, now = Date.now()): RelativeTimeParts | null {
  const ms = Date.parse(String(iso || ''));
  if (!Number.isFinite(ms)) return null;
  const minutes = Math.floor((now - ms) / 60000);
  if (minutes < 1) return { kind: 'justNow' }; // 含未来时间（时钟漂移）收敛为刚刚
  if (minutes < 60) return { kind: 'minutes', n: minutes };
  const hours = Math.floor(minutes / 60);
  if (hours < 24) return { kind: 'hours', n: hours };
  const days = Math.floor(hours / 24);
  if (days < 30) return { kind: 'days', n: days };
  return { kind: 'date', date: new Date(ms).toLocaleDateString() };
}

/** chunk_count 角标文案（缺字段/0/负数 → 空串隐藏；≥1000 缩写为 k）。 */
export function formatChunkCount(n: unknown): string {
  const v = typeof n === 'number' && Number.isFinite(n) ? Math.floor(n) : 0;
  if (v <= 0) return '';
  if (v >= 1000) return `${(v / 1000).toFixed(1).replace(/\.0$/, '')}k`;
  return String(v);
}
