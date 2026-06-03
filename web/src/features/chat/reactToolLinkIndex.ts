/** Session-level index for ReAct ACTION ↔ tool_call (avoids per-row O(n²) enrich). */

import { isActivityMessage } from './mergeSessionMessages';
import { enrichReactStepsWithToolEvents } from './reactPlannerToolLink';
import { parseReactPlannerContent } from './reactPlannerParse';
import type { Message, ReactStepWithTools, ReactToolLinkIndex } from './types';

export type { ReactToolLinkIndex } from './types';

export function emptyReactToolLinkIndex(): ReactToolLinkIndex {
  return {
    linkedToolIds: new Set(),
    stepsByAssistantIndex: new Map(),
  };
}

export function buildReactToolLinkIndex(messages: Message[]): ReactToolLinkIndex {
  if (!messages.length) return emptyReactToolLinkIndex();
  const linkedToolIds = new Set<string>();
  const stepsByAssistantIndex = new Map<number, ReactStepWithTools[]>();

  for (let i = 0; i < messages.length; i++) {
    const row = messages[i];
    if (row.role !== 'assistant' || isActivityMessage(row)) continue;
    const parsed = parseReactPlannerContent(row.content_markdown ?? '');
    if (!parsed?.steps.length) continue;
    const enriched = enrichReactStepsWithToolEvents(parsed.steps, i, messages);
    stepsByAssistantIndex.set(i, enriched);
    for (const step of enriched) {
      for (const tool of step.linkedTools) {
        if (tool.id) linkedToolIds.add(tool.id);
      }
    }
  }

  return { linkedToolIds, stepsByAssistantIndex };
}

export function isToolLinkedInReactIndex(index: ReactToolLinkIndex, toolEventId: string | undefined): boolean {
  if (!toolEventId?.trim()) return false;
  return index.linkedToolIds.has(toolEventId.trim());
}
