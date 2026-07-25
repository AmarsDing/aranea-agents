// web/src/features/graph/__tests__/useGraphValidationDock.spec.ts
import { describe, it, expect, vi } from 'vitest';
import { ref, nextTick, defineComponent } from 'vue';
import { mount } from '@vue/test-utils';
import { useGraphValidationDock } from '../useGraphValidationDock';
import type { ValidationIssue } from '../types';

function makeIssue(partial: Partial<ValidationIssue>): ValidationIssue {
  return {
    nodeId: '',
    nodeLabel: '',
    level: 'error',
    code: 'X',
    field: '',
    message: 'm',
    ...partial,
  };
}

function mountDock(issues: ReturnType<typeof ref<ValidationIssue[]>>, onRevalidate?: () => Promise<void> | void) {
  const Comp = defineComponent({
    setup() {
      const dock = useGraphValidationDock(issues, { onRevalidate });
      return { dock };
    },
    template: '<div />',
  });
  return mount(Comp);
}

describe('useGraphValidationDock - R2-7 page integration', () => {
  it('starts with panel closed and no spotlight', () => {
    const wrapper = mountDock(ref([]));
    expect(wrapper.vm.dock.panelOpen.value).toBe(false);
    expect(wrapper.vm.dock.spotlightNodeId.value).toBeNull();
    wrapper.unmount();
  });

  it('derives counts and nodeIssueMap from issues (error wins over warning)', () => {
    const issues = ref<ValidationIssue[]>([
      makeIssue({ nodeId: 'n1', nodeLabel: 'A', level: 'warning', code: 'W1', message: 'warn' }),
      makeIssue({ nodeId: 'n1', nodeLabel: 'A', level: 'error', code: 'E1', message: 'err' }),
      makeIssue({ nodeId: 'n2', nodeLabel: 'B', level: 'warning', code: 'W2', message: 'warn2' }),
      makeIssue({ nodeId: '', level: 'error', code: 'G1', message: 'graph-level' }),
    ]);
    const wrapper = mountDock(issues);
    expect(wrapper.vm.dock.errorCount.value).toBe(2);
    expect(wrapper.vm.dock.warningCount.value).toBe(2);
    const map = wrapper.vm.dock.nodeIssueMap.value;
    expect(Object.keys(map).sort()).toEqual(['n1', 'n2']);
    expect(map.n1.level).toBe('error');
    expect(map.n2.level).toBe('warning');
    wrapper.unmount();
  });

  it('togglePanel opens and closes; closing clears spotlight', async () => {
    const wrapper = mountDock(ref([]));
    const dock = wrapper.vm.dock;
    dock.togglePanel();
    expect(dock.panelOpen.value).toBe(true);
    dock.locateNode('n1');
    expect(dock.spotlightNodeId.value).toBe('n1');
    dock.togglePanel();
    expect(dock.panelOpen.value).toBe(false);
    expect(dock.spotlightNodeId.value).toBeNull();
    wrapper.unmount();
  });

  it('locateNode keeps panel open and sets spotlight', () => {
    const wrapper = mountDock(ref([]));
    const dock = wrapper.vm.dock;
    dock.togglePanel();
    dock.locateNode('n9');
    expect(dock.panelOpen.value).toBe(true);
    expect(dock.spotlightNodeId.value).toBe('n9');
    wrapper.unmount();
  });

  it('Esc clears spotlight first, then closes panel', async () => {
    const wrapper = mountDock(ref([]));
    const dock = wrapper.vm.dock;
    dock.togglePanel();
    dock.locateNode('n1');
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await nextTick();
    expect(dock.spotlightNodeId.value).toBeNull();
    expect(dock.panelOpen.value).toBe(true);
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await nextTick();
    expect(dock.panelOpen.value).toBe(false);
    wrapper.unmount();
  });

  it('Esc does nothing when panel closed and no spotlight', async () => {
    const wrapper = mountDock(ref([]));
    const dock = wrapper.vm.dock;
    window.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape' }));
    await nextTick();
    expect(dock.panelOpen.value).toBe(false);
    expect(dock.spotlightNodeId.value).toBeNull();
    wrapper.unmount();
  });

  it('revalidate invokes callback with validating flag toggling', async () => {
    let resolveFn: (() => void) | null = null;
    const onRevalidate = vi.fn(
      () =>
        new Promise<void>((resolve) => {
          resolveFn = resolve;
        }),
    );
    const wrapper = mountDock(ref([]), onRevalidate);
    const dock = wrapper.vm.dock;
    const p = dock.revalidate();
    expect(dock.validating.value).toBe(true);
    resolveFn!();
    await p;
    expect(onRevalidate).toHaveBeenCalledTimes(1);
    expect(dock.validating.value).toBe(false);
    wrapper.unmount();
  });

  it('clearSpotlight resets spotlight without closing panel', () => {
    const wrapper = mountDock(ref([]));
    const dock = wrapper.vm.dock;
    dock.togglePanel();
    dock.locateNode('n1');
    dock.clearSpotlight();
    expect(dock.spotlightNodeId.value).toBeNull();
    expect(dock.panelOpen.value).toBe(true);
    wrapper.unmount();
  });
});
