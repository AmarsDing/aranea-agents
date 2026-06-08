/**
 * OBS-01: Auto-collapse composable for TurnBlockGroups.
 *
 * Manages collapsed/expanded state of chat turn blocks.
 * Completed blocks auto-collapse; users can manually toggle or expand all.
 */
import { ref, watch } from 'vue';
import type { TurnBlockGroup } from '../groupMessagesByTurn';

export function useAutoCollapse(turnBlocks: { value: TurnBlockGroup[] }) {
  /** Set of block keys that are currently collapsed. */
  const collapsedBlockKeys = ref<Set<number>>(new Set());

  /** Whether "expand all" mode is active (overrides individual collapses). */
  const expandAllActive = ref(false);

  /**
   * Watch turn blocks and auto-collapse newly completed blocks.
   * A block is auto-collapsed when:
   * 1. It transitions to isCompleted === true
   * 2. The user has not manually expanded it
   */
  watch(
    () => turnBlocks.value,
    (blocks, prevBlocks) => {
      if (expandAllActive.value) return;
      // Index previous blocks by key for O(1) lookup
      const prevMap = new Map<number, TurnBlockGroup>();
      if (prevBlocks) {
        for (const b of prevBlocks) prevMap.set(b.key, b);
      }
      for (const block of blocks) {
        if (!block.isCompleted) continue;
        const prev = prevMap.get(block.key);
        if (!prev) {
          // New block that is already completed — auto-collapse
          collapsedBlockKeys.value.add(block.key);
        } else if (!prev.isCompleted) {
          // Existing block just became completed — auto-collapse
          collapsedBlockKeys.value.add(block.key);
        }
      }
    },
    { deep: true },
  );

  /** Check if a block is collapsed. */
  function isCollapsed(blockKey: number): boolean {
    if (expandAllActive.value) return false;
    return collapsedBlockKeys.value.has(blockKey);
  }

  /** Toggle a block's collapsed state. */
  function toggleBlock(blockKey: number): void {
    expandAllActive.value = false;
    if (collapsedBlockKeys.value.has(blockKey)) {
      collapsedBlockKeys.value.delete(blockKey);
    } else {
      collapsedBlockKeys.value.add(blockKey);
    }
  }

  /** Expand all blocks (clears all collapses). */
  function expandAll(): void {
    expandAllActive.value = true;
    collapsedBlockKeys.value.clear();
  }

  /** Collapse all completed blocks. */
  function collapseAll(): void {
    expandAllActive.value = false;
    for (const block of turnBlocks.value) {
      if (block.isCompleted) {
        collapsedBlockKeys.value.add(block.key);
      }
    }
  }

  /** Reset state (e.g., on session change). */
  function reset(): void {
    collapsedBlockKeys.value.clear();
    expandAllActive.value = false;
  }

  return {
    collapsedBlockKeys,
    expandAllActive,
    isCollapsed,
    toggleBlock,
    expandAll,
    collapseAll,
    reset,
  };
}
