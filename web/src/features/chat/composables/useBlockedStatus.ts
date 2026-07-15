// web/src/features/chat/composables/useBlockedStatus.ts
import { computed, toValue, type MaybeRef } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task } from '../v2Types';

export type BlockedType = 'none' | 'tool' | 'confirm' | 'llm';

export interface BlockedResult {
  /** True when any blocking condition is active (type !== 'none'). */
  blocked: boolean;
  type: BlockedType;
  agentKey?: string;
  stepId?: string;
}

/** @deprecated Use {@link BlockedResult} instead. Kept for backward compatibility. */
export type BlockedInfo = BlockedResult;

export const EMPTY_BLOCKED: BlockedResult = { blocked: false, type: 'none' };

/**
 * useBlockedStatus scans the store for steps with blocking statuses.
 * Replaces the old tree-walking implementation with direct store queries.
 *
 * @param tasks MaybeRef<Task[]> — accepts both a Ref and a plain array,
 *   so callers can pass `session.v2Tasks` (auto-unwrapped by reactive)
 *   without manually wrapping with toRef.
 */
export function useBlockedStatus(tasks: MaybeRef<Task[]>) {
  const store = useChatActivityStore();

  const blockedInfo = computed<BlockedResult>(() => {
    const list = toValue(tasks);
    for (const task of list) {
      if (task.Status !== 'running') continue;
      // Scan all steps directly by TaskID (Step carries its own TaskID field,
      // so we don't need to go through the Turns map — this also catches steps
      // that arrive before their Turn entity).
      for (const step of store.steps.values()) {
        if (step.TaskID !== task.ID) continue;
        if (step.Status === 'tool_blocked') {
          return { blocked: true, type: 'tool', agentKey: step.AuthorAgentKey, stepId: step.ID };
        }
        if (step.Kind === 'confirm' && step.Status === 'running') {
          return { blocked: true, type: 'confirm', agentKey: step.AuthorAgentKey, stepId: step.ID };
        }
      }
    }
    return EMPTY_BLOCKED;
  });

  return { blockedInfo };
}
