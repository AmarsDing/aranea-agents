import { describe, it, expect } from 'vitest';
import { ref, nextTick } from 'vue';
import { useAutoCollapse } from '../useAutoCollapse';
import type { TurnBlockGroup } from '../../groupMessagesByTurn';

function makeBlock(overrides: Partial<TurnBlockGroup> & Pick<TurnBlockGroup, 'key'>): TurnBlockGroup {
  return {
    turnId: `turn-${overrides.key}`,
    user: null,
    assistants: [],
    rounds: [],
    tools: [],
    members: [],
    isCompleted: false,
    ...overrides,
  };
}

describe('useAutoCollapse', () => {
  it('initially no blocks are collapsed', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { isCollapsed } = useAutoCollapse(turnBlocks);

    expect(isCollapsed(0)).toBe(false);
    expect(isCollapsed(1)).toBe(false);
  });

  it('isCollapsed(key) returns false for unknown keys', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { isCollapsed } = useAutoCollapse(turnBlocks);

    expect(isCollapsed(99)).toBe(false);
    expect(isCollapsed(42)).toBe(false);
  });

  it('toggleBlock(key) collapses an uncollapsed block', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { isCollapsed, toggleBlock } = useAutoCollapse(turnBlocks);

    toggleBlock(1);
    expect(isCollapsed(1)).toBe(true);
  });

  it('toggleBlock(key) expands a collapsed block', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { isCollapsed, toggleBlock } = useAutoCollapse(turnBlocks);

    toggleBlock(1);
    expect(isCollapsed(1)).toBe(true);

    toggleBlock(1);
    expect(isCollapsed(1)).toBe(false);
  });

  it('expandAll() clears all collapses and sets expandAllActive', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { isCollapsed, toggleBlock, expandAll, expandAllActive } = useAutoCollapse(turnBlocks);

    toggleBlock(1);
    toggleBlock(2);
    expect(isCollapsed(1)).toBe(true);
    expect(isCollapsed(2)).toBe(true);

    expandAll();
    expect(expandAllActive.value).toBe(true);
  });

  it('after expandAll(), isCollapsed() returns false even for previously collapsed blocks', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { isCollapsed, toggleBlock, expandAll } = useAutoCollapse(turnBlocks);

    toggleBlock(1);
    toggleBlock(2);
    expandAll();

    expect(isCollapsed(1)).toBe(false);
    expect(isCollapsed(2)).toBe(false);
  });

  it('collapseAll() collapses all completed blocks with tools/members', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([
      makeBlock({ key: 0, isCompleted: true, tools: [{} as any] }),
      makeBlock({ key: 1, isCompleted: false }),
      makeBlock({ key: 2, isCompleted: true, members: [{} as any] }),
      makeBlock({ key: 3, isCompleted: true }), // pure assistant reply
    ]);
    const { isCollapsed, collapseAll, expandAllActive } = useAutoCollapse(turnBlocks);

    collapseAll();

    expect(isCollapsed(0)).toBe(true);
    expect(isCollapsed(1)).toBe(false);
    expect(isCollapsed(2)).toBe(true);
    expect(isCollapsed(3)).toBe(false); // pure assistant reply not collapsed
    expect(expandAllActive.value).toBe(false);
  });

  it('reset() clears all state', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([
      makeBlock({ key: 0, isCompleted: true }),
    ]);
    const { isCollapsed, toggleBlock, expandAll, expandAllActive, reset } = useAutoCollapse(turnBlocks);

    toggleBlock(0);
    expandAll();
    reset();

    expect(isCollapsed(0)).toBe(false);
    expect(expandAllActive.value).toBe(false);
  });

  it('auto-collapses newly completed blocks via watcher', async () => {
    const turnBlocks = ref<TurnBlockGroup[]>([
      makeBlock({ key: 0, isCompleted: false, tools: [{} as any] }),
    ]);
    const { isCollapsed } = useAutoCollapse(turnBlocks);

    expect(isCollapsed(0)).toBe(false);

    // Simulate block becoming completed
    turnBlocks.value = [makeBlock({ key: 0, isCompleted: true, tools: [{} as any] })];
    await nextTick();

    expect(isCollapsed(0)).toBe(true);
  });

  it('does not auto-collapse pure assistant replies (no tools, no members)', async () => {
    const turnBlocks = ref<TurnBlockGroup[]>([
      makeBlock({ key: 0, isCompleted: false }),
    ]);
    const { isCollapsed } = useAutoCollapse(turnBlocks);

    // Simulate block becoming completed with no tools/members
    turnBlocks.value = [makeBlock({ key: 0, isCompleted: true })];
    await nextTick();

    expect(isCollapsed(0)).toBe(false);
  });

  it('does not auto-collapse when expandAllActive is true', async () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { isCollapsed, expandAll } = useAutoCollapse(turnBlocks);

    expandAll();

    turnBlocks.value = [makeBlock({ key: 0, isCompleted: true })];
    await nextTick();

    expect(isCollapsed(0)).toBe(false);
  });

  it('toggleBlock deactivates expandAllActive', () => {
    const turnBlocks = ref<TurnBlockGroup[]>([]);
    const { expandAllActive, expandAll, toggleBlock } = useAutoCollapse(turnBlocks);

    expandAll();
    expect(expandAllActive.value).toBe(true);

    toggleBlock(1);
    expect(expandAllActive.value).toBe(false);
  });
});
