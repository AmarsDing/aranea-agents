/**
 * OBS-01: Auto-collapse composable for TurnBlockGroups.
 *
 * Manages collapsed/expanded state of chat turn blocks.
 * Blocks are never auto-collapsed — the entire conversation should remain
 * expanded so users can follow the temporal flow of thinking → tools → reply.
 * Users can still manually collapse or expand individual blocks.
 */
import { ref } from 'vue';
import type { TurnBlockGroup } from '../groupMessagesByTurn';

export function useAutoCollapse(turnBlocks: { value: TurnBlockGroup[] }) {
  /** Set of block keys that are currently collapsed. */
  const collapsedBlockKeys = ref<Set<number>>(new Set());

  /** Whether "expand all" mode is active (overrides individual collapses). */
  const expandAllActive = ref(false);

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
