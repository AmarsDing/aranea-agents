/** Link ReAct ACTION steps to nearby tool_call activity rows (51 envelope projection). */

import { toolEventFromMessage } from './envelopeToolCall';
import { isActivityMessage } from './mergeSessionMessages';
import type { ReactStep } from './reactPlannerTypes';
import type { Message, ReactStepWithTools, ToolUseEvent } from './types';

export type { ReactStepWithTools } from './types';

const TOOL_NAME_PATTERNS = [
  /functions\.([a-zA-Z0-9_-]+)/g,
  /function\s*:\s*([a-zA-Z0-9_-]+)/gi,
  /tool[_\s-]*name["\s:]+["']?([a-zA-Z0-9_-]+)/gi,
  /`([a-zA-Z0-9_-]+)`/g,
];

export function extractToolNamesFromActionBody(body: string): string[] {
  const text = (body || '').trim();
  if (!text) return [];
  const found = new Set<string>();
  for (const re of TOOL_NAME_PATTERNS) {
    re.lastIndex = 0;
    let m: RegExpExecArray | null;
    while ((m = re.exec(text)) !== null) {
      const name = String(m[1] ?? '').trim();
      if (name.length >= 2) found.add(name);
    }
  }
  return [...found];
}

/** Tool activity rows immediately after an assistant ReAct message (until next substantive assistant). */
export function collectToolEventsAfterMessage(messages: Message[], assistantIndex: number): ToolUseEvent[] {
  if (assistantIndex < 0 || assistantIndex >= messages.length) return [];
  const out: ToolUseEvent[] = [];
  for (let i = assistantIndex + 1; i < messages.length; i++) {
    const row = messages[i];
    if (row.role === 'assistant' && !isActivityMessage(row) && (row.content_markdown ?? '').trim()) {
      break;
    }
    if (row.role === 'user') break;
    const ev = toolEventFromMessage(row);
    if (ev) out.push(ev);
  }
  return out;
}

function toolMatchesHints(event: ToolUseEvent, hints: string[]): boolean {
  if (hints.length === 0) return true;
  const name = (event.tool_name || '').toLowerCase();
  const label = (event.display_label || event.tool_label || '').toLowerCase();
  return hints.some((h) => {
    const hint = h.toLowerCase();
    return name.includes(hint) || hint.includes(name) || label.includes(hint);
  });
}

function pickToolForAction(pool: ToolUseEvent[], used: Set<string>, hints: string[]): ToolUseEvent | null {
  const available = pool.filter((e) => !used.has(e.id));
  if (available.length === 0) return null;
  if (hints.length > 0) {
    const matched = available.find((e) => toolMatchesHints(e, hints));
    if (matched) return matched;
  }
  return available[0];
}

/** Attach tool_call envelope rows to ACTION steps on the same assistant message. */
export function enrichReactStepsWithToolEvents(
  steps: ReactStep[],
  assistantMessageIndex: number,
  messages: Message[],
): ReactStepWithTools[] {
  const pool = collectToolEventsAfterMessage(messages, assistantMessageIndex);
  const used = new Set<string>();
  return steps.map((step) => {
    if (step.kind !== 'action') {
      return { ...step, linkedTools: [] };
    }
    const hints = extractToolNamesFromActionBody(step.body);
    const linked: ToolUseEvent[] = [];
    const picked = pickToolForAction(pool, used, hints);
    if (picked) {
      used.add(picked.id);
      linked.push(picked);
    }
    return { ...step, linkedTools: linked };
  });
}
