// web/src/features/chat/composables/useBlockedStatus.ts
import { computed, type Ref } from 'vue';
import { useChatActivityStore } from '../../../stores/chat/activityV2Store';
import type { Task } from '../v2Types';

export type BlockedType = 'none' | 'tool' | 'confirm' | 'llm';

export interface BlockedInfo {
  type: BlockedType;
  agentKey?: string;
  stepId?: string;
}

export const EMPTY_BLOCKED: BlockedInfo = { type: 'none' };

/**
 * useBlockedStatus scans the store for steps with blocking statuses.
 * Replaces the old tree-walking implementation with direct store queries.
 */
export function useBlockedStatus(tasks: Ref<Task[]>) {
  const store = useChatActivityStore();

  const blockedInfo = computed<BlockedInfo>(() => {
    for (const task of tasks.value) {
      if (task.Status !== 'running') continue;
      // Scan all steps directly by TaskID (Step carries its own TaskID field,
      // so we don't need to go through the Turns map — this also catches steps
      // that arrive before their Turn entity).
      for (const step of store.steps.values()) {
        if (step.TaskID !== task.ID) continue;
        if (step.Status === 'tool_blocked') {
          return { type: 'tool', agentKey: step.AuthorAgentKey, stepId: step.ID };
        }
        if (step.Kind === 'confirm' && step.Status === 'running') {
          return { type: 'confirm', agentKey: step.AuthorAgentKey, stepId: step.ID };
        }
      }
    }
    return EMPTY_BLOCKED;
  });

  return { blockedInfo };
}
