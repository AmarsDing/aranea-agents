// web/src/features/graph/__tests__/useVirtualRows.spec.ts
import { describe, it, expect } from 'vitest';
import { ref, computed, nextTick } from 'vue';
import { useVirtualRows } from '../useVirtualRows';

function makeRows(n: number) {
  return Array.from({ length: n }, (_, i) => ({ id: `row-${i}` }));
}

describe('useVirtualRows - fixed row height windowing (R2-6)', () => {
  it('renders only viewport + buffer rows', async () => {
    const rows = ref(makeRows(512));
    const { containerRef, visibleRows, totalHeight, onScroll, measure } = useVirtualRows({
      rows,
      rowHeight: 32,
      buffer: 5,
    });

    // 模拟容器：高度 320px（10 行可视），scrollTop = 0
    const el = document.createElement('div');
    Object.defineProperty(el, 'clientHeight', { value: 320, configurable: true });
    containerRef.value = el;
    measure();
    await nextTick();

    expect(totalHeight.value).toBe(512 * 32);
    // start=0, end=ceil(320/32)+5=15 → 15 行
    expect(visibleRows.value.length).toBe(15);
    expect(visibleRows.value[0].index).toBe(0);
    expect(visibleRows.value[0].top).toBe(0);
    expect(visibleRows.value[14].index).toBe(14);
    expect(visibleRows.value[14].top).toBe(14 * 32);
  });

  it('slides window on scroll with buffer', async () => {
    const rows = ref(makeRows(512));
    const { containerRef, visibleRows, onScroll, measure } = useVirtualRows({
      rows,
      rowHeight: 32,
      buffer: 5,
    });

    const el = document.createElement('div');
    Object.defineProperty(el, 'clientHeight', { value: 320, configurable: true });
    containerRef.value = el;
    measure();

    // 滚动到 3200px（第 100 行）
    Object.defineProperty(el, 'scrollTop', { value: 3200, writable: true, configurable: true });
    onScroll();
    await nextTick();

    // start = floor(3200/32) - 5 = 95; end = ceil((3200+320)/32) + 5 = 115
    expect(visibleRows.value[0].index).toBe(95);
    expect(visibleRows.value[visibleRows.value.length - 1].index).toBe(114);
    expect(visibleRows.value.length).toBe(20);
  });

  it('clamps window at list end', async () => {
    const rows = ref(makeRows(12));
    const { containerRef, visibleRows, onScroll, measure } = useVirtualRows({
      rows,
      rowHeight: 32,
      buffer: 5,
    });

    const el = document.createElement('div');
    Object.defineProperty(el, 'clientHeight', { value: 320, configurable: true });
    Object.defineProperty(el, 'scrollTop', { value: 99999, writable: true, configurable: true });
    containerRef.value = el;
    measure();
    onScroll();
    await nextTick();

    const last = visibleRows.value[visibleRows.value.length - 1];
    expect(last.index).toBe(11);
  });

  it('reacts to rows shrink (filter) by clamping window', async () => {
    const rows = ref(makeRows(512));
    const filtered = computed(() => rows.value);
    const { containerRef, visibleRows, onScroll, measure } = useVirtualRows({
      rows: filtered,
      rowHeight: 32,
      buffer: 5,
    });

    const el = document.createElement('div');
    Object.defineProperty(el, 'clientHeight', { value: 320, configurable: true });
    Object.defineProperty(el, 'scrollTop', { value: 16000, writable: true, configurable: true });
    containerRef.value = el;
    measure();
    onScroll();
    await nextTick();
    expect(visibleRows.value[visibleRows.value.length - 1].index).toBe(511);

    // 过滤后只剩 3 行
    rows.value = makeRows(3);
    await nextTick();
    expect(visibleRows.value.length).toBe(3);
    expect(visibleRows.value[0].index).toBe(0);
  });

  it('handles empty rows', async () => {
    const rows = ref<Array<{ id: string }>>([]);
    const { containerRef, visibleRows, totalHeight, measure } = useVirtualRows({
      rows,
      rowHeight: 32,
    });
    const el = document.createElement('div');
    Object.defineProperty(el, 'clientHeight', { value: 320, configurable: true });
    containerRef.value = el;
    measure();
    await nextTick();

    expect(visibleRows.value).toEqual([]);
    expect(totalHeight.value).toBe(0);
  });
});
