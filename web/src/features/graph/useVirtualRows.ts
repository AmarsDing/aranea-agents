// web/src/features/graph/useVirtualRows.ts
import { computed, onBeforeUnmount, onMounted, ref, watch } from 'vue';
import type { ComputedRef, Ref } from 'vue';

export type VirtualRow<T> = {
  item: T;
  /** 在完整列表中的索引（用于 key 与 top 定位） */
  index: number;
  /** 行绝对定位 top = index * rowHeight */
  top: number;
};

/**
 * R2-6 固定行高虚拟滚动 composable（详情面板节点列表 / Schema 抽屉共用）。
 *
 * 算法：scrollTop → 窗口 [start, end)，buffer 上下各多渲染 buffer 行；
 * 容器 position:relative + 行 position:absolute; top: index*rowHeight。
 * 512 行仅渲染可视区 ±buffer（约 15~20 个 DOM 行）。
 */
export function useVirtualRows<T>(options: {
  rows: Ref<readonly T[]> | ComputedRef<readonly T[]>;
  rowHeight: number;
  /** 可视区上下各多渲染的行数，默认 5 */
  buffer?: number;
}) {
  const { rows, rowHeight } = options;
  const buffer = options.buffer ?? 5;

  const containerRef = ref<HTMLElement | null>(null);
  const scrollTop = ref(0);
  const viewportHeight = ref(0);

  let resizeObserver: ResizeObserver | null = null;

  function measure() {
    const el = containerRef.value;
    if (!el) return;
    viewportHeight.value = el.clientHeight;
    scrollTop.value = el.scrollTop;
  }

  function onScroll(event?: Event) {
    const el = event?.currentTarget instanceof HTMLElement ? event.currentTarget : containerRef.value;
    if (!el) return;
    scrollTop.value = el.scrollTop;
  }

  onMounted(() => {
    measure();
    const el = containerRef.value;
    if (el && typeof ResizeObserver !== 'undefined') {
      resizeObserver = new ResizeObserver(() => measure());
      resizeObserver.observe(el);
    }
  });

  onBeforeUnmount(() => {
    resizeObserver?.disconnect();
    resizeObserver = null;
  });

  // 行数变化（过滤/增删）时回到顶部并重新测量，避免窗口越界停留在空白区
  watch(
    () => rows.value.length,
    () => {
      const el = containerRef.value;
      if (el) el.scrollTop = 0;
      scrollTop.value = 0;
      measure();
    },
  );

  const start = computed(() => {
    const firstVisible = Math.floor(scrollTop.value / rowHeight);
    return Math.max(0, Math.min(firstVisible - buffer, Math.max(rows.value.length - 1, 0)));
  });

  const end = computed(() => {
    const lastVisible = Math.ceil((scrollTop.value + viewportHeight.value) / rowHeight);
    return Math.min(rows.value.length, lastVisible + buffer);
  });

  const visibleRows = computed<VirtualRow<T>[]>(() => {
    const s = start.value;
    const e = Math.max(end.value, s);
    const out: VirtualRow<T>[] = [];
    for (let i = s; i < e; i++) {
      const item = rows.value[i];
      if (item === undefined) break;
      out.push({ item, index: i, top: i * rowHeight });
    }
    return out;
  });

  const totalHeight = computed(() => rows.value.length * rowHeight);

  return {
    containerRef,
    visibleRows,
    totalHeight,
    onScroll,
    measure,
  };
}
